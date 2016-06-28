package main

import (
    "fmt"
    "flag"
    "log"
    "net/http"
    "net/http/httputil"
    "strings"
)

func main() {
    port := flag.Int("port", 80, "listening on port")
    flag.Parse()

    fmt.Println("starting proxy on port", *port)
    log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), &httputil.ReverseProxy{
        Director: func(req *http.Request) {
            log.Print(req.Host)
            req.URL.Host = strings.Split(req.Host, ".")[0]
            req.URL.Scheme = "http"
        },
    }))
}
