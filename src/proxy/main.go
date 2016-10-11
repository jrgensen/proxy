package main

import (
	"bytes"
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
		Transport: errorHandlingTransport{http.DefaultTransport},
		Director: func(req *http.Request) {
			req.URL.Host = strings.Split(req.Host, ".")[0]
			req.URL.Scheme = "http"
		},
	}))
}

// The ReverseProxy implementation does not write any meaningful response if
// the request fails. This overwritten RoundTripper (which does not conform to
// the round tripper specification), converts a failed request to a BAD GATEWAY
// response
type errorHandlingTransport struct {
	http.RoundTripper
}

func (t errorHandlingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	result, err := t.RoundTripper.RoundTrip(request)
	if err != nil {
		result = &http.Response{
			Status:        "BAD GATEWAY",
			StatusCode:    http.StatusBadGateway,
			Body:          createErrorMsg(fmt.Sprintf("Proxy error when accessing %v\n%v", request.URL, err)),
			Proto:         request.Proto,
			ProtoMajor:    request.ProtoMajor,
			ProtoMinor:    request.ProtoMinor,
			ContentLength: -1,
		}
		err = nil
	}
	return result, err
}

func createErrorMsg(str string) ClosingBuffer {
	// Suppress "friendly error pages" in IE and Chrome.
	if len(str) < 512 {
		str += strings.Repeat(" ", 512-len(str))
	}
	return ClosingBuffer{bytes.NewBufferString(str)}
}

// bytes.Buffer does not implement a Close() method, add a dummy one.
type ClosingBuffer struct {
	*bytes.Buffer
}

func (cb ClosingBuffer) Close() (err error) {
	return
}
