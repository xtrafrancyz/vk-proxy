package main

import (
	"flag"
	"log"
	"runtime"
	"strings"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/pprofhandler"
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
	pprofHost := flag.String("pprof-bind", "", "address to bind pprof handler (like 127.0.0.1:7777)")

	iniflags.Parse()

	if *pprofHost != "" {
		go func() {
			log.Printf("Starting pprof server on http://%s", *pprofHost)
			err := fasthttp.ListenAndServe(*pprofHost, pprofhandler.PprofHandler)
			if err != nil {
				log.Fatalf("Could not start pprof server: %s", err)
			}
		}()
	} else {
		runtime.MemProfileRate = 0
	}

	p := NewProxy(config)

	for _, host := range strings.Split(*bind, ",") {
		go func(host string) {
			var err error
			if strings.HasPrefix(host, "/") {
				err = p.ListenUnix(host)
			} else {
				err = p.ListenTCP(host)
			}
			if err != nil {
				log.Printf("Failed to bind listener on %s with %s", host, err.Error())
			}
		}(strings.TrimSpace(host))
	}

	// Sleep forever
	select {}
}
