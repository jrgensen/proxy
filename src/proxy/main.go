package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
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

func websocketProxy(target string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, err := net.Dial("tcp", target)
		if err != nil {
			http.Error(w, "Error contacting backend server.", 500)
			log.Printf("Error dialing websocket backend %s: %v", target, err)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "Not a hijacker?", 500)
			return
		}
		nc, _, err := hj.Hijack()
		if err != nil {
			log.Printf("Hijack error: %v", err)
			return
		}
		defer nc.Close()
		defer d.Close()

		err = r.Write(d)
		if err != nil {
			log.Printf("Error copying request to target: %v", err)
			return
		}

		errc := make(chan error, 2)
		cp := func(dst io.Writer, src io.Reader) {
			_, err := io.Copy(dst, src)
			errc <- err
		}
		go cp(d, nc)
		go cp(nc, d)
		<-errc
	})
}

type Server struct{}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isWebsocket(r) {
		p := websocketProxy(strings.Split(r.Host, ".")[0] + ":80")
		p.ServeHTTP(w, r)
		return
	}

	handler := &httputil.ReverseProxy{
		Transport: errorHandlingTransport{http.DefaultTransport},
		Director: func(req *http.Request) {
			req.URL.Host = strings.Split(req.Host, ".")[0]
			req.URL.Scheme = "http"
		},
	}
	handler.ServeHTTP(w, r)
}
func isWebsocket(req *http.Request) bool {
	// if this is not an upgrade request it's not a websocket
	if len(req.Header["Connection"]) == 0 || strings.ToLower(req.Header["Connection"][0]) != "upgrade" {
		return false
	}
	if len(req.Header["Upgrade"]) == 0 {
		return false
	}

	return (strings.ToLower(req.Header["Upgrade"][0]) == "websocket")
}

func main() {
	port := flag.Int("port", 80, "listening on port")
	flag.Parse()

	fmt.Println("starting proxy on port", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), &Server{}))
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
