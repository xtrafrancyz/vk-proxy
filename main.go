package main

import (
	"flag"
	"log"
	"strings"

	"github.com/vharitonsky/iniflags"
)

func main() {
	config := ProxyConfig{}

	bind := flag.String("bind", ":8881", "address to bind proxy (can be a unix domain socket: /var/run/vk-proxy.sock)")
	flag.StringVar(&config.BaseDomain, "domain", "vk-api-proxy.example.com", "domain for the replaces")
	flag.StringVar(&config.BaseStaticDomain, "domain-static", "vk-static-proxy.example.com", "replacement of the static.vk.com")
	flag.IntVar(&config.LogVerbosity, "log-verbosity", 1, "0 - only errors, 1 - stats every minute, 2 - all requests, 3 - requests with body")
	flag.BoolVar(&config.ReduceMemoryUsage, "reduce-memory-usage", false, "reduces memory usage at the cost of higher CPU usage")
	flag.BoolVar(&config.FilterFeed, "filter-feed", true, "when enabled, ads from feed will be removed")
	flag.BoolVar(&config.GzipUpstream, "gzip-upstream", true, "use gzip for requests to api.vk.com")
	flag.IntVar(&config.Concurrency, "concurrency", 256*1024, "the maximum number of concurrent connections the server may serve")

	iniflags.Parse()

	p := NewProxy(config)

	var err error
	if strings.HasPrefix(*bind, "/") {
		err = p.ListenUnix(*bind)
	} else {
		err = p.ListenTCP(*bind)
	}

	if err != nil {
		log.Fatalf("Could not start server: %s", err)
	}
}
