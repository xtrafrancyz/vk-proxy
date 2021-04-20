package replacer

import (
	_ "embed"
	"io/ioutil"
	"math/rand"
	"net"

	"github.com/phuslu/iploc"
)

type customPost struct {
	probability float64
	profiles    []interface{}
	groups      []interface{}
	items       []interface{}
}

var (
	adPost *customPost

	//go:embed proxy-not-needed.json
	uselessProxyPostData []byte
	uselessProxyPost     *customPost
)

func (p *customPost) apply(response map[string]interface{}, inFront bool) bool {
	if p.probability == 0.0 || rand.Float64() > p.probability {
		return false
	}
	if len(p.profiles) > 0 {
		pp, ok := response["profiles"]
		if !ok {
			return false
		}
		response["profiles"] = append(pp.([]interface{}), p.profiles...)
	}
	if len(p.groups) > 0 {
		pp, ok := response["groups"]
		if !ok {
			return false
		}
		response["groups"] = append(pp.([]interface{}), p.groups...)
	}
	if len(p.items) > 0 {
		pp, ok := response["items"]
		if !ok {
			return false
		}
		if inFront {
			response["items"] = append(p.items, pp.([]interface{})...)
		} else {
			response["items"] = append(pp.([]interface{}), p.items...)
		}
	}
	return true
}

func readCustomPost(bytes []byte) *customPost {
	post := customPost{}
	var parsed map[string]interface{}
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		panic(err)
	}
	parsed = parsed["response"].(map[string]interface{})

	post.probability = parsed["probability"].(float64)
	if pp, ok := parsed["profiles"]; ok {
		post.profiles = pp.([]interface{})
	}
	if pp, ok := parsed["groups"]; ok {
		post.groups = pp.([]interface{})
	}
	if pp, ok := parsed["items"]; ok {
		post.items = pp.([]interface{})
	}
	return &post
}

func init() {
	uselessProxyPost = readCustomPost(uselessProxyPostData)
	if bytes, err := ioutil.ReadFile("newsfeed.json"); err == nil {
		adPost = readCustomPost(bytes)
	}
}

func tryInsertAdPost(response map[string]interface{}) bool {
	return adPost.apply(response, false)
}

func tryInsertUselessProxyPost(response map[string]interface{}, ctx *ReplaceContext) bool {
	realIp := ctx.RequestCtx.Request.Header.Peek("X-Real-IP")
	if len(realIp) == 0 {
		return false
	}

	if ip := net.ParseIP(string(realIp)); ip != nil {
		country := string(iploc.Country(ip))
		if country == "RU" {
			args := ctx.RequestCtx.Request.PostArgs()
			if startFrom := args.Peek("start_from"); len(startFrom) == 1 && startFrom[0] == '0' {
				return uselessProxyPost.apply(response, true)
			}
		}
	}
	return false
}
