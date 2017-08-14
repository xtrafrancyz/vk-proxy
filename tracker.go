package main

import (
	"time"
	"log"
	"sync/atomic"

	"code.cloudfoundry.org/bytefmt"
	"github.com/valyala/fasthttp"
)

var tRequests uint32 = 0
var tBytes uint64 = 0

func StartTicker() {
	ticker := time.NewTicker(60 * time.Second)
	go func() {
		for {
			<-ticker.C
			log.Printf("Requests: %d, Traffic: %s", atomic.LoadUint32(&tRequests), bytefmt.ByteSize(atomic.LoadUint64(&tBytes)))
			atomic.StoreUint32(&tRequests, 0)
			atomic.StoreUint64(&tBytes, 0)
		}
	}()
}

func trackRequestStart(ctx *fasthttp.RequestCtx) {
	if Config.logRequests {
		ip := ctx.Request.Header.Peek("CF-Connecting-IP") // Cloudflare
		if ip == nil {
			ip = ctx.Request.Header.Peek("X-Real-IP") // nginx
		}
		if ip == nil {
			ip = []byte(ctx.RemoteIP().String()) // real
		}
		log.Printf("%s %s [%s]", ctx.Method(), ctx.Request.RequestURI(), ip)
	}
	atomic.AddUint32(&tRequests, 1)
}

func trackRequestEnd(size int) {
	atomic.AddUint64(&tBytes, uint64(size))
}

func trackTime(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s took %s", name, elapsed)
}
