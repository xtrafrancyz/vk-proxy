package hardcode

import (
	"bytes"
	"sync"
	"unicode/utf8"

	"github.com/valyala/bytebufferpool"
)

var (
	userapiStr            = []byte("userapi")
	vkuserStr             = []byte("vkuser")
	vkcdnStr              = []byte("vk-cdn")
	vkStr                 = []byte("vk")
	mycdnStr              = []byte("mycdn")
	comStr                = []byte("com")
	netStr                = []byte("net")
	meStr                 = []byte("me")
	videoStr              = []byte("video")
	audioStr              = []byte("audio")
	liveStr               = []byte("live")
	m3u8Str               = []byte(".m3u8")
	escapedSlashStr       = []byte(`\/`)
	escapedDoubleSlashStr = []byte(`\/\/`)
	jsonHttpsStr          = []byte(`"https:`)
	docPathStr            = []byte(`doc`)
	imagesPathStr         = []byte(`images\/`)
	imagesPath2Str        = []byte(`\/images\/`)
	stickerPathStr        = []byte(`sticker`)
	stickersPathEndingStr = []byte(`s_`)
	videoHlsStr           = []byte(`video_hls.php`)

	domainChars = func() (as asciiSet) {
		for i := 'a'; i <= 'z'; i++ {
			as.add(byte(i))
		}
		for i := 'A'; i <= 'Z'; i++ {
			as.add(byte(i))
		}
		for i := '0'; i <= '9'; i++ {
			as.add(byte(i))
		}
		as.add(byte('-'))
		as.add(byte('_'))
		return
	}()

	insertionsPool = sync.Pool{New: func() interface{} {
		i := make([]insertion, 0, 32)
		return &i
	}}

	parsedUriPool = sync.Pool{New: func() interface{} {
		return &parsedUri{}
	}}
)

const (
	maxDomainPartLen      = 15
	escapedDoubleSlashLen = 4
	jsonHttpsLen          = 7
)

type HardcodedDomainReplaceConfig struct {
	Pool *bytebufferpool.Pool

	// Домен, который просто пропускает трафик через себя без обработки, обычно domain.com\/_\/
	SimpleReplace string

	// Домен для замены с обработкой, обычно domain.com\/@
	SmartReplace string
}

type hardcodedDomainReplace struct {
	pool   *bytebufferpool.Pool
	simple []byte
	smart  []byte
}

type insertion struct {
	offset  int
	content []byte
}

type parsedUri struct {
	host [][]byte
}

func (u *parsedUri) prepareHost() [][]byte {
	if u.host == nil {
		u.host = make([][]byte, 3, 3)
	} else {
		u.host = u.host[:3]
	}
	return u.host
}

func (u *parsedUri) getPath(s []byte, offset int) []byte {
	if offset+5 > len(s) || s[offset] != '\\' {
		return nil
	}
	return s[offset+2:]
}

// Этот реплейс работает абсолютно так же, как вместе взятые следующие регулярки:
// - "https:\\/\\/[-_a-zA-Z0-9]{1,15}\.(?:userapi\.com|vk-cdn\.net|vk\.(?:me|com)|vkuser(?:live|video|audio)\.(?:net|com))\\/
//    -> "https:\/\/proxy_domain\/_\/$1\/
// - "https:\/\/vk.com\/video_hls.php
//    -> "https:\/\/proxy_domain\/@vk.com\/video_hls.php
// - "https:\\/\\/vk\.com\\/((?:\\/)?images\\/|sticker(:?\\/|s_)|doc-?[0-9]+_)
//    -> "https:\/\/proxy_domain\/_\/vk.com\/$1
func NewHardcodedDomainReplace(config HardcodedDomainReplaceConfig) *hardcodedDomainReplace {
	v := &hardcodedDomainReplace{
		pool:   config.Pool,
		simple: []byte(config.SimpleReplace),
		smart:  []byte(config.SmartReplace),
	}
	return v
}

