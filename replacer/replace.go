package replacer

import (
	"bytes"
	"regexp"

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
	idxs := v.regex.FindAllSubmatchIndex(input.B, -1)
	l := len(idxs)
	if l == 0 {
		return input
	}
	output := replaceBufferPool.Get()
	output.B = append(output.B, input.B[:idxs[0][0]]...)
	for i, pair := range idxs {
		output.B = v.regex.Expand(output.B, v.replacement, input.B, pair)
		if i+1 < l {
			output.B = append(output.B, input.B[pair[1]:idxs[i+1][0]]...)
		}
	}
	output.B = append(output.B, input.B[idxs[l-1][1]:]...)
	replaceBufferPool.Put(input)
	return output
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
	idxs := v.regex.FindAllIndex(input.B, -1)
	l := len(idxs)
	if l == 0 {
		return input
	}
	output := replaceBufferPool.Get()
	output.B = append(output.B, input.B[:idxs[0][0]]...)
	for i, pair := range idxs {
		output.B = v.replacer(input.B, output.B, pair[0], pair[1])
		if i+1 < l {
			output.B = append(output.B, input.B[pair[1]:idxs[i+1][0]]...)
		}
	}
	output.B = append(output.B, input.B[idxs[l-1][1]:]...)
	replaceBufferPool.Put(input)
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
	matches := make([]int, 0, 16)
	offset := 0
	for {
		index := bytes.Index(input.B[offset:], v.needle)
		if index == -1 {
			break
		}
		matches = append(matches, offset+index)
		offset += index + v.needleLen
	}
	l := len(matches)
	if l == 0 {
		return input
	}

	output := replaceBufferPool.Get()
	neededLength := input.Len() + l*(v.replLen-v.needleLen)
	if cap(output.B) < neededLength {
		replaceBufferPool.Put(output)
		output = &bytebufferpool.ByteBuffer{}
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
	offset += copy(output.B[offset:], input.B[matches[l-1]+v.needleLen:])
	output.B = output.B[0:offset]

	replaceBufferPool.Put(input)
	return output
}
