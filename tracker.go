package main

import (
	"log"
	"sync"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"github.com/valyala/fasthttp"
)

var tLock = sync.Mutex{}
var tRequests uint32 = 0
var tBytes uint64 = 0
var tUniqueUsers = make(map[string]bool)

func StartTicker() {
	ticker := time.NewTicker(60 * time.Second)
	go func() {
		for {
			<-ticker.C
			tLock.Lock()
			log.Printf("Requests: %d, Traffic: %s, Online: %d", tRequests, bytefmt.ByteSize(tBytes), len(tUniqueUsers))
			tRequests = 0
			tBytes = 0
			tUniqueUsers = make(map[string]bool)
			tLock.Unlock()
		}
	}()
}

func trackRequest(ctx *fasthttp.RequestCtx, size int) {
	ip := ctx.Request.Header.Peek("CF-Connecting-IP") // Cloudflare
	if ip == nil {
		ip = ctx.Request.Header.Peek("X-Real-IP") // nginx
	}
	if ip == nil {
		ip = []byte(ctx.RemoteIP().String()) // real
	}

	if Config.logRequests {
		log.Printf("%s %s [%s]", ctx.Method(), ctx.Request.RequestURI(), ip)
	}

	tLock.Lock()

	tUniqueUsers[string(ip)] = true
	tRequests++
	tBytes += uint64(size)

	tLock.Unlock()
}

// Used only for performance testing
// >> defer trackTime(time.Now(), "name")
func trackTime(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s took %s", name, elapsed)
}
