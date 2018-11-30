package main

import (
	"bytes"
	"log"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"github.com/valyala/fasthttp"
	"github.com/xtrafrancyz/vk-proxy/replacer"
)

const (
	readBufferSize = 8192
)

var (
	atPath = []byte("/@")

	gzip            = []byte("gzip")
	vkProxyName     = []byte("vk-proxy")
	setCookie       = []byte("Set-Cookie")
	acceptEncoding  = []byte("Accept-Encoding")
	contentEncoding = []byte("Content-Encoding")
	serverHeader    = []byte("Server")
)

type ProxyConfig struct {
	ReduceMemoryUsage bool
	BaseDomain        string
	LogVerbosity      int
	GzipUpstream      bool
	FilterFeed        bool
}

type Proxy struct {
	client   *fasthttp.Client
	server   *fasthttp.Server
	replacer *replacer.Replacer
	tracker  *tracker
	config   ProxyConfig
}

func NewProxy(config ProxyConfig) *Proxy {
	p := &Proxy{
		client: &fasthttp.Client{
			Name:           "vk-proxy",
			ReadBufferSize: readBufferSize,
		},
		replacer: &replacer.Replacer{},
		tracker: &tracker{
			uniqueUsers: make(map[string]bool),
		},
		config: config,
	}
	p.server = &fasthttp.Server{
		Handler:           p.handleProxy,
		ReduceMemoryUsage: config.ReduceMemoryUsage,
		ReadBufferSize:    readBufferSize,
		Name:              "vk-proxy",
	}
	if p.config.LogVerbosity > 0 {
		p.tracker.start()
	}
	return p
}

func (p *Proxy) ListenTCP(host string) error {
	log.Printf("Starting server on http://%s", host)
	return p.server.ListenAndServe(host)
}

func (p *Proxy) ListenUnix(path string) error {
	log.Printf("Starting server on http://unix:%s", path)
	return p.server.ListenAndServeUNIX(path, 0777)
}

func (p *Proxy) handler(ctx *fasthttp.RequestCtx) {
	switch string(ctx.Path()) {
	case "/away", "/away.php":
		p.handleAway(ctx)
	default:
		p.handleProxy(ctx)
	}
}

func (p *Proxy) handleAway(ctx *fasthttp.RequestCtx) {
	to := string(ctx.QueryArgs().Peek("to"))
	if to == "" {
		ctx.Error("Bad Request: 'to' argument is not set", 400)
		return
	}
	to, err := url.QueryUnescape(to)
	if err != nil {
		ctx.Error("Bad Request: could not unescape url", 400)
		return
	}
	ctx.Redirect(to, fasthttp.StatusMovedPermanently)
}

func (p *Proxy) handleProxy(ctx *fasthttp.RequestCtx) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic when proxying the request: %s%s", r, debug.Stack())
			ctx.Error("500 Internal Server Error", 500)
		}
	}()
	start := time.Now()

	var baseDomain string
	if p.config.BaseDomain != "" {
		baseDomain = p.config.BaseDomain
	} else {
		baseDomain = string(ctx.Host())
	}

	if !p.prepareProxyRequest(ctx) {
		ctx.Error("400 Bad Request", 400)
		return
	}

	err := p.client.DoTimeout(&ctx.Request, &ctx.Response, 30*time.Second)
	if err == nil {
		err = p.processProxyResponse(baseDomain, ctx)
	}

	elapsed := time.Since(start).Round(100 * time.Microsecond)

	if err != nil {
		log.Printf("%s %s error: %s", elapsed, ctx.Path(), err)
		if strings.Contains(err.Error(), "timed out") || strings.Contains(err.Error(), "timeout") {
			ctx.Error("408 Request Timeout", 408)
		} else {
			ctx.Error("500 Internal Server Error", 500)
		}
		return
	}

	if p.config.LogVerbosity > 0 {
		ip := ctx.Request.Header.Peek("CF-Connecting-IP") // Cloudflare
		if ip == nil {
			ip = ctx.Request.Header.Peek("X-Real-IP") // nginx
		}
		if ip == nil {
			ip = []byte(ctx.RemoteIP().String()) // real
		}
		p.tracker.trackRequest(string(ip), len(ctx.Response.Body()))
	}

	if p.config.LogVerbosity == 2 {
		log.Printf("%s %s %s", elapsed, ctx.Path(), bytefmt.ByteSize(uint64(len(ctx.Response.Body()))))
	} else if p.config.LogVerbosity == 3 {
		log.Printf("%s %s %s\n%s", elapsed, ctx.Path(), bytefmt.ByteSize(uint64(len(ctx.Response.Body()))), ctx.Response.Body())
	}
}

func (p *Proxy) prepareProxyRequest(ctx *fasthttp.RequestCtx) bool {
	req := &ctx.Request
	path := req.RequestURI()
	if bytes.HasPrefix(path, atPath) {
		slashIndex := bytes.IndexRune(path[1:], '/')
		if slashIndex == -1 {
			return false
		}
		endpoint := string(path[len(atPath) : slashIndex+1])
		if endpoint != "vk.com" && !strings.HasSuffix(endpoint, ".vk.com") {
			return false
		}
		req.Header.SetHost(endpoint)
		req.SetRequestURIBytes([]byte(path[1+slashIndex:]))
	} else {
		req.SetHost("api.vk.com")
	}
	// After req.URI() call it is impossible to modify URI
	req.URI().SetScheme("https")
	if p.config.GzipUpstream {
		req.Header.SetBytesKV(acceptEncoding, gzip)
	} else {
		req.Header.DelBytes(acceptEncoding)
	}
	return true
}

func (p *Proxy) processProxyResponse(baseDomain string, ctx *fasthttp.RequestCtx) error {
	res := &ctx.Response
	res.Header.DelBytes(setCookie)
	res.Header.SetBytesKV(serverHeader, vkProxyName)

	var body []byte
	// Gunzip body if needed
	if bytes.Contains(res.Header.PeekBytes(contentEncoding), gzip) {
		res.Header.DelBytes(contentEncoding)
		var err error
		body, err = res.BodyGunzip()
		if err != nil {
			return err
		}
	} else {
		body = res.Body()
	}

	body = p.replacer.DoReplace(body, replacer.ReplaceContext{
		BaseDomain: baseDomain,
		Domain:     string(ctx.Host()),
		Path:       string(ctx.Path()),
		FilterFeed: p.config.FilterFeed,
	})

	res.SetBody(body)
	return nil
}

type tracker struct {
	lock        sync.Mutex
	requests    uint32
	bytes       uint64
	uniqueUsers map[string]bool
}

func (t *tracker) start() {
	go func() {
		for range time.Tick(60 * time.Second) {
			t.lock.Lock()
			log.Printf("Requests: %d, Traffic: %s, Online: %d", t.requests, bytefmt.ByteSize(t.bytes), len(t.uniqueUsers))
			t.requests = 0
			t.bytes = 0
			t.uniqueUsers = make(map[string]bool)
			t.lock.Unlock()
		}
	}()
}

func (t *tracker) trackRequest(ip string, size int) {
	t.lock.Lock()

	t.uniqueUsers[ip] = true
	t.requests++
	t.bytes += uint64(size)

	t.lock.Unlock()
}
