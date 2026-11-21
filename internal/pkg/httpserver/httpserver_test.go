package httpserver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	lm "github.com/name-cho/newhole/internal/app/newhole/models"
)

func proxyForTest(t *testing.T, upstream *httptest.Server, specs lm.RemoteEndpointSpecs) *httptest.Server {
	t.Helper()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(target.Port())
	builder := New().
		WithSiteID("mysite").
		WithDomain("example.test").
		WithRemoteSpecs(specs).
		Proxy().
		ToEndpoint(lm.Endpoint{Protocol: "http", Host: target.Hostname(), Port: int32(port)}).
		DisableProxyErrorPage()

	server, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(server.Handler)
}

func TestProxySendsSiteHostHeaderByDefault(t *testing.T) {
	var receivedHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
	}))
	defer upstream.Close()

	proxy := proxyForTest(t, upstream, lm.RemoteEndpointSpecs{})
	defer proxy.Close()

	resp, err := http.Get(proxy.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	want := "mysite.example.test"
	if receivedHost != want {
		t.Fatalf("upstream host header %q differs from expected %q", receivedHost, want)
	}
	if strings.Contains(receivedHost, "127.0.0.1") {
		t.Fatalf("upstream host header %q leaks the local address", receivedHost)
	}
}

func TestProxyOverrideHostHeader(t *testing.T) {
	var receivedHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
	}))
	defer upstream.Close()

	proxy := proxyForTest(t, upstream, lm.RemoteEndpointSpecs{OverrideHostHeader: "internal.service:8080"})
	defer proxy.Close()

	resp, err := http.Get(proxy.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if receivedHost != "internal.service:8080" {
		t.Fatalf("host header %q differs from the override", receivedHost)
	}
}

func TestProxyPreserveHostHeader(t *testing.T) {
	var receivedHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
	}))
	defer upstream.Close()

	proxy := proxyForTest(t, upstream, lm.RemoteEndpointSpecs{PreserveHostHeader: true})
	defer proxy.Close()

	req, _ := http.NewRequest(http.MethodGet, proxy.URL+"/", nil)
	req.Host = "mysite.example.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if receivedHost != "mysite.example.test" {
		t.Fatalf("host header %q differs from the original one", receivedHost)
	}
}

func TestProxyForwardsRealClientIP(t *testing.T) {
	var forwardedFor, realIP string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedFor = r.Header.Get("X-Forwarded-For")
		realIP = r.Header.Get("X-Real-IP")
	}))
	defer upstream.Close()

	proxy := proxyForTest(t, upstream, lm.RemoteEndpointSpecs{RealIPHeader: "X-Real-IP"})
	defer proxy.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(proxy.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: mysite.example.test\r\nConnection: close\r\nX-Forwarded-For: 1.2.3.4\r\n\r\n")
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	io.Copy(io.Discard, conn)
	conn.Close()

	if forwardedFor != "127.0.0.1" {
		t.Fatalf("X-Forwarded-For %q was not replaced with the real client ip", forwardedFor)
	}
	if realIP != "127.0.0.1" {
		t.Fatalf("X-Real-IP %q was not set", realIP)
	}
}

func TestProxyLargeQueryStringIsAccepted(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.URL.RawQuery)
	}))
	defer upstream.Close()

	proxy := proxyForTest(t, upstream, lm.RemoteEndpointSpecs{})
	defer proxy.Close()

	query := "data=" + strings.Repeat("a", 20000)
	resp, err := http.Get(proxy.URL + "/?" + query)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d differs from expected", resp.StatusCode)
	}
	if len(body) < 20000 {
		t.Fatalf("query was truncated to %d bytes", len(body))
	}
}

func TestProxyCORSPreflight(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "body")
	}))
	defer upstream.Close()

	proxy := proxyForTest(t, upstream, lm.RemoteEndpointSpecs{CORSAllowedOrigins: []string{"https://app.example"}})
	defer proxy.Close()

	req, _ := http.NewRequest(http.MethodOptions, proxy.URL+"/", nil)
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d differs from expected", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatal("missing CORS allow origin header")
	}
}

func TestProxyRateLimit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer upstream.Close()

	proxy := proxyForTest(t, upstream, lm.RemoteEndpointSpecs{RateLimitRequests: 2, RateLimitWindow: 60})
	defer proxy.Close()

	statuses := []int{}
	for i := 0; i < 4; i++ {
		resp, err := http.Get(proxy.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		statuses = append(statuses, resp.StatusCode)
		resp.Body.Close()
	}
	if statuses[0] != http.StatusOK || statuses[3] != http.StatusTooManyRequests {
		t.Fatalf("unexpected status sequence %v", statuses)
	}
}

func TestProxyBodyLimit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
	}))
	defer upstream.Close()

	proxy := proxyForTest(t, upstream, lm.RemoteEndpointSpecs{MaxRequestBody: 1024})
	defer proxy.Close()

	resp, err := http.Post(proxy.URL+"/", "text/plain", strings.NewReader(strings.Repeat("x", 4096)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d differs from expected", resp.StatusCode)
	}
}

