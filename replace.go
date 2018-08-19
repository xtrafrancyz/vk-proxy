package main

import (
	"bytes"
	"regexp"
)

type replace interface {
	apply(data []byte) []byte
}

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

func (v *regexReplace) apply(data []byte) []byte {
	return v.regex.ReplaceAll(data, v.replacement)
}

type regexFastReplace struct {
	regex    *regexp.Regexp
	replacer func(src, dst []byte, start, end int) []byte
}

func newRegexFastReplace(regex string, replacer func(src, dst []byte, start, end int) []byte) *regexFastReplace {
	return &regexFastReplace{
		regex:    regexp.MustCompile(regex),
		replacer: replacer,
	}
}

func (v *regexFastReplace) apply(data []byte) []byte {
	idxs := v.regex.FindAllIndex(data, -1)
	l := len(idxs)
	if l == 0 {
		return data
	}
	ret := append([]byte{}, data[:idxs[0][0]]...)
	for i, pair := range idxs {
		ret = v.replacer(data, ret, pair[0], pair[1])
		if i+1 < l {
			ret = append(ret, data[pair[1]:idxs[i+1][0]]...)
		}
	}
	ret = append(ret, data[idxs[l-1][1]:]...)
	return ret
}

type stringReplace struct {
	needle      []byte
	replacement []byte
}

func newStringReplace(needle, replace string) *stringReplace {
	return &stringReplace{
		needle:      []byte(needle),
		replacement: []byte(replace),
	}
}

func (v *stringReplace) apply(data []byte) []byte {
	return bytes.Replace(data, v.needle, v.replacement, -1)
}
