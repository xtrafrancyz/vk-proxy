package replacer

import (
	"bytes"
	"strings"

	"github.com/json-iterator/go"
	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
	"github.com/xtrafrancyz/vk-proxy/shared"
)

var json = jsoniter.ConfigFastest

type domainConfig struct {
	apiReplaces                []replace
	apiOfficialLongpollReplace replace
	apiVkmeLongpollReplace     replace
	apiLongpollReplace         replace

	siteHlsReplace replace

	headLocationReplace replace

	vkuiLangsHtml replace
	vkuiLangsJs   replace
	vkuiApiJs     replace
}

type Replacer struct {
	ProxyBaseDomain   string
	ProxyStaticDomain string

	config *domainConfig
}

type ReplaceContext struct {
	Method []byte
	Domain string
	Path   string

	FilterFeed bool
}

func (r *Replacer) getDomainConfig() *domainConfig {
	if r.config == nil {
		cfg := &domainConfig{}
		var replacementStart = []byte(`\/\/` + r.ProxyBaseDomain + `\/_\/`)
		var jsonUrlPrefix = []byte(`"https:`)
		cfg = &domainConfig{}
		cfg.apiReplaces = []replace{
			newStringReplace(`"https:\/\/vk.com\/video_hls.php`, `"https:\/\/`+r.ProxyBaseDomain+`\/@vk.com\/video_hls.php`),
			newRegexFuncReplace(`\\/\\/[-_a-zA-Z0-9]{1,15}\.(?:userapi\.com|vk-cdn\.net|vk\.(?:me|com)|vkuser(?:live|video|audio)\.(?:net|com))\\/`,
				func(src, dst []byte, start, end int) []byte {
					// check if url has valid prefix (like in regexp backreference)
					if start < 7 || !bytes.Equal(src[start-7:start], jsonUrlPrefix) {
						goto cancel
					}
					// do not proxy m.vk.com domain (bugged articles)
					if bytes.Equal(src[start+4:end-2], shared.DomainVkMobile) {
						goto cancel
					}
					dst = append(dst, replacementStart...)
					dst = append(dst, src[start+4:end]...)
					return dst
				cancel:
					return append(dst, src[start:end]...)
				}),
			newRegexReplace(`"https:\\/\\/vk\.com\\/((?:\\/)?images\\/|sticker(:?\\/|s_)|doc-?[0-9]+_)`, `"https:\/\/`+r.ProxyBaseDomain+`\/_\/vk.com\/$1`),
		}
		cfg.apiOfficialLongpollReplace = newStringReplace(`"server":"api.vk.com\/newuim`, `"server":"`+r.ProxyBaseDomain+`\/_\/api.vk.com\/newuim`)
		cfg.apiVkmeLongpollReplace = newStringReplace(`"server":"api.vk.me\/uim`, `"server":"`+r.ProxyBaseDomain+`\/_\/api.vk.me\/uim`)
		cfg.apiLongpollReplace = newStringReplace(`"server":"`, `"server":"`+r.ProxyBaseDomain+`\/_\/`)

		cfg.siteHlsReplace = newRegexReplace(`https:\/\/([-_a-zA-Z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|vkuser(?:live|video)\.(?:net|com)))\/`, `https://`+r.ProxyBaseDomain+`/_/$1/`)

		cfg.headLocationReplace = newRegexReplace(`https?://([^/]+)(.*)`, `https://`+r.ProxyBaseDomain+`/@$1$2`)

		cfg.vkuiLangsHtml = newRegexReplace(` src="https://(?:vk.com|'[^']+')/js/vkui_lang`, ` src="https://`+r.ProxyBaseDomain+`/_/vk.com/js/vkui_lang`)
		cfg.vkuiLangsJs = newStringReplace(`langpackEntry:"https://vk.com"`, `langpackEntry:"https://`+r.ProxyBaseDomain+`/_/vk.com"`)
		cfg.vkuiApiJs = newStringReplace(`api.vk.com`, r.ProxyBaseDomain)
		r.config = cfg
	}
	return r.config
}

