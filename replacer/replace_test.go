package replacer

import (
	"bytes"
	"io/ioutil"
	"testing"
)

const domain = "vk-api-proxy.xtrafrancyz.net"

var rawData, _ = ioutil.ReadFile("test/raw-data.json")
var replacedData, _ = ioutil.ReadFile("test/replaced-data.json")
var replacementStart = []byte(`\/\/` + domain + `\/_\/`)
var jsonUrlPrefix = []byte(`"https:`)
var mVkCom = []byte(`m.vk.com`)

var _regexReplace = newRegexReplace(
	`"https:\\/\\/(pu\.vk\.com|[-_a-zA-Z0-9]+\.(?:userapi\.com|vk-cdn\.net|vk\.me|vkuser(?:live|video|audio)\.(?:net|com)))\\/`,
	`"https:\/\/`+domain+`\/_\/$1\/`,
)
var _regexFastReplace = newRegexFastReplace(`\\/\\/[-_a-zA-Z0-9]{1,15}\.(?:userapi\.com|vk-cdn\.net|vk\.(?:me|com)|vkuser(?:live|video|audio)\.(?:net|com))\\/`,
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
	},
)

func TestRegex(t *testing.T) {
	if !bytes.Equal(_regexReplace.apply(rawData), replacedData) {
		t.Error("Regex replace is not valid")
	}
}

func TestRegexFast(t *testing.T) {
	if !bytes.Equal(_regexFastReplace.apply(rawData), replacedData) {
		t.Error("FastRegex replace is not valid")
	}
}

func BenchmarkReplace(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_regexReplace.apply(rawData)
	}
}

func BenchmarkReplaceFast(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_regexFastReplace.apply(rawData)
	}
}
