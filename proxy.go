package main

import (
	"bytes"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/json-iterator/go"
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
	apiOfficialLongpollPath  = []byte("/method/execute")
	apiOfficialLongpollPath2 = []byte("/method/execute.imGetLongPollHistoryExtended")
	apiOfficialLongpollPath3 = []byte("/method/execute.imLpInit")
	apiOfficialNewsfeedPath  = []byte("/method/execute.getNewsfeedSmart")
	apiLongpollPath          = []byte("/method/messages.getLongPollServer")
	apiNewsfeedGet           = []byte("/method/newsfeed.get")
	videoHlsPath             = []byte("/video_hls.php")
	atPath                   = []byte("/%40")
	awayPath                 = []byte("/away")
	https                    = []byte("https")
	apiHost                  = []byte("api.vk.com")
	siteHost                 = []byte("vk.com")
	siteHostRoot             = []byte(".vk.com")
)

var client = &fasthttp.Client{
	Name: "vk-proxy",
}
var domains = make(map[string]*DomainConfig)
var json = jsoniter.ConfigFastest

func getDomainConfig(domain string) *DomainConfig {
	cfg, ok := domains[domain]
	if !ok {
		cfg = &DomainConfig{}
		cfg.apiReplaces = []replace{
			newStringReplace(`"https:\/\/vk.com\/video_hls.php`, `"https:\/\/`+domain+`\/@vk.com\/video_hls.php`),
			newRegexReplace(`"https:\\/\\/([-_a-zA-Z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.(?:me|com)|vkuser(?:live|video|audio)\.(?:net|com)))\\/`, `"https:\/\/`+domain+`\/_\/$1\/`),
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

	var config *DomainConfig
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

	if err := client.DoTimeout(&ctx.Request, &ctx.Response, 30*time.Second); err != nil {
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
	} else {
		postResponse(config, ctx)
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
	req.Header.Del("Accept-Encoding")
	return true
}

func postResponse(config *DomainConfig, ctx *fasthttp.RequestCtx) {
	uri := ctx.Request.URI()
	res := &ctx.Response
	res.Header.Del("Set-Cookie")
	body := res.Body()
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
