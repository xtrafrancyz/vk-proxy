package main

import (
	"bytes"
	"regexp"
	"errors"
	"encoding/json"

	"github.com/valyala/fasthttp"
)

type replace interface {
	apply(data []byte) []byte
}

type regexReplace struct {
	regex       *regexp.Regexp
	replacement []byte
}

func newRegexReplace(regex, replace string) *regexReplace {
	return &regexReplace{
		regex:       regexp.MustCompile(regex),
		replacement: []byte(replace),
	}
}

func (v *regexReplace) apply(data []byte) []byte {
	return v.regex.ReplaceAll(data, v.replacement)
}

type stringReplace struct {
	needle      []byte
	replacement []byte
}

func newStringReplace(needle, replace string) *stringReplace {
	return &stringReplace{
		needle:      []byte(needle),
		replacement: []byte(replace),
	}
}

func (v *stringReplace) apply(data []byte) []byte {
	return bytes.Replace(data, v.needle, v.replacement, -1)
}

type DomainConfig struct {
	apiReplaces                []replace
	apiOfficialLongpollReplace replace
	apiLongpollReplace         replace
	siteHlsReplace             replace
}

// Constants
var (
	apiOfficialLongpollPath = []byte("/method/execute")
	apiOfficialNewsfeedPath = []byte("/method/execute.getNewsfeedSmart")
	apiLongpollPath         = []byte("/method/messages.getLongPollServer")
	videoHlsPath            = []byte("/video_hls.php")
	atPath                  = []byte("/%40")
	https                   = []byte("https")
	apiHost                 = []byte("api.vk.com")
	siteHost                = []byte("vk.com")
	siteHostRoot            = []byte(".vk.com")
)

var client = &fasthttp.Client{}
var domains = make(map[string]*DomainConfig)

func getDomainConfig(domain string) *DomainConfig {
	cfg, ok := domains[domain]
	if !ok {
		cfg = &DomainConfig{}
		cfg.apiReplaces = []replace{
			newRegexReplace(`"https:\\/\\/(pu\.vk\.com|[-a-z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|vkuser(?:live|video|audio)\.(?:net|com)))\\/([^"]+)`, `"https:\/\/`+domain+`\/_\/$1\/$2`),
			newStringReplace(`"https:\/\/vk\.com\/video_hls.php`, `"https:\/\/`+domain+`\/@vk.com\/video_hls.php`),
			newRegexReplace(`"https:\\/\\/vk\.com\\/(?:images|doc[0-9]+_)`, `"https:\/\/`+domain+`\/_\/vk.com\/$1`),
		}
		cfg.apiOfficialLongpollReplace = newStringReplace(`"server":"api.vk.com\/newuim`, `"server":"`+domain+`\/newuim`)
		cfg.apiLongpollReplace = newStringReplace(`"server":"`, `"server":"`+domain+`\/@`)
		cfg.siteHlsReplace = newRegexReplace(`https:\/\/([-a-z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|vkuser(?:live|video)\.(?:net|com)))\/`, `https://`+domain+`/_/$1/`)
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
			ctx.SetBodyString("500 Internal error")
		}
	}()

	var config *DomainConfig
	if Config.domain != "" {
		config = getDomainConfig(Config.domain)
	} else {
		config = getDomainConfig(string(ctx.Host()))
	}

	req := &ctx.Request
	if err := preRequest(req); err != nil {
		ctx.Response.BodyWriter().Write([]byte(err.Error()))
		ctx.Response.SetStatusCode(500)
		return
	}
	if err := client.Do(req, &ctx.Response); err != nil {
		ctx.Logger().Printf("error when proxying the request: %s", err)
		trackRequest(ctx, 0)
	} else {
		postResponse(config, ctx)
	}
}

func preRequest(req *fasthttp.Request) error {
	path := req.RequestURI()
	if bytes.HasPrefix(path, atPath) {
		slashIndex := bytes.IndexRune(path[1:], '/')
		if slashIndex == -1 {
			return errors.New("bad request")
		}
		endpoint := []byte(path[4:slashIndex+1])
		if !bytes.Equal(endpoint, siteHost) && !bytes.HasSuffix(endpoint, siteHostRoot) {
			return errors.New("bad endpoint")
		}
		req.Header.SetHostBytes(endpoint)
		req.SetRequestURIBytes([]byte(path[1+slashIndex:]))
	} else {
		req.SetHostBytes(apiHost)
	}
	// After req.URI() call it is impossible to modify URI
	req.URI().SetSchemeBytes(https)
	req.Header.Del("Accept-Encoding")
	return nil
}

func postResponse(config *DomainConfig, ctx *fasthttp.RequestCtx) {
	uri := ctx.Request.URI()
	res := &ctx.Response
	res.Header.Del("Set-Cookie")
	body := res.Body()

	if bytes.Equal(uri.Host(), siteHost) {
		if bytes.Equal(uri.Path(), videoHlsPath) {
			body = config.siteHlsReplace.apply(body)
		}
	} else {
		for _, replace := range config.apiReplaces {
			body = replace.apply(body)
		}

		// Replace longpoll server
		if bytes.Equal(uri.Path(), apiLongpollPath) {
			body = config.apiLongpollReplace.apply(body)
		} else

		// Replace longpoll server for official app
		if bytes.Equal(uri.Path(), apiOfficialLongpollPath) {
			body = config.apiOfficialLongpollReplace.apply(body)
		} else

		// Clear feed from SPAM
		if bytes.Equal(uri.Path(), apiOfficialNewsfeedPath) {
			var parsed map[string]interface{}
			if err := json.Unmarshal(body, &parsed); err == nil {
				removed := 0
				if parsed["response"] != nil {
					response := parsed["response"].(map[string]interface{})
					if response["items"] != nil {
						items := response["items"].([]interface{})
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
							response["items"] = newItems
						}
					}
				}
				if removed > 0 {
					body, err = json.Marshal(parsed)
				}
			}
		}
	}
	res.SetBody(body)

	trackRequest(ctx, len(body))
}
