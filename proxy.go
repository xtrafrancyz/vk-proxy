package main

import (
	"bytes"
	"regexp"
	"encoding/json"

	"github.com/valyala/fasthttp"
)

type Replace struct {
	regex   *regexp.Regexp
	replace []byte
}

func NewReplace(regex, replace string) (result Replace) {
	result.regex = regexp.MustCompile(regex)
	result.replace = []byte(replace)
	return
}

func (v *Replace) apply(data []byte) []byte {
	return v.regex.ReplaceAll(data, v.replace)
}

type DomainConfig struct {
	apiReplaces        []Replace
	apiLongpollReplace Replace
	siteHlsReplace     Replace
}

// Variables for api proxying # api.vk.com
var apiProxy = &fasthttp.HostClient{
	Addr:  "api.vk.com:443",
	IsTLS: true,
}
var apiLongpollPath = []byte("/method/execute")
var apiNewsfeedPath = []byte("/method/execute.getNewsfeedSmart")

// Variables for site proxying # vk.com
var siteProxy = &fasthttp.HostClient{
	Addr:  "vk.com:443",
	IsTLS: true,
}
var sitePath = []byte("/vk.com")
var siteHost = []byte("vk.com")

var domains = make(map[string]DomainConfig)

func getDomainConfig(domain string) DomainConfig {
	cfg, ok := domains[domain]
	if !ok {
		cfg = DomainConfig{}
		cfg.apiReplaces = []Replace{
			NewReplace(`"https:\\/\\/(pu\.vk\.com|[-a-z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|vkuser(?:live|video|audio)\.(?:net|com)))\\/([^"]+)`, `"https:\/\/`+domain+`\/_\/$1\/$2`),
			NewReplace(`"https:\\/\\/vk\.com\\/(video_hls\.php[^"]+)`, `"https:\/\/`+domain+`\/vk.com\/$1`),
			NewReplace(`"https:\\/\\/vk\.com\\/((images|doc[0-9]+_)[^"]+)`, `"https:\/\/`+domain+`\/_\/vk.com\/$1`),
			NewReplace(`"preview_page":"[0-9_]",?`, ``),
		}
		cfg.apiLongpollReplace = NewReplace(`"server":"api.vk.com\\/newuim`, `"server":"`+domain+`\/newuim`)
		cfg.siteHlsReplace = NewReplace(`https:\/\/([-a-z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|vkuser(?:live|video)\.(?:net|com)))\/`, `https://`+domain+`/_/$1/`)
		domains[domain] = cfg
	}
	return cfg
}

func reverseProxyHandler(ctx *fasthttp.RequestCtx) {
	var config DomainConfig
	if Config.domain != "" {
		config = getDomainConfig(Config.domain)
	} else {
		config = getDomainConfig(string(ctx.Host()))
	}

	req := &ctx.Request
	proxyClient := preRequest(req)
	if err := proxyClient.Do(req, &ctx.Response); err != nil {
		ctx.Logger().Printf("error when proxying the request: %s", err)
		trackRequest(ctx, 0)
	} else {
		postResponse(config, ctx)
	}
}

func preRequest(req *fasthttp.Request) (client *fasthttp.HostClient) {
	path := req.RequestURI()
	if bytes.HasPrefix(req.URI().Path(), sitePath) {
		client = siteProxy
		req.SetRequestURIBytes([]byte(path[7:]))
		req.SetHost("vk.com")
	} else {
		client = apiProxy
		req.SetHost("api.vk.com")
	}
	req.Header.Del("Accept-Encoding")
	return
}

func postResponse(config DomainConfig, ctx *fasthttp.RequestCtx) {
	uri := ctx.Request.URI()
	res := &ctx.Response
	res.Header.Del("Set-Cookie")
	body := res.Body()
	if bytes.Compare(uri.Host(), siteHost) == 0 {
		body = config.siteHlsReplace.apply(body)
	} else {
		for _, replace := range config.apiReplaces {
			body = replace.apply(body)
		}

		if bytes.Compare(uri.Path(), apiLongpollPath) == 0 {
			body = config.apiLongpollReplace.apply(body)
		} else

		// Clear feed from SPAM
		if bytes.Compare(uri.Path(), apiNewsfeedPath) == 0 {
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
