package x

import "github.com/valyala/bytebufferpool"

type Replace interface {
	Apply(input *bytebufferpool.ByteBuffer) *bytebufferpool.ByteBuffer
}
