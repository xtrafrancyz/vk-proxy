package main

import (
	"bytes"
	"crypto/tls"
	"log"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
	"github.com/xtrafrancyz/vk-proxy/bytefmt"
	"github.com/xtrafrancyz/vk-proxy/replacer"
)

const (
	readBufferSize = 8192

	byteBufferPoolSetupRounds = 42000   // == bytebufferpool.calibrateCallsThreshold
	byteBufferPoolSetupSize   = 2097152 // 2**21
)

func init() {
	// Настройка размера буфера для ответа
	r := fasthttp.Response{}
	for i := 0; i <= byteBufferPoolSetupRounds; i++ {
		r.SetBodyString("")
		if b := r.Body(); cap(b) != byteBufferPoolSetupSize {
			r.SwapBody(make([]byte, byteBufferPoolSetupSize))
		} else {
			r.SwapBody(b[:cap(b)])
		}
		r.ResetBody()
	}

	// Настройка размера реплейсера
	for i := 0; i <= byteBufferPoolSetupRounds; i++ {
		b := replacer.AcquireBuffer()
		if cap(b.B) != byteBufferPoolSetupSize {
			b.B = make([]byte, byteBufferPoolSetupSize)
		} else {
			b.B = b.B[:cap(b.B)]
		}
		replacer.ReleaseBuffer(b)
	}
}

var (
	gzip        = []byte("gzip")
	vkProxyName = []byte("vk-proxy")

	replaceContextPool = sync.Pool{New: func() interface{} {
		return &replacer.ReplaceContext{}
	}}
)

type ProxyConfig struct {
	ReduceMemoryUsage bool
	BaseDomain        string
	BaseStaticDomain  string
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
			Name:                      "vk-proxy",
			ReadBufferSize:            readBufferSize,
			TLSConfig:                 &tls.Config{InsecureSkipVerify: true},
			ReadTimeout:               30 * time.Second,
			WriteTimeout:              10 * time.Second,
			DisablePathNormalizing:    true,
			NoDefaultUserAgentHeader:  true,
			MaxIdemponentCallAttempts: 0,
			RetryIf: func(request *fasthttp.Request) bool {
				return false
			},
		},
		replacer: &replacer.Replacer{
			ProxyBaseDomain:   config.BaseDomain,
			ProxyStaticDomain: config.BaseStaticDomain,
		},
		tracker: &tracker{
			uniqueUsers: make(map[string]bool),
		},
		config: config,
	}
	p.server = &fasthttp.Server{
		Handler:                      p.handleProxy,
		ReduceMemoryUsage:            config.ReduceMemoryUsage,
		ReadBufferSize:               readBufferSize,
		ReadTimeout:                  10 * time.Second,
		WriteTimeout:                 20 * time.Second,
		IdleTimeout:                  1 * time.Minute,
		NoDefaultContentType:         true,
		DisablePreParseMultipartForm: true,
		Name:                         "vk-proxy",
	}
	p.tracker.server = p.server
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

