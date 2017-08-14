package main

import (
	"log"
	"flag"
	"fmt"

	"github.com/valyala/fasthttp"
	"github.com/vharitonsky/iniflags"
)

var Config struct {
	domain      string
	host        string
	port        int
	logRequests bool
}

func main() {
	flag.StringVar(&Config.host, "host", "0.0.0.0", "address to bind")
	flag.IntVar(&Config.port, "port", 8881, "port to bind")
	flag.StringVar(&Config.domain, "domain", "", "force use this domain for replaces")
	flag.BoolVar(&Config.logRequests, "log-requests", false, "print every request to the log")

	iniflags.Parse()

	StartTicker()

	log.Printf("Starting server on %s:%d", Config.host, Config.port)
	if err := fasthttp.ListenAndServe(fmt.Sprintf("%s:%d", Config.host, Config.port), reverseProxyHandler); err != nil {
		log.Fatalf("error in fasthttp server: %s", err)
	}
}
