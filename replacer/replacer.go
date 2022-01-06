package replacer

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/json-iterator/go"
	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
	"github.com/xtrafrancyz/vk-proxy/replacer/hardcode"
	"github.com/xtrafrancyz/vk-proxy/replacer/x"
)

var (
	json = jsoniter.ConfigFastest

	httpsStr         = []byte("https:")
	indexM3u8Str     = []byte("/index.m3u8")
	methodOptionsStr = []byte("OPTIONS")
	methodPostStr    = []byte("POST")
)

type domainConfig struct {
	apiGlobalReplace           x.Replace
	apiOfficialLongpollReplace x.Replace
	apiVkmeLongpollReplace     x.Replace
	apiLongpollReplace         x.Replace

	hlsReplace      x.Replace
	m3u8Replace     x.Replace
	m3u8PathReplace *regexFuncReplace

	headLocationReplace x.Replace

	vkuiLangsHtml x.Replace
	vkuiApiJs     x.Replace
}

type Replacer struct {
	ProxyBaseDomain        string
	ProxyStaticDomain      string
	FilterFeed             bool
	AddUselessProxyMessage bool

	config *domainConfig
}

type ReplaceContext struct {
	RequestCtx *fasthttp.RequestCtx
	Method     []byte
	OriginHost string
	Host       string
	Path       string
}

func (c *ReplaceContext) Reset() {
	c.RequestCtx = nil
	c.Method = nil
	c.OriginHost = ""
	c.Host = ""
	c.Path = ""
}

func (r *Replacer) getDomainConfig() *domainConfig {
	if r.config == nil {
		cfg := &domainConfig{}
		cfg.apiGlobalReplace = hardcode.NewHardcodedDomainReplace(hardcode.HardcodedDomainReplaceConfig{
			Pool:          &replaceBufferPool,
			SimpleReplace: r.ProxyBaseDomain + `\/_\/`,
			SmartReplace:  r.ProxyBaseDomain + `\/@`,
		})
		cfg.apiOfficialLongpollReplace = newStringReplace(`"server":"api.vk.com\/`, `"server":"`+r.ProxyBaseDomain+`\/@api.vk.com\/`)
		cfg.apiVkmeLongpollReplace = newStringReplace(`"server":"api.vk.me\/`, `"server":"`+r.ProxyBaseDomain+`\/@api.vk.me\/`)
		cfg.apiLongpollReplace = newStringReplace(`"server":"`, `"server":"`+r.ProxyBaseDomain+`\/@`)

		cfg.hlsReplace = newRegexReplace(`https:\/\/([-_a-zA-Z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|mycdn\.me|vkuser(?:live|video)\.(?:net|com)))\/`, `https://`+r.ProxyBaseDomain+`/_/$1/`)
		cfg.m3u8Replace = newRegexReplace(`https:\/\/([-_a-zA-Z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|mycdn\.me|vkuseraudio\.(?:net|com)))\/`, `https://`+r.ProxyBaseDomain+`/_/$1/`)
		cfg.m3u8PathReplace = &regexFuncReplace{
			regex: regexp.MustCompile(`(?mi)^[a-z0-9_-]`),
		}

		cfg.headLocationReplace = newRegexReplace(`^https?://([^/]+)(.*)`, `https://`+r.ProxyBaseDomain+`/@$1$2`)

		cfg.vkuiLangsHtml = newStringReplace(`https://vk.com/js/vkui_lang.js`, `https://`+r.ProxyBaseDomain+`/_/vk.com/js/vkui_lang.js`)
		cfg.vkuiApiJs = newStringReplace(`api.vk.com`, r.ProxyBaseDomain)
		r.config = cfg
	}
	return r.config
}

