package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"github.com/tomasen/fcgi_client"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"
	"time"
	//    "net/url"
	//    "github.com/flashmob/go-fastcgi-client"
)

func Log(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		before := time.Now()
		handler.ServeHTTP(w, r)
		log.Printf("%s %s %.4f %s\n", r.RemoteAddr, r.Method, time.Since(before), r.URL)
	})
}

type protocolHandler struct {
	handlers map[int]http.Handler
	services map[string]int
}

func (ph *protocolHandler) AddProtocol(tcpPort int, handler http.Handler) {
	if ph.handlers == nil {
		ph.handlers = make(map[int]http.Handler)
	}
	ph.handlers[tcpPort] = handler
}

func (ph *protocolHandler) probeForHandler(serviceName string) (handler http.Handler, err error) {
	if port, ok := ph.services[serviceName]; ok {
		return ph.handlers[port], nil
	}
	if ph.services == nil {
		ph.services = make(map[string]int)
	}
	for port, handler := range ph.handlers {
		conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", serviceName, port))
		if err != nil {
			continue
		}
		defer conn.Close()
		ph.services[serviceName] = port
		fmt.Println(fmt.Sprintf("Adding service '%s' on port %d", serviceName, port))
		return handler, nil
	}
	return nil, errors.New("Returned to sender, no such number")
}

func (ph *protocolHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	svcName := strings.Split(r.Host, ".")[0]
	handler, err := ph.probeForHandler(svcName)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	handler.ServeHTTP(w, r)
}

func main() {
	port := flag.Int("port", 80, "listening on port")
	flag.Parse()

	fmt.Println("Starting frontrunner on port", *port)

	ph := &protocolHandler{}
	ph.AddProtocol(9000, &fastcgi{})
	ph.AddProtocol(80, &httputil.ReverseProxy{
		Transport: errorHandlingTransport{http.DefaultTransport},
		Director: func(req *http.Request) {
			req.URL.Host = strings.Split(req.Host, ".")[0]
			req.URL.Scheme = "http"
		},
	})

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), ph))

}

type fastcgi struct{}

func (f *fastcgi) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	serviceName := strings.Split(req.Host, ".")[0]

	// connect to the fastcgi backend,
	// and check whether there is an error or not .
	fcgi, err := fcgiclient.Dial("tcp", fmt.Sprintf("%s:9000", serviceName))
	if err != nil {
		log.Println(err)
		http.Error(res, "Unable to connect to the backend", 502)
		return
	}
	// automatically close the fastcgi connection and the requested body at the end .
	defer fcgi.Close()
	defer req.Body.Close()
	remote_addr, remote_port, _ := net.SplitHostPort(req.RemoteAddr)
	req.URL.Path = req.URL.ResolveReference(req.URL).Path
	env := map[string]string{
		"SCRIPT_FILENAME": "/var/www/public/index.php",
		"REQUEST_METHOD":  req.Method,
		"REQUEST_URI":     req.URL.RequestURI(),
		"REQUEST_PATH":    req.URL.Path,
		"PATH_INFO":       req.URL.Path,
		"CONTENT_LENGTH":  fmt.Sprintf("%d", req.ContentLength),
		"CONTENT_TYPE":    req.Header.Get("Content-Type"),
		"REMOTE_ADDR":     remote_addr,
		"REMOTE_PORT":     remote_port,
		"REMOTE_HOST":     remote_addr,
		"QUERY_STRING":    req.URL.Query().Encode(),
		"SERVER_SOFTWARE": VERSION,
		"SERVER_NAME":     req.Host,
		"SERVER_ADDR":     "0.0.0.0",
		"SERVER_PORT":     "80",
		"SERVER_PROTOCOL": req.Proto,
		"FCGI_PROTOCOL":   "tcp",
		"FCGI_ADDR":       FCGI_ADDR,
		"HTTPS":           "",
		"HTTP_HOST":       req.Host,
	}
	// iterate over request headers and append them to the environment varibales in the valid format .
	for k, v := range req.Header {
		env["HTTP_"+strings.Replace(strings.ToUpper(k), "-", "_", -1)] = strings.Join(v, ";")
	}
	// fethcing the response from the fastcgi backend,
	// and check for errors .
	resp, err := fcgi.Request(env, req.Body)
	if err != nil {
		log.Println("err> ", err.Error())
		http.Error(res, "Unable to fetch the response from the backend", 502)
		return
	}
	// parse the fastcgi status .
	resp.Status = resp.Header.Get("Status")
	resp.StatusCode, _ = strconv.Atoi(strings.Split(resp.Status, " ")[0])
	if resp.StatusCode < 100 {
		resp.StatusCode = 200
	}
	// automatically close the fastcgi response body at the end .
	defer resp.Body.Close()
	// read the fastcgi response headers,
	// and apply the actions related to them .
	for k, v := range resp.Header {
		for i := 0; i < len(v); i++ {
			if res.Header().Get(k) == "" {
				res.Header().Set(k, v[i])
			} else {
				res.Header().Add(k, v[i])
			}
		}
	}
	// fetch the fastcgi response location header
	// then redirect the client, then ignore any output .
	if resp.Header.Get("Location") != "" {
		http.Redirect(res, req, resp.Header.Get("Location"), resp.StatusCode)
		return
	}
	// write the response status code .
	res.WriteHeader(resp.StatusCode)
	// only sent the header if the request method isn't HEAD .
	if req.Method != "HEAD" {
		io.Copy(res, resp.Body)
	}
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
