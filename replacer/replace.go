package replacer

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
	"sync"

	"github.com/valyala/bytebufferpool"
)

var (
	replaceBufferPool bytebufferpool.Pool
)

type regexReplace struct {
	regex       *regexp.Regexp
	replacement []byte
}

func newRegexReplace(regex, replace string) *regexReplace {
	return &regexReplace{
		regex:       regexp.MustCompile(regex),
		replacement: []byte(replace),
	}
}

func (v *regexReplace) Apply(input *bytebufferpool.ByteBuffer) *bytebufferpool.ByteBuffer {
	return applyRegexpBytes(v.regex.FindAllSubmatchIndex(input.B, -1), input, func(src, dst []byte, match []int) []byte {
		return v.regex.Expand(dst, v.replacement, src, match)
	})
}

type regexFuncReplace struct {
	regex    *regexp.Regexp
	replacer func(src, dst []byte, start, end int) []byte
}

func newRegexFuncReplace(regex string, replacer func(src, dst []byte, start, end int) []byte) *regexFuncReplace {
	return &regexFuncReplace{
		regex:    regexp.MustCompile(regex),
		replacer: replacer,
	}
}

func (v *regexFuncReplace) Apply(input *bytebufferpool.ByteBuffer) *bytebufferpool.ByteBuffer {
	return v.ApplyFunc(input, v.replacer)
}

func (v *regexFuncReplace) ApplyFunc(input *bytebufferpool.ByteBuffer,
	f func(src, dst []byte, start, end int) []byte) *bytebufferpool.ByteBuffer {
	return applyRegexpBytes(v.regex.FindAllIndex(input.B, -1), input, func(src, dst []byte, match []int) []byte {
		return f(src, dst, match[0], match[1])
	})
}

func applyRegexpBytes(matches [][]int, input *bytebufferpool.ByteBuffer,
	expand func(src, dst []byte, match []int) []byte) *bytebufferpool.ByteBuffer {
	l := len(matches)
	if l == 0 {
		return input
	}
	output := AcquireBuffer()
	output.B = append(output.B, input.B[:matches[0][0]]...)
	for i, match := range matches {
		output.B = expand(input.B, output.B, match)
		if i+1 < l {
			output.B = append(output.B, input.B[match[1]:matches[i+1][0]]...)
		}
	}
	output.B = append(output.B, input.B[matches[l-1][1]:]...)
	ReleaseBuffer(input)
	return output
}

type stringReplace struct {
	needle      []byte
	needleLen   int
	replacement []byte
	replLen     int
}

func newStringReplace(needle, replace string) *stringReplace {
	r := &stringReplace{
		needle:      []byte(needle),
		replacement: []byte(replace),
	}
	r.replLen = len(r.replacement)
	r.needleLen = len(r.needle)
	return r
}

func (v *stringReplace) Apply(input *bytebufferpool.ByteBuffer) *bytebufferpool.ByteBuffer {
	var matches []int
	offset := 0
	for {
		index := bytes.Index(input.B[offset:], v.needle)
		if index == -1 {
			break
		}
		if matches == nil {
			matches = acquireMatches()
		}
		matches = append(matches, offset+index)
		offset += index + v.needleLen
	}
	if matches == nil {
		return input
	}

	output := AcquireBuffer()
	neededLength := input.Len() + len(matches)*(v.replLen-v.needleLen)
	if cap(output.B) < neededLength {
		output.B = make([]byte, neededLength)
	} else {
		output.B = output.B[0:neededLength]
	}

	offset = 0
	for i, idx := range matches {
		if i == 0 {
			offset += copy(output.B[offset:], input.B[0:idx])
		} else {
			offset += copy(output.B[offset:], input.B[matches[i-1]+v.needleLen:idx])
		}
		offset += copy(output.B[offset:], v.replacement)
	}
	offset += copy(output.B[offset:], input.B[matches[len(matches)-1]+v.needleLen:])
	output.B = output.B[0:offset]

	releaseMatches(matches)
	ReleaseBuffer(input)
	return output
}

func (v *stringReplace) ApplyStream(r *bufio.Reader, w *bufio.Writer) error {
	for {
		data, err := r.ReadSlice(v.needle[0])
		// Нечего заменять, пишем в ответ
		if err != nil {
			if len(data) > 0 {
				_, err = w.Write(data)
				if err != nil {
					return err
				}
				continue
			}
			if err == io.EOF {
				return nil
			}
			return err
		}
		// Пишем все что было до найденного
		_, err = w.Write(data[:len(data)-1])
		if err != nil {
			return err
		}

		// Пробуем считать длину искомой строки
		buf := make([]byte, v.needleLen-1)
		n, err := io.ReadFull(r, buf)
		if err != nil {
			// Исходник закончился слишком быстро
			if err == io.ErrUnexpectedEOF {
				goto skip
			}
			return err
		}

		// Если это искомая строка
		if bytes.Equal(v.needle[1:], buf) {
			_, err = w.Write(v.replacement)
			if err != nil {
				return err
			}
			continue
		}

	skip:
		// Иначе пишем все как было
		err = w.WriteByte(v.needle[0])
		if err != nil {
			return err
		}
		_, err = w.Write(buf[:n])
		if err != nil {
			return err
		}
	}
}

func acquireMatches() []int {
	v := matchesPool.Get()
	if v != nil {
		return v.([]int)
	}
	return make([]int, 0, 16)
}

func releaseMatches(a []int) {
	a = a[:0]
	matchesPool.Put(a)
}

var matchesPool sync.Pool
