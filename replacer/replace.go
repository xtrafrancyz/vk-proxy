package replacer

import (
	"bytes"
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
	idxs := v.regex.FindAllSubmatchIndex(input.B, -1)
	l := len(idxs)
	if l == 0 {
		return input
	}
	output := AcquireBuffer()
	output.B = append(output.B, input.B[:idxs[0][0]]...)
	for i, pair := range idxs {
		output.B = v.regex.Expand(output.B, v.replacement, input.B, pair)
		if i+1 < l {
			output.B = append(output.B, input.B[pair[1]:idxs[i+1][0]]...)
		}
	}
	output.B = append(output.B, input.B[idxs[l-1][1]:]...)
	ReleaseBuffer(input)
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
	output := AcquireBuffer()
	output.B = append(output.B, input.B[:idxs[0][0]]...)
	for i, pair := range idxs {
		output.B = v.replacer(input.B, output.B, pair[0], pair[1])
		if i+1 < l {
			output.B = append(output.B, input.B[pair[1]:idxs[i+1][0]]...)
		}
	}
	output.B = append(output.B, input.B[idxs[l-1][1]:]...)
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
		ReleaseBuffer(output)
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
	offset += copy(output.B[offset:], input.B[matches[len(matches)-1]+v.needleLen:])
	output.B = output.B[0:offset]

	releaseMatches(matches)
	ReleaseBuffer(input)
	return output
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
