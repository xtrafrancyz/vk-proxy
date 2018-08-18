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