func TestProxyErrorPageIsDisabledForBrokenBackends(t *testing.T) {
	builder := New().
		WithSiteID("mysite").
		WithDomain("example.test").
		Proxy().
		ToEndpoint(lm.Endpoint{Protocol: "http", Host: "127.0.0.1", Port: 1})

	server, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status %d differs from expected", recorder.Code)
	}
}

func TestTLSErrorsAreClassifiedAsDebug(t *testing.T) {
	cases := map[string]string{
		"http: TLS handshake error from 127.0.0.1:1: tls: first record does not look like a TLS handshake":               "debug",
		"http: TLS handshake error from 127.0.0.1:1: local error: tls: bad record MAC":                                   "debug",
		"http: TLS handshake error from 127.0.0.1:1: tls: client requested unsupported application protocols ([hq h2c])": "debug",
		"http: TLS handshake error from 127.0.0.1:1: acme/autocert: missing certificate":                                 "debug",
		"something completely unexpected": "warn",
	}
	for message, expected := range cases {
		level, explanation := classifyTLSError(message)
		if level != expected {
			t.Fatalf("%q was classified as %q instead of %q", message, level, expected)
		}
		if explanation == "" {
			t.Fatalf("%q has no explanation", message)
		}
	}
}

func TestStaticServerDirectoryListing(t *testing.T) {
	directory := t.TempDir()
	if err := writeFile(directory+"/hello.txt", "hi"); err != nil {
		t.Fatal(err)
	}

	builder := New().
		WithSiteID("mysite").
		WithDomain("example.test").
		ServeStatic().
		FromDirectory(directory).
		WithDirectoryListing(true)
	server, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}

	for path, expected := range map[string]string{"/": "hello.txt", "/hello.txt": "hi"} {
		recorder := httptest.NewRecorder()
		server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("%s did not contain %q: %s", path, expected, recorder.Body.String())
		}
	}
}

func TestStaticServerHidesDirectoryListingByDefault(t *testing.T) {
	directory := t.TempDir()
	if err := writeFile(directory+"/secret.txt", "hi"); err != nil {
		t.Fatal(err)
	}

	builder := New().
		WithSiteID("mysite").
		WithDomain("example.test").
		ServeStatic().
		FromDirectory(directory)
	server, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(recorder.Body.String(), "secret.txt") {
		t.Fatal("directory listing leaked the file name")
	}
}

func TestBasicAuthRejectsWrongCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "secret")
	}))
	defer upstream.Close()

	target, _ := url.Parse(upstream.URL)
	port, _ := strconv.Atoi(target.Port())
	builder := New().
		WithSiteID("mysite").
		WithDomain("example.test").
		Proxy().
		ToEndpoint(lm.Endpoint{Protocol: "http", Host: target.Hostname(), Port: int32(port)}).
		WithBasicAuth("user", "password")
	server, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status %d differs from expected", recorder.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetBasicAuth("user", "password")
	recorder = httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d differs from expected", recorder.Code)
	}
}

var _ = tls.VersionTLS12

func TestProxyClientDisconnectIsNotAnError(t *testing.T) {
	cases := []struct {
		err      error
		expected bool
	}{
		{context.Canceled, true},
		{errors.New("http: proxy error: context canceled"), true},
		{errors.New("write tcp 127.0.0.1: broken pipe"), true},
		{errors.New("read tcp 127.0.0.1: connection reset by peer"), true},
		{io.EOF, true},
		{errors.New("dial tcp 127.0.0.1:8080: connection refused"), false},
	}
	for _, testCase := range cases {
		if got := isClientDisconnect(testCase.err); got != testCase.expected {
			t.Fatalf("isClientDisconnect(%v) = %v, expected %v", testCase.err, got, testCase.expected)
		}
	}
}

func TestProxyErrorPageOnBrokenBackend(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()

	server, err := New().
		WithSiteID("some-site").
		WithDomain("loophole.site").
		Proxy().
		ToEndpoint(lm.Endpoint{Host: "127.0.0.1", Port: 1}).
		WithDialer(func() (net.Conn, error) {
			return net.Dial("tcp", address)
		}).
		Build()
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://some-site.loophole.site/", nil)
	server.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status %d differs from 502", recorder.Code)
	}
}

func TestProxyCompressionNegotiation(t *testing.T) {
	var receivedEncoding string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedEncoding = r.Header.Get("Accept-Encoding")
	}))
	defer upstream.Close()

	proxy := proxyForTest(t, upstream, lm.RemoteEndpointSpecs{Compress: true})
	defer proxy.Close()

	request, _ := http.NewRequest(http.MethodGet, proxy.URL+"/", nil)
	request.Header.Set("Accept-Encoding", "gzip, deflate")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if receivedEncoding != "gzip" {
		t.Fatalf("upstream received %q instead of gzip", receivedEncoding)
	}

	request, _ = http.NewRequest(http.MethodGet, proxy.URL+"/", nil)
	request.Header.Set("Accept-Encoding", "br")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if receivedEncoding != "br" {
		t.Fatalf("upstream received %q instead of the original header", receivedEncoding)
	}
}
