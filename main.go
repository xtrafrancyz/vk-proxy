package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/valyala/fasthttp"
	"github.com/vharitonsky/iniflags"
)

var Config struct {
	domain            string
	host              string
	unix              string
	port              int
	logRequests       bool
	reduceMemoryUsage bool
	removeAdsFromFeed bool
	debug             bool
}

func main() {
	if os.Getenv("VK_PROXY_DEBUG") == "1" {
		Config.debug = true
	}

	flag.StringVar(&Config.unix, "unix", "", "unix domain socket to bind (example /var/run/vk-proxy.sock)")
	flag.StringVar(&Config.host, "host", "0.0.0.0", "address to bind")
	flag.IntVar(&Config.port, "port", 8881, "port to bind")
	flag.StringVar(&Config.domain, "domain", "", "force use this domain for replaces")
	flag.BoolVar(&Config.logRequests, "log-requests", false, "print every request to the log")
	flag.BoolVar(&Config.reduceMemoryUsage, "reduce-memory-usage", false, "reduces memory usage at the cost of higher CPU usage")
	flag.BoolVar(&Config.removeAdsFromFeed, "remove-ads-from-feed", true, "when enabled, ads from feed will be removed")

	iniflags.Parse()

	StartTicker()

	server := &fasthttp.Server{
		Handler:           reverseProxyHandler,
		ReduceMemoryUsage: Config.reduceMemoryUsage,
	}

	var err error
	if Config.unix != "" {
		log.Printf("Starting server on http://unix:%s", Config.unix)
		err = server.ListenAndServeUNIX(Config.unix, 0777)
	} else {
		log.Printf("Starting server on http://%s:%d", Config.host, Config.port)
		err = server.ListenAndServe(fmt.Sprintf("%s:%d", Config.host, Config.port))
	}

	if err != nil {
		log.Fatalf("error in fasthttp server: %s", err)
	}
}
