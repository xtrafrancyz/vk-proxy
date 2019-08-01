package hardcode

import (
	"bytes"
	"sync"
	"unicode/utf8"

	"github.com/valyala/bytebufferpool"
)

var domainChars = func() (as asciiSet) {
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

var (
	userapiStr            = []byte("userapi")
	vkuserStr             = []byte("vkuser")
	vkcdnStr              = []byte("vk-cdn")
	vkStr                 = []byte("vk")
	comStr                = []byte("com")
	netStr                = []byte("net")
	meStr                 = []byte("me")
	videoStr              = []byte("video")
	audioStr              = []byte("audio")
	liveStr               = []byte("live")
	escapedSlashStr       = []byte(`\/`)
	escapedDoubleSlashStr = []byte(`\/\/`)
	dotStr                = []byte(`.`)
	jsonHttpsStr          = []byte(`"https:`)
	docPathStr            = []byte(`doc`)
	imagesPathStr         = []byte(`images\/`)
	imagesPath2Str        = []byte(`\/images\/`)
	stickerPathStr        = []byte(`sticker`)
	stickersPathEndingStr = []byte(`s_`)
	videoHlsStr           = []byte(`video_hls.php`)
)

const (
	maxDomainPartLen      = 15
	escapedDoubleSlashLen = 4
	jsonHttpsLen          = 7
)

type HardcodedDomainReplaceConfig struct {
	Pool bytebufferpool.Pool

	// Домен, который просто пропускает трафик через себя без обработки, обычно domain.com\/_\/
	SimpleReplace string

	// Домен для замены с обработкой, обычно domain.com\/@
	SmartReplace string
}

type hardcodedDomainReplace struct {
	pool   bytebufferpool.Pool
	simple []byte
	smart  []byte
}

type insertion struct {
	offset  int
	content []byte
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
	var output *bytebufferpool.ByteBuffer
	var host [][]byte
	offset := 0
	inputLen := len(input.B)
	var insertions []insertion
	for {
		index := bytes.Index(input.B[offset:], escapedDoubleSlashStr)
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
		if host == nil {
			host = make([][]byte, 3, 3)
		} else {
			host = host[:cap(host)]
		}
		host = split(input.B[offset:offset+domainLength], '.', host)
		for i := len(host) - 1; i >= 0; i-- {
			if len(host[i]) > maxDomainPartLen || !testDomainPart(host[i]) {
				continue
			}
		}

		ins := v.simple
		// Проверка что домен можно проксировать
		if len(host) == 2 {
			if bytes.Equal(host[0], vkStr) && bytes.Equal(host[1], comStr) { // vk.com
				// Слишком короткий путь, гуляем
				if offset+domainLength+5 > inputLen || input.B[offset+domainLength] != '\\' {
					continue
				}
				path := input.B[offset+domainLength+2:]

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
		} else if len(host) == 3 {
			if bytes.Equal(host[1], userapiStr) { // *.userapi.com
				if !bytes.Equal(host[2], comStr) {
					continue
				}
			} else if bytes.Equal(host[1], vkcdnStr) { // *.vk-cdn.net
				if !bytes.Equal(host[2], netStr) {
					continue
				}
			} else if bytes.Equal(host[1], vkStr) { // *.vk.com
				if bytes.Equal(host[2], comStr) {
					// Домен m.vk.com не проксим
					if len(host[0]) == 1 && host[0][0] == 'm' {
						continue
					}
				} else if !bytes.Equal(host[2], meStr) { // *.vk.me
					continue
				} else {
					continue
				}
			} else if bytes.HasPrefix(host[1], vkuserStr) { // *.vkuser(audio|video|live).(net|com)
				if !bytes.Equal(host[2], comStr) && !bytes.Equal(host[2], netStr) {
					continue
				}
				r := host[1][len(vkuserStr):]
				if !bytes.Equal(r, videoStr) && !bytes.Equal(r, audioStr) && !bytes.Equal(r, liveStr) {
					continue
				}
			} else {
				continue
			}
		} else {
			continue
		}
		if insertions == nil {
			insertions = acquireInsertions()
		}
		insertions = append(insertions, insertion{
			offset:  offset,
			content: ins,
		})
	}
	if insertions == nil {
		return input
	}

	neededLength := inputLen
	for _, ins := range insertions {
		neededLength += len(ins.content)
	}

	output = v.pool.Get()
	if cap(output.B) < neededLength {
		v.pool.Put(output)
		output = &bytebufferpool.ByteBuffer{}
		output.B = make([]byte, 0, neededLength)
	}

	lastAppend := 0
	for _, ins := range insertions {
		output.B = append(append(output.B, input.B[lastAppend:ins.offset]...), ins.content...)
		lastAppend = ins.offset
	}
	output.B = append(output.B, input.B[lastAppend:]...)

	releaseInsertions(insertions)
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
	n := cap(result) -1
	i := 0
	for i < n {
		m := bytes.IndexByte(s, sep)
		if m < 0 {
			break
		}
		result[i] = s[: m : m]
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

func acquireInsertions() []insertion {
	v := insertionsPool.Get()
	if v != nil {
		return v.([]insertion)
	}
	return make([]insertion, 0, 32)
}

func releaseInsertions(a []insertion) {
	a = a[:0]
	insertionsPool.Put(a)
}

var insertionsPool sync.Pool
