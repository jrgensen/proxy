package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

func Log(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		before := time.Now()
		handler.ServeHTTP(w, r)
		log.Printf("%s %s %.4f %s\n", r.RemoteAddr, r.Method, time.Since(before), r.URL)
	})
}

func main() {
	port := flag.Int("port", 80, "listening on port")
	flag.Parse()

	fmt.Println("starting proxy on port", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Host = strings.Split(req.Host, ".")[0]
			req.URL.Scheme = "http"
		},
	}))
}
