package main

import (
	"log"
	"flag"
	"fmt"

	"github.com/valyala/fasthttp"
)

var config struct {
	domain      string
	host        string
	port        int
	logRequests bool
}

func main() {
	flag.StringVar(&config.domain, "domain", "", "used in replaces")
	flag.StringVar(&config.host, "host", "0.0.0.0", "address to bind")
	flag.IntVar(&config.port, "port", 8881, "port to bind")
	flag.BoolVar(&config.logRequests, "log-requests", false, "print every request to the log")

	flag.Parse()

	if config.domain == "" {
		fmt.Println("ERROR: You must specify domain with flag  -domain=your.domain")
		return
	}

	InitReplaces()
	StartTicker()

	log.Printf("Starting server on %s:%d", config.host, config.port)
	if err := fasthttp.ListenAndServe(fmt.Sprintf("%s:%d", config.host, config.port), reverseProxyHandler); err != nil {
		log.Fatalf("error in fasthttp server: %s", err)
	}
}