func (r *Replacer) DoReplaceRequest(req *fasthttp.Request, ctx ReplaceContext) {
	if bytes.Equal(ctx.Method, shared.MethodOptions) {
		// Заменяем заголовок Origin для CORS на заспросах со страниц VKUI
		// api.vk.com принимает только "https://static.vk.com" в заголовке Origin
		// Плюс к этому, если послать некорректный Referer, вк тоже пошлет нас куда подальше
		if origin := req.Header.Peek("Origin"); origin != nil {
			origins := string(origin)
			if strings.HasSuffix(origins, r.ProxyStaticDomain) {
				req.Header.Set("Origin", "https://static.vk.com")
				if referer := req.Header.Peek("Referer"); referer != nil {
					req.Header.Set("Referer", strings.Replace(string(referer), r.ProxyStaticDomain, "static.vk.com", 1))
				}
			}
		}
	}

	if ctx.Domain == "oauth.vk.com" {
		// Для авторизации страницы VKUI используют не уже готовый токен авторизации, а получают его при каждом
		// открытии страницы. В запросе авторизации передается текущий урл страницы VKUI, а так как она проксируется,
		// то она отличается от оригинальной. ВК проверяет этот урл страницы и отвергает авторизацию если он не
		// совпадает.
		// Зачем такие костыли - никто не знает, но нужно их обходить.
		// Если в запросе авторизации используется проксируемый статик домен - заменяем его на оригинальный.
		if strings.HasPrefix(ctx.Path, "/authorize?") {
			uri := fasthttp.AcquireURI()
			uri.Update(ctx.Path)
			args := uri.QueryArgs()
			sourceUrl := args.Peek("source_url")
			if sourceUrl != nil {
				sourceUrls := string(sourceUrl)
				modified := strings.Replace(sourceUrls, r.ProxyStaticDomain, "static.vk.com", 1)
				if modified != sourceUrls {
					args.Set("source_url", modified)
					req.SetRequestURIBytes(uri.RequestURI())
					req.SetHost("oauth.vk.com")
				}
			}
			fasthttp.ReleaseURI(uri)
		}
	}
}

func (r *Replacer) DoReplaceResponse(res *fasthttp.Response, body *bytebufferpool.ByteBuffer, ctx ReplaceContext) *bytebufferpool.ByteBuffer {
	config := r.getDomainConfig()

	if bytes.Equal(ctx.Method, shared.MethodOptions) {
		// Если в ответ на запрос OPTIONS с заданным Origin придет какой-то кривой ответ, то запросы не будут проходить
		if corsOrigin := res.Header.Peek("Access-Control-Allow-Origin"); corsOrigin != nil {
			res.Header.Set("Access-Control-Allow-Origin", "https://"+r.ProxyStaticDomain)
		}
		return body
	}

	if ctx.Domain == "api.vk.com" {
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
				if err := json.Unmarshal(body.B, &parsed); err == nil {
					if parsed["response"] != nil {
						response := parsed["response"].(map[string]interface{})
						modified := tryRemoveAds(response)
						modified = tryInsertPost(response) || modified
						if modified {
							body.B, err = json.Marshal(parsed)
						}
					}
				}
			}
		}

	} else if ctx.Domain == "vk.com" {
		if ctx.Path == "/video_hls.php" {
			body = config.siteHlsReplace.apply(body)
		}

	} else if ctx.Domain == "static.vk.com" {
		if location := res.Header.Peek("Location"); location != nil {
			// Абсолютный редирект на статик меняем на относительный
			locstr := string(location)
			if idx := strings.Index(locstr, "static.vk.com"); idx != -1 {
				relativePath := ctx.Path
				if idx0 := strings.LastIndexByte(relativePath, '/'); idx0 != -1 {
					relativePath = relativePath[:idx0+1]
				}
				relativeRedirectPath := locstr[idx+13 /*static.vk.com*/:]
				relativeRedirectPath = relativeRedirectPath[longestCommonPrefix(relativeRedirectPath, relativePath):]
				res.Header.Set("Location", relativeRedirectPath)
			} else {
				buf := bytebufferpool.Get()
				buf.Set(location)
				buf = config.headLocationReplace.apply(buf)
				res.Header.SetBytesV("Location", buf.Bytes())
			}
		}

		if strings.HasSuffix(ctx.Path, ".js") {
			body = config.vkuiApiJs.apply(body)
			if strings.HasPrefix(ctx.Path, "/community_manage") {
				body = config.vkuiLangsJs.apply(body)
			}
		} else {
			contentType := string(res.Header.ContentType())
			if strings.HasPrefix(contentType, "text/html") {
				body = config.vkuiLangsHtml.apply(body)
			}
		}
	}

	return body
}

func tryRemoveAds(response map[string]interface{}) bool {
	raw, ok := response["items"]
	if !ok {
		return false
	}
	items := raw.([]interface{})
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
		response["items"] = newItems
		return true
	}
	return false
}

func AcquireBuffer() *bytebufferpool.ByteBuffer {
	return replaceBufferPool.Get()
}

func ReleaseBuffer(buffer *bytebufferpool.ByteBuffer) {
	replaceBufferPool.Put(buffer)
}

func longestCommonPrefix(a, b string) (i int) {
	for ; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			break
		}
	}
	return
}
