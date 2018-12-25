package replacer

import (
	"bytes"

	"github.com/json-iterator/go"
)

var json = jsoniter.ConfigFastest

type domainConfig struct {
	apiReplaces                []replace
	apiOfficialLongpollReplace replace
	apiVkmeLongpollReplace     replace
	apiLongpollReplace         replace
	siteHlsReplace             replace
}

type Replacer struct {
	domains map[string]*domainConfig
}

type ReplaceContext struct {
	// vk-proxy domain name
	BaseDomain string

	Domain string
	Path   string

	FilterFeed bool
}

func (r *Replacer) getDomainConfig(domain string) *domainConfig {
	cfg, ok := r.domains[domain]
	if !ok {
		var replacementStart = []byte(`\/\/` + domain + `\/_\/`)
		var jsonUrlPrefix = []byte(`"https:`)
		var mVkCom = []byte(`m.vk.com`)
		cfg = &domainConfig{}
		cfg.apiReplaces = []replace{
			newStringReplace(`"https:\/\/vk.com\/video_hls.php`, `"https:\/\/`+domain+`\/@vk.com\/video_hls.php`),
			newRegexFastReplace(`\\/\\/[-_a-zA-Z0-9]{1,15}\.(?:userapi\.com|vk-cdn\.net|vk\.(?:me|com)|vkuser(?:live|video|audio)\.(?:net|com))\\/`,
				func(src, dst []byte, start, end int) []byte {
					// check if url has valid prefix (like in regexp backreference)
					if start < 7 || !bytes.Equal(src[start-7:start], jsonUrlPrefix) {
						goto cancel
					}
					// do not proxy m.vk.com domain (bugged articles)
					if bytes.Equal(src[start+4:end-2], mVkCom) {
						goto cancel
					}
					dst = append(dst, replacementStart...)
					dst = append(dst, src[start+4:end]...)
					return dst
				cancel:
					return append(dst, src[start:end]...)
				}),
			newRegexReplace(`"https:\\/\\/vk\.com\\/((?:\\/)?images\\/|sticker(:?\\/|s_)|doc-?[0-9]+_)`, `"https:\/\/`+domain+`\/_\/vk.com\/$1`),
		}
		cfg.apiOfficialLongpollReplace = newStringReplace(`"server":"api.vk.com\/newuim`, `"server":"`+domain+`\/_\/api.vk.com\/newuim`)
		cfg.apiVkmeLongpollReplace = newStringReplace(`"server":"api.vk.me\/uim`, `"server":"`+domain+`\/_\/api.vk.me\/uim`)
		cfg.apiLongpollReplace = newStringReplace(`"server":"`, `"server":"`+domain+`\/_\/`)
		cfg.siteHlsReplace = newRegexReplace(`https:\/\/([-_a-zA-Z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|vkuser(?:live|video)\.(?:net|com)))\/`, `https://`+domain+`/_/$1/`)
		if r.domains == nil {
			r.domains = make(map[string]*domainConfig)
		}
		r.domains[domain] = cfg
	}
	return cfg
}

func (r *Replacer) DoReplace(body []byte, ctx ReplaceContext) []byte {
	config := r.getDomainConfig(ctx.BaseDomain)

	if ctx.Domain == "vk.com" {
		if ctx.Path == "/video_hls.php" {
			body = config.siteHlsReplace.apply(body)
		}
	} else {
		for _, replace := range config.apiReplaces {
			body = replace.apply(body)
		}

		// Replace longpoll server
		if ctx.Path == "/method/messages.getLongPollServer" {
			body = config.apiLongpollReplace.apply(body)
		} else

		// Replace longpoll server for official app
		if ctx.Path == "/method/execute" ||
			ctx.Path == "/method/execute.imGetLongPollHistoryExtended" ||
			ctx.Path == "/method/execute.imLpInit" {
			body = config.apiOfficialLongpollReplace.apply(body)
			body = config.apiVkmeLongpollReplace.apply(body)
		}

		if ctx.FilterFeed {
			if ctx.Path == "/method/execute.getNewsfeedSmart" ||
				ctx.Path == "/method/newsfeed.get" {
				var parsed map[string]interface{}
				if err := json.Unmarshal(body, &parsed); err == nil {
					if parsed["response"] != nil {
						response := parsed["response"].(map[string]interface{})
						mod0 := false
						mod := false
						if response["items"] != nil {
							newItems, mod := filterFeed(response["items"].([]interface{}))
							if mod {
								mod0 = true
								response["items"] = newItems
							}
						}
						response, mod = tryInsertPost(response)
						if mod {
							mod0 = true
							parsed["response"] = response
						}
						if mod0 {
							body, err = json.Marshal(parsed)
						}
					}
				}
			}
		}
	}

	return body
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
