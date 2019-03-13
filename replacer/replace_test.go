package replacer

import (
	"bytes"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/valyala/bytebufferpool"
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
var _regexFuncReplace = newRegexFuncReplace(`\\/\\/[-_a-zA-Z0-9]{1,15}\.(?:userapi\.com|vk-cdn\.net|vk\.(?:me|com)|vkuser(?:live|video|audio)\.(?:net|com))\\/`,
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

func fillPool() {
	for i := 0; i < 100; i++ {
		replaceBufferPool.Put(&bytebufferpool.ByteBuffer{
			B: make([]byte, 1*1024*1024),
		})
	}
}

func getBufferedData() *bytebufferpool.ByteBuffer {
	buffer := replaceBufferPool.Get()
	buffer.Set(rawData)
	return buffer
}

func TestStringReplace(t *testing.T) {
	testStringReplace(t, "test", "test", "bed")
	testStringReplace(t, "testtest", "test", "bed")
	testStringReplace(t, "test2test", "test", "bed")
	testStringReplace(t, "2testtest", "test", "bed")
	testStringReplace(t, "testtest2", "test", "bed")
	testStringReplace(t, "2test2test2", "test", "bed")
	testStringReplace(t, "2test2test2", "test", "beddd")
}

func testStringReplace(t *testing.T, input, needle, replace string) {
	buffer := replaceBufferPool.Get()
	buffer.SetString(input)
	buffer = newStringReplace(needle, replace).apply(buffer)
	if string(buffer.B) != strings.Replace(input, needle, replace, -1) {
		t.Errorf("%s must replaced to '%s' but got '%s'", input, strings.Replace(input, needle, replace, -1), string(buffer.B))
	}
	replaceBufferPool.Put(buffer)
}

func TestRegex(t *testing.T) {
	buffer := getBufferedData()
	buffer = _regexReplace.apply(buffer)
	if !bytes.Equal(buffer.B, replacedData) {
		t.Error("Regex replace is not valid")
	}
	replaceBufferPool.Put(buffer)
}

func TestRegexFunc(t *testing.T) {
	buffer := getBufferedData()
	buffer = _regexFuncReplace.apply(buffer)
	if !bytes.Equal(buffer.B, replacedData) {
		t.Error("FuncRegex replace is not valid")
	}
	replaceBufferPool.Put(buffer)
}

func BenchmarkReplace(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		replaceBufferPool.Put(_regexReplace.apply(getBufferedData()))
	}
}

func BenchmarkReplaceFunc(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		replaceBufferPool.Put(_regexFuncReplace.apply(getBufferedData()))
	}
}

func BenchmarkStringReplace(b *testing.B) {
	b.ReportAllocs()
	needle := []byte(".com")
	replace := []byte("bigstring")
	for i := 0; i < b.N; i++ {
		bytes.Replace(rawData, needle, replace, -1)
	}
}

func BenchmarkMyStringReplace(b *testing.B) {
	b.ReportAllocs()
	replacer := newStringReplace(".com", "bigstring")
	for i := 0; i < b.N; i++ {
		replaceBufferPool.Put(replacer.apply(getBufferedData()))
	}
}
