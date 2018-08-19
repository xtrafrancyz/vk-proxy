package main

import (
	"bytes"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/json-iterator/go"
	"github.com/valyala/fasthttp"
)

type domainConfig struct {
	apiReplaces                []replace
	apiOfficialLongpollReplace replace
	apiLongpollReplace         replace
	siteHlsReplace             replace
}

var client = &fasthttp.Client{
	Name: "vk-proxy",
}
var domains = make(map[string]*domainConfig)
var json = jsoniter.ConfigFastest

func getDomainConfig(domain string) *domainConfig {
	cfg, ok := domains[domain]
	if !ok {
		var replacementStart = []byte(`\/\/` + domain + `\/_\/`)
		cfg = &domainConfig{}
		cfg.apiReplaces = []replace{
			newStringReplace(`"https:\/\/vk.com\/video_hls.php`, `"https:\/\/`+domain+`\/@vk.com\/video_hls.php`),
			newRegexFastReplace(`\\/\\/(?:pu\.vk\.com|[-_a-zA-Z0-9]{1,15}\.(?:userapi\.com|vk-cdn\.net|vk\.me|vkuser(?:live|video|audio)\.(?:net|com)))\\/`,
				func(src, dst []byte, start, end int) []byte {
					if start < 7 || !bytes.Equal(src[start-7:start], jsonUrlPrefix) {
						return append(dst, src[start:end]...)
					}
					dst = append(dst, replacementStart...)
					dst = append(dst, src[start+4:end]...)
					return dst
				}),
			newRegexReplace(`"https:\\/\\/vk\.com\\/(images\\/|doc-?[0-9]+_)`, `"https:\/\/`+domain+`\/_\/vk.com\/$1`),
		}
		cfg.apiOfficialLongpollReplace = newStringReplace(`"server":"api.vk.com\/newuim`, `"server":"`+domain+`\/_\/api.vk.com\/newuim`)
		cfg.apiLongpollReplace = newStringReplace(`"server":"`, `"server":"`+domain+`\/_\/`)
		cfg.siteHlsReplace = newRegexReplace(`https:\/\/([-_a-zA-Z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|vkuser(?:live|video)\.(?:net|com)))\/`, `https://`+domain+`/_/$1/`)
		domains[domain] = cfg
	}
	return cfg
}

func reverseProxyHandler(ctx *fasthttp.RequestCtx) {
	defer func() {
		if r := recover(); r != nil {
			ctx.Logger().Printf("panic when proxying the request: %s", r)
			ctx.Response.Reset()
			ctx.SetStatusCode(500)
			ctx.SetBodyString("500 Internal Server Error")
		}
	}()

	var config *domainConfig
	if Config.domain != "" {
		config = getDomainConfig(Config.domain)
	} else {
		config = getDomainConfig(string(ctx.Host()))
	}

	if !preRequest(ctx) {
		ctx.Response.SetStatusCode(400)
		ctx.Response.SetBodyString("400 Bad Request")
		return
	}

	// In case of redirect
	if ctx.Response.StatusCode() != 200 {
		trackRequest(ctx, 0)
		return
	}

	err := client.DoTimeout(&ctx.Request, &ctx.Response, 30*time.Second)
	if err == nil {
		err = postResponse(config, ctx)
	}

	if err != nil {
		ctx.Logger().Printf("error when proxying the request: %s", err)
		ctx.Response.Reset()
		if strings.Contains(err.Error(), "timed out") || strings.Contains(err.Error(), "timeout") {
			ctx.SetStatusCode(408)
			ctx.SetBodyString("408 Request Timeout")
		} else {
			ctx.SetStatusCode(500)
			ctx.SetBodyString("500 Internal Server Error")
		}
		trackRequest(ctx, 0)
	}
}

func preRequest(ctx *fasthttp.RequestCtx) bool {
	req := &ctx.Request
	path := req.RequestURI()
	if bytes.HasPrefix(path, atPath) {
		slashIndex := bytes.IndexRune(path[1:], '/')
		if slashIndex == -1 {
			return false
		}
		endpoint := []byte(path[4:slashIndex+1])
		if !bytes.Equal(endpoint, siteHost) && !bytes.HasSuffix(endpoint, siteHostRoot) {
			return false
		}
		req.Header.SetHostBytes(endpoint)
		req.SetRequestURIBytes([]byte(path[1+slashIndex:]))
	} else if bytes.HasPrefix(path, awayPath) {
		to := string(req.URI().QueryArgs().Peek("to"))
		if to == "" {
			return false
		}
		to, err := url.QueryUnescape(to)
		if err != nil {
			return false
		}
		ctx.Redirect(to, 301)
		return true
	} else {
		req.SetHostBytes(apiHost)
	}
	// After req.URI() call it is impossible to modify URI
	req.URI().SetSchemeBytes(https)
	if Config.gzipUpstream {
		req.Header.SetBytesKV(acceptEncoding, gzip)
	} else {
		req.Header.DelBytes(acceptEncoding)
	}
	return true
}

func postResponse(config *domainConfig, ctx *fasthttp.RequestCtx) error {
	res := &ctx.Response
	res.Header.DelBytes(setCookie)
	var body []byte
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
	uri := ctx.Request.URI()
	path := uri.Path()

	if bytes.Equal(uri.Host(), siteHost) {
		if bytes.Equal(path, videoHlsPath) {
			body = config.siteHlsReplace.apply(body)
		}
	} else {
		for _, replace := range config.apiReplaces {
			body = replace.apply(body)
		}

		// Replace longpoll server
		if bytes.Equal(path, apiLongpollPath) {
			body = config.apiLongpollReplace.apply(body)
		} else

		// Replace longpoll server for official app
		if bytes.Equal(path, apiOfficialLongpollPath) ||
			bytes.Equal(path, apiOfficialLongpollPath2) ||
			bytes.Equal(path, apiOfficialLongpollPath3) {
			body = config.apiOfficialLongpollReplace.apply(body)
		}

		// Clear feed from SPAM
		if Config.removeAdsFromFeed {
			if bytes.Equal(path, apiOfficialNewsfeedPath) ||
				bytes.Equal(path, apiNewsfeedGet) {
				var parsed map[string]interface{}
				if err := json.Unmarshal(body, &parsed); err == nil {
					if parsed["response"] != nil {
						response := parsed["response"].(map[string]interface{})
						if response["items"] != nil {
							newItems, modified := filterFeed(response["items"].([]interface{}))
							if modified {
								response["items"] = newItems
								body, err = json.Marshal(parsed)
							}
						}
					}
				}
			}
		}
	}
	res.SetBody(body)

	if Config.debug {
		log.Println(string(path) + "\n" + string(body))
	}

	trackRequest(ctx, len(body))
	return nil
}

func filterFeed(items []interface{}) ([]interface{}, bool) {
	removed := 0
	for i := len(items) - 1; i >= 0; i-- {
		post := items[i].(map[string]interface{})
		if post["type"] == "ads" || (post["type"] == "post" && post["marked_as_ads"] != nil && post["marked_as_ads"].(float64) == 1) {
			items[i] = items[len(items)-1]
			items[len(items)-1] = nil
			items = items[:len(items)-1]
			removed++
		}
	}
	if removed > 0 {
		newItems := make([]interface{}, len(items))
		copy(newItems, items)
		return newItems, true
	}
	return nil, false
}
