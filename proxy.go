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

// Variables for api proxying # api.vk.com
var apiProxy = &fasthttp.HostClient{
	Addr:  "api.vk.com:443",
	IsTLS: true,
}
var apiReplaces []Replace
var apiLongpollPath = []byte("/method/execute")
var apiLongpollReplace Replace
var apiNewsfeedPath = []byte("/method/execute.getNewsfeedSmart")

// Variables for site proxying # vk.com
var siteProxy = &fasthttp.HostClient{
	Addr:  "vk.com:443",
	IsTLS: true,
}
var siteHlsReplace Replace
var sitePath = []byte("/vk.com")
var siteHost = []byte("vk.com")

func InitReplaces(domain string) {
	apiReplaces = []Replace{
		NewReplace(`"https:\\/\\/(pu\.vk\.com|[-a-z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|vkuser(?:live|video|audio)\.(?:net|com)))\\/([^"]+)`, `"https:\/\/`+domain+`\/_\/$1\/$2`),
		NewReplace(`"https:\\/\\/vk\.com\\/(video_hls\.php[^"]+)`, `"https:\/\/`+domain+`\/vk.com\/$1`),
		NewReplace(`"https:\\/\\/vk\.com\\/((images|doc[0-9]+_)[^"]+)`, `"https:\/\/`+domain+`\/_\/vk.com\/$2`),
		NewReplace(`"preview_url":"https:\\/\\/m\.vk\.com\\/(article[0-9]+)[^"]+"(,"preview_page":"[^"]+",?)?`, ``),
	}
	apiLongpollReplace = NewReplace(`"server":"api.vk.com\\/newuim`, `"server":"`+domain+`\/newuim`)

	siteHlsReplace = NewReplace(`https:\/\/([-a-z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|vkuser(?:live|video)\.(?:net|com)))\/`, `https://`+domain+`/_/$1/`)
}

func reverseProxyHandler(ctx *fasthttp.RequestCtx) {
	req := &ctx.Request
	res := &ctx.Response
	proxyClient := preRequest(req)
	if err := proxyClient.Do(req, res); err != nil {
		ctx.Logger().Printf("error when proxying the request: %s", err)
	} else {
		postResponse(req.URI(), res)
	}
}

func preRequest(req *fasthttp.Request) (client *fasthttp.HostClient) {
	trackRequestStart()

	path := req.URI().Path()
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

func postResponse(uri *fasthttp.URI, res *fasthttp.Response) {
	res.Header.Del("Set-Cookie")
	body := res.Body()
	if bytes.Compare(uri.Host(), siteHost) == 0 {
		body = siteHlsReplace.regex.ReplaceAll(body, siteHlsReplace.replace)
	} else {
		for _, replace := range apiReplaces {
			body = replace.regex.ReplaceAll(body, replace.replace)
		}

		if bytes.Compare(uri.Path(), apiLongpollPath) == 0 {
			body = apiLongpollReplace.regex.ReplaceAll(body, apiLongpollReplace.replace)
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

	trackRequestEnd(len(body))
}