func (p *Proxy) handleProxy(ctx *fasthttp.RequestCtx) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic when proxying the request: %s%s", r, debug.Stack())
			ctx.Error("500 Internal Server Error", 500)
		}
	}()
	start := time.Now()

	replaceContext := replaceContextPool.Get().(*replacer.ReplaceContext)
	replaceContext.Method = ctx.Method()
	replaceContext.OriginHost = string(ctx.Request.Host())
	replaceContext.FilterFeed = p.config.FilterFeed

	if !p.prepareProxyRequest(ctx, replaceContext) {
		ctx.Error("400 Bad Request", 400)
		return
	}

	if replaceContext.Host == "api.vk.com" &&
		(replaceContext.Path == "/away" || replaceContext.Path == "/away.php") {
		p.handleAway(ctx)
		return
	}

	err := p.client.Do(&ctx.Request, &ctx.Response)
	if err == nil {
		err = p.processProxyResponse(ctx, replaceContext)
	}

	replaceContext.Reset()
	replaceContextPool.Put(replaceContext)

	elapsed := time.Since(start).Round(100 * time.Microsecond)

	if err != nil {
		log.Printf("%s %s %s%s error: %s", elapsed, ctx.Request.Header.Method(), ctx.Host(), ctx.Path(), err)
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
			ip = ctx.Request.Header.Peek(fasthttp.HeaderXForwardedFor) // nginx
		}
		if ip == nil {
			ip = []byte(ctx.RemoteIP().String()) // real
		}
		p.tracker.trackRequest(string(ip), len(ctx.Response.Body()))
	}

	if p.config.LogVerbosity == 2 {
		log.Printf("%s %s %s%s %s", elapsed, ctx.Request.Header.Method(), ctx.Host(), ctx.Path(),
			bytefmt.ByteSize(uint64(len(ctx.Response.Body()))))
	} else if p.config.LogVerbosity == 3 {
		log.Printf("%s %s %s%s %s\n%s", elapsed, ctx.Request.Header.Method(), ctx.Host(), ctx.Path(),
			bytefmt.ByteSize(uint64(len(ctx.Response.Body()))), ctx.Response.Body())
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

func (p *Proxy) prepareProxyRequest(ctx *fasthttp.RequestCtx, replaceContext *replacer.ReplaceContext) bool {
	// Routing
	req := &ctx.Request
	uri := string(req.RequestURI())
	host := ""
	if strings.HasPrefix(uri, "/@") {
		slashIndex := strings.IndexByte(uri[2:], '/')
		if slashIndex == -1 {
			return false
		}
		endpoint := uri[2 : slashIndex+2]
		if endpoint != "vk.com" &&
			!strings.HasSuffix(endpoint, ".vk.com") &&
			!strings.HasSuffix(endpoint, ".vkuseraudio.net") &&
			!strings.HasSuffix(endpoint, ".vkuseraudio.com") {
			return false
		}
		host = endpoint
		uri = uri[2+slashIndex:]
		req.SetRequestURI(uri)
	} else if altHost := req.Header.Peek("Proxy-Host"); altHost != nil {
		host = string(altHost)
		switch host {
		case "static.vk.com":
		case "oauth.vk.com":
		default:
			return false
		}
		req.Header.Del("Proxy-Host")
	} else {
		host = "api.vk.com"
	}
	req.SetHost(host)

	// Replace some request data
	replaceContext.Host = host
	replaceContext.Path = string(ctx.Path())
	p.replacer.DoReplaceRequest(req, replaceContext)

	// After req.URI() call it is impossible to modify URI
	req.URI().SetScheme("https")
	if p.config.GzipUpstream {
		req.Header.SetBytesV(fasthttp.HeaderAcceptEncoding, gzip)
	} else {
		req.Header.Del(fasthttp.HeaderAcceptEncoding)
	}

	req.Header.Del(fasthttp.HeaderConnection)
	return true
}

func (p *Proxy) processProxyResponse(ctx *fasthttp.RequestCtx, replaceContext *replacer.ReplaceContext) error {
	res := &ctx.Response
	res.Header.Del(fasthttp.HeaderSetCookie)
	res.Header.Del(fasthttp.HeaderConnection)
	res.Header.SetBytesV(fasthttp.HeaderServer, vkProxyName)

	var buf *bytebufferpool.ByteBuffer
	// Gunzip body if needed
	if bytes.Contains(res.Header.Peek(fasthttp.HeaderContentEncoding), gzip) {
		res.Header.Del(fasthttp.HeaderContentEncoding)
		buf = replacer.AcquireBuffer()
		_, err := fasthttp.WriteGunzip(buf, res.Body())
		if err != nil {
			replacer.ReleaseBuffer(buf)
			return err
		}
		replacer.ReleaseBuffer(&bytebufferpool.ByteBuffer{
			B: res.SwapBody(nil),
		})
	} else {
		buf = &bytebufferpool.ByteBuffer{
			B: res.SwapBody(nil),
		}
	}

	buf = p.replacer.DoReplaceResponse(res, buf, replaceContext)

	// avoid copying and save old buffer
	buf.B = res.SwapBody(buf.B)
	if cap(buf.B) > 10 {
		replacer.ReleaseBuffer(buf)
	}
	return nil
}

type tracker struct {
	lock        sync.Mutex
	requests    uint32
	bytes       uint64
	uniqueUsers map[string]bool
	server      *fasthttp.Server
}

func (t *tracker) start() {
	go func() {
		for range time.Tick(60 * time.Second) {
			t.lock.Lock()
			log.Printf("Requests: %d, Traffic: %s, Online: %d, Concurrency: %d",
				t.requests, bytefmt.ByteSize(t.bytes), len(t.uniqueUsers),
				t.server.GetCurrentConcurrency(),
			)
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