func (v *hardcodedDomainReplace) Apply(input *bytebufferpool.ByteBuffer) *bytebufferpool.ByteBuffer {
	// Быстрый путь для ответов без ссылок
	index := bytes.Index(input.B, escapedDoubleSlashStr)
	if index == -1 {
		return input
	}

	offset := index
	inputLen := len(input.B)

	uri := parsedUriPool.Get().(*parsedUri)
	_insertion := insertionsPool.Get().(*[]insertion)
	insertions := *_insertion
	defer func() {
		*_insertion = insertions[:0]
		insertionsPool.Put(_insertion)
	}()

	for {
		index = bytes.Index(input.B[offset:], escapedDoubleSlashStr)
		if index == -1 {
			break
		}
		match := offset + index
		offset += index + escapedDoubleSlashLen
		if offset+5 > inputLen {
			break
		}

		// Проверка на то что ссылка начинается с http и стоит в начале json строки
		if match < jsonHttpsLen || !bytes.Equal(input.B[match-jsonHttpsLen:match], jsonHttpsStr) {
			continue
		}

		// Чтение домена
		domainLength := bytes.Index(input.B[offset:min(inputLen, offset+maxDomainPartLen*3)], escapedSlashStr)
		if domainLength == -1 {
			continue
		}

		// Проверка домена на допустимые символы
		uri.host = split(input.B[offset:offset+domainLength], '.', uri.prepareHost())
		for _, part := range uri.host {
			if len(part) > maxDomainPartLen || !testDomainPart(part) {
				continue
			}
		}

		ins := v.simple
		// Проверка что домен можно проксировать
		if len(uri.host) == 2 {
			if bytes.Equal(uri.host[0], vkStr) && bytes.Equal(uri.host[1], comStr) { // vk.com
				path := uri.getPath(input.B, offset+domainLength)
				if bytes.HasPrefix(path, docPathStr) { // vk.com/doc[-0-9]*
					c := path[len(docPathStr)]
					if c != '-' && !(c >= '0' && c <= '9') {
						continue
					}
				} else if bytes.HasPrefix(path, imagesPathStr) || bytes.HasPrefix(path, imagesPath2Str) { // vk.com//?images/*
					// allow
				} else if bytes.HasPrefix(path, stickerPathStr) { // vk.com/sticker*
					path2 := path[len(stickerPathStr):]
					if !bytes.HasPrefix(path2, escapedSlashStr) && // vk.com/sticker/*
						!bytes.HasPrefix(path2, stickersPathEndingStr) { // vk.com/stickers_
						continue
					}
				} else if bytes.HasPrefix(path, videoHlsStr) {
					ins = v.smart
				} else {
					continue
				}
			} else {
				continue
			}
		} else if len(uri.host) == 3 {
			if bytes.Equal(uri.host[1], userapiStr) { // *.userapi.com
				if !bytes.Equal(uri.host[2], comStr) {
					continue
				}
			} else if bytes.Equal(uri.host[1], vkcdnStr) { // *.vk-cdn.net
				if !bytes.Equal(uri.host[2], netStr) {
					continue
				}
			} else if bytes.Equal(uri.host[1], mycdnStr) { // *.mycdn.me
				if bytes.Equal(uri.host[2], meStr) {
					path := uri.getPath(input.B, offset+domainLength)
					if bytes.Contains(path[:min(100, len(path))], m3u8Str) {
						ins = v.smart
					}
				} else {
					continue
				}
			} else if bytes.Equal(uri.host[1], vkStr) { // *.vk.*
				if bytes.Equal(uri.host[2], comStr) { // *.vk.com
					// Домен m.vk.com не проксим, все остальные *.vk.com проксятся
					if len(uri.host[0]) == 1 && uri.host[0][0] == 'm' {
						continue
					}
				} else {
					continue
				}
			} else if bytes.Equal(uri.host[1], vkuserStr) { // *.vkuser.net
				if !bytes.Equal(uri.host[2], netStr) {
					continue
				}
			} else if bytes.HasPrefix(uri.host[1], vkuserStr) { // *.vkuser(audio|video|live).(net|com)
				if !bytes.Equal(uri.host[2], comStr) && !bytes.Equal(uri.host[2], netStr) {
					continue
				}
				r := uri.host[1][len(vkuserStr):]
				if bytes.Equal(r, audioStr) {
					path := uri.getPath(input.B, offset+domainLength)
					if bytes.Contains(path[:min(100, len(path))], m3u8Str) {
						ins = v.smart
					}
				} else if !bytes.Equal(r, videoStr) && !bytes.Equal(r, liveStr) {
					continue
				}
			} else {
				continue
			}
		} else {
			continue
		}
		insertions = append(insertions, insertion{
			offset:  offset,
			content: ins,
		})
	}
	parsedUriPool.Put(uri)

	if len(insertions) == 0 {
		return input
	}

	neededLength := inputLen
	for _, ins := range insertions {
		neededLength += len(ins.content)
	}

	output := v.pool.Get()
	if cap(output.B) < neededLength {
		output.B = make([]byte, 0, roundUpToPowerOfTwo(neededLength))
	}

	lastAppend := 0
	for _, ins := range insertions {
		output.B = append(append(output.B, input.B[lastAppend:ins.offset]...), ins.content...)
		lastAppend = ins.offset
	}
	output.B = append(output.B, input.B[lastAppend:]...)

	v.pool.Put(input)
	return output
}

func testDomainPart(part []byte) bool {
	for i := len(part) - 1; i >= 0; i-- {
		if !domainChars.contains(part[i]) {
			return false
		}
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

// https://stackoverflow.com/a/466242/6620659
func roundUpToPowerOfTwo(i int) int {
	i--
	i |= i >> 1
	i |= i >> 2
	i |= i >> 4
	i |= i >> 8
	i |= i >> 16
	return i + 1
}

func split(s []byte, sep byte, result [][]byte) [][]byte {
	n := cap(result) - 1
	i := 0
	for i < n {
		m := bytes.IndexByte(s, sep)
		if m < 0 {
			break
		}
		result[i] = s[:m:m]
		s = s[m+1:]
		i++
	}
	result[i] = s
	return result[:i+1]
}

// asciiSet is a 32-byte value, where each bit represents the presence of a
// given ASCII character in the set. The 128-bits of the lower 16 bytes,
// starting with the least-significant bit of the lowest word to the
// most-significant bit of the highest word, map to the full range of all
// 128 ASCII characters. The 128-bits of the upper 16 bytes will be zeroed,
// ensuring that any non-ASCII character will be reported as not in the set.
type asciiSet [8]uint32

// contains reports whether c is inside the set.
func (as *asciiSet) add(c byte) bool {
	if c >= utf8.RuneSelf {
		return false
	}
	as[c>>5] |= 1 << uint(c&31)
	return true
}

func (as *asciiSet) contains(c byte) bool {
	return (as[c>>5] & (1 << uint(c&31))) != 0
}