func (r *Replacer) DoReplaceRequest(req *fasthttp.Request, ctx *ReplaceContext) {
	if bytes.Equal(ctx.Method, methodOptionsStr) {
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

	// UPD 22.08.2021 - Пока не будет выяснен способ получения secret для генерации подписи, этот код не имеет смысла
	// UPD 17.11.2021 (YTKAB0BP) - Похоже, sig больше не используется в новых версиях VK, поэтому можно использовать замену без него
	if ctx.Host == "oauth.vk.com" {
		// Для авторизации страницы VKUI используют не уже готовый токен авторизации, а получают его при каждом
		// открытии страницы. В запросе авторизации передается текущий урл страницы VKUI, а так как она проксируется,
		// то она отличается от оригинальной. ВК проверяет этот урл страницы и отвергает авторизацию если он не
		// совпадает.
		// Зачем такие костыли - никто не знает, но нужно их обходить.
		// Если в запросе авторизации используется проксируемый статик домен - заменяем его на оригинальный.
		if ctx.Path == "/authorize" {
			var args *fasthttp.Args
			if bytes.Equal(ctx.Method, methodPostStr) {
				args = req.PostArgs()
			} else {
				args = req.URI().QueryArgs()
			}

			if args.Peek("sig") == nil {
				dirty := false

				sourceUrl := args.Peek("source_url")
				if sourceUrl != nil {
					sourceUrls := string(sourceUrl)
					modified := strings.Replace(sourceUrls, r.ProxyStaticDomain, "static.vk.com", 1)
					if modified != sourceUrls {
						args.Set("source_url", modified)
						dirty = true
					}
				}
				redirectUri := args.Peek("redirect_uri")
				if redirectUri != nil {
					redirectUris := string(redirectUri)
					modified := strings.Replace(redirectUris, ctx.OriginHost, "oauth.vk.com", 1)
					if modified != redirectUris {
						args.Set("redirect_uri", modified)
						dirty = true
					}
				}

				// Для изменения PostArgs нужно вручную вставить их в боди или очистить боди
				if dirty && bytes.Equal(ctx.Method, methodPostStr) {
					req.SetBody(args.QueryString())
				}
			}
		}
	}
}

func (r *Replacer) DoReplaceResponse(res *fasthttp.Response, body *bytebufferpool.ByteBuffer, ctx *ReplaceContext) *bytebufferpool.ByteBuffer {
	config := r.getDomainConfig()

	if bytes.Equal(ctx.Method, methodOptionsStr) {
		// Если в ответ на запрос OPTIONS с заданным Origin придет какой-то кривой ответ, то запросы не будут проходить
		if corsOrigin := res.Header.Peek("Access-Control-Allow-Origin"); corsOrigin != nil {
			res.Header.Set("Access-Control-Allow-Origin", "https://"+r.ProxyStaticDomain)
		}
		return body
	}

	if ctx.Host == "api.vk.com" {
		body = config.apiGlobalReplace.Apply(body)

		// Replace longpoll server
		if ctx.Path == "/method/messages.getLongPollServer" {
			body = config.apiLongpollReplace.Apply(body)
		} else

		// Replace longpoll server for official app
		if ctx.Path == "/method/execute" ||
			ctx.Path == "/method/execute.imGetLongPollHistoryExtended" ||
			ctx.Path == "/method/execute.imLpInit" {
			body = config.apiOfficialLongpollReplace.Apply(body)
			body = config.apiVkmeLongpollReplace.Apply(body)
		}

		if r.FilterFeed {
			if ctx.Path == "/method/execute.getNewsfeedSmart" ||
				ctx.Path == "/method/newsfeed.get" {
				var parsed map[string]interface{}
				if err := json.Unmarshal(body.B, &parsed); err == nil {
					if parsed["response"] != nil {
						response := parsed["response"].(map[string]interface{})
						modified := tryRemoveAds(response)
						modified = tryInsertAdPost(response) || modified
						if r.AddUselessProxyMessage {
							modified = tryInsertUselessProxyPost(response, ctx) || modified
						}
						if modified {
							body.B, err = json.Marshal(parsed)
						}
					}
				}
			}
		}

	} else if ctx.Host == "vk.com" {
		if ctx.Path == "/video_hls.php" {
			body = config.hlsReplace.Apply(body)
		} else if ctx.Path == "/err404.php" {
			if location := res.Header.Peek("Location"); location != nil {
				// Если редирект идет на .m3u8, то редиректим на прокси с заменой
				if bytes.Contains(location, indexM3u8Str) {
					replaceLocationHeader(config, location, res)
				}
			}
		}

	} else if ctx.Host == "static.vk.com" {
		if location := res.Header.Peek("Location"); location != nil {
			// Абсолютный редирект на статик меняем на относительный
			locstr := string(location)
			if idx := strings.Index(locstr, "static.vk.com"); idx != -1 {
				relativePath := ctx.Path
				if idx0 := strings.LastIndexByte(relativePath, '/'); idx0 != -1 {
					relativePath = relativePath[:idx0+1]
				}
				relativeRedirectPath := locstr[idx+13: /*static.vk.com*/]
				relativeRedirectPath = relativeRedirectPath[longestCommonPrefix(relativeRedirectPath, relativePath):]
				res.Header.Set("Location", relativeRedirectPath)
			} else {
				replaceLocationHeader(config, location, res)
			}
		}

		if strings.HasSuffix(ctx.Path, ".js") {
			body = config.vkuiApiJs.Apply(body)
		} else {
			contentType := string(res.Header.ContentType())
			if strings.HasPrefix(contentType, "text/html") {
				body = config.vkuiLangsHtml.Apply(body)
			}
		}
	} else if strings.HasSuffix(ctx.Host, ".vkuseraudio.net") || strings.HasSuffix(ctx.Host, ".vkuseraudio.com") {
		if strings.HasSuffix(ctx.Path, ".m3u8") {
			if location := res.Header.Peek("Location"); location != nil {
				replaceLocationHeader(config, location, res)
			} else {
				body = config.m3u8Replace.Apply(body)
				absolutePath := "https://" + r.ProxyBaseDomain + "/_/" + ctx.Host + ctx.Path[:strings.LastIndexByte(ctx.Path, '/')+1]
				body = config.m3u8PathReplace.ApplyFunc(body, func(src, dst []byte, start, end int) []byte {
					// Пропускаем если это абсолютная ссылка
					if end+5 > cap(src) || bytes.Equal(src[start:end+5], httpsStr) {
						goto cancel
					}
					return append(append(dst, absolutePath...), src[start:end]...)
				cancel:
					return append(dst, src[start:end]...)
				})
			}
		}
	} else if strings.HasSuffix(ctx.Host, ".mycdn.me") {
		if strings.HasSuffix(ctx.Path, ".m3u8") {
			if location := res.Header.Peek("Location"); location != nil {
				replaceLocationHeader(config, location, res)
			} else {
				body = config.m3u8Replace.Apply(body)
			}
		}
	} else if ctx.Host == "oauth.vk.com" {
		if ctx.Path == "/token" {
			body = config.apiGlobalReplace.Apply(body)
		}

		// При авторизации из VKUI порядок запросов следующий:
		// 1. /authorize с подписью от приложения
		// 2. Редирект на /auth_by_token
		// 3. Редирект на oauth.vk.com/blank.html, который мы и заменяем на прокси домен
		//
		// В официальных приложениях вк проверяется редирект и если он == "oauth.vk.com/blank.html", то авторизация успешная.
		// В модах все упоминания oauth.vk.com заменяются на прокси домен, следовательно нам нужно подставлять туда
		// наш домен.
		if ctx.Path == "/auth_by_token" {
			if location := res.Header.Peek("Location"); location != nil {
				res.Header.Set(
					"Location",
					strings.Replace(string(location), "oauth.vk.com", ctx.OriginHost, 1),
				)
			}
		}
	}

	return body
}

func replaceLocationHeader(config *domainConfig, location []byte, res *fasthttp.Response) {
	// Заменяем абсолютные редиректы на прокси с заменой
	buf := AcquireBuffer()
	buf.Set(location)
	buf = config.headLocationReplace.Apply(buf)
	res.Header.SetBytesV("Location", buf.Bytes())
	ReleaseBuffer(buf)
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
