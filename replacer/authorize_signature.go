package replacer

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/valyala/fasthttp"
)

var vkAppSecrets = map[string]string{
	"2274003": "hHbZxrka2uZ6jB1inYsH", // VK Android
	"3697615": "AlVXZFMUqyrnABp8ncuU", // VK Windows App
	"3502557": "PEObAuQi6KloPM4T30DV", // VK WP App
	"3140623": "VeWdmVclDCtn6ihuP1nt", // VK Iphone
	"3682744": "mY6CDUswIVdJLCD3j15n", // VK Ipad
	"6146827": "qVxWRF1CwHERuIrKBnqe", // VK ME
}

func signAuthrorize(args *fasthttp.Args) {
	args.Del("sig")

	buf := AcquireBuffer()
	buf.B = append(buf.B, "/authorize?"...)
	args.VisitAll(func(key, value []byte) {
		buf.B = append(buf.B, key...)
		buf.B = append(buf.B, '=')
		buf.B = append(buf.B, value...)
		buf.B = append(buf.B, '&')
	})
	// Обрезаем последний &
	if buf.Len() > 0 {
		buf.B = buf.B[:buf.Len()-1]
	}

	buf.B = append(buf.B, "UNKNOWN SECRET"...)
	hash := md5.Sum(buf.B)
	hexHash := hex.EncodeToString(hash[:])
	args.Add("sig", hexHash)

	ReleaseBuffer(buf)
}
