package main

import (
	"log"
	"flag"
	"os"
	"fmt"

	"github.com/valyala/fasthttp"
)

func main() {
	pDomain := flag.String("domain", "", "used in replaces")
	pHost := flag.String("host", "0.0.0.0", "address to bind")
	pPort := flag.Int("port", 8881, "port to bind")

	flag.Parse()

	if *pDomain == "" {
		fmt.Println("ERROR: You must specify domain with flag  -domain=your.domain")
		os.Exit(0)
	}

	InitReplaces(*pDomain)
	StartTicker()

	log.Printf("Starting server on %s:%d", *pHost, *pPort)
	if err := fasthttp.ListenAndServe(fmt.Sprintf("%s:%d", *pHost, *pPort), reverseProxyHandler); err != nil {
		log.Fatalf("error in fasthttp server: %s", err)
	}
}
