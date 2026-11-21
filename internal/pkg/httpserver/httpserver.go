package httpserver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	auth "github.com/abbot/go-http-auth"
	lm "github.com/name-cho/newhole/internal/app/newhole/models"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/urlmaker"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/net/webdav"
)

const logoURL = "https://raw.githubusercontent.com/name-cho/newhole/main/docs/logo.png"

const (
	DefaultMaxHeaderBytes = 1 << 20
	DefaultMaxRequestBody = 1 << 30
	DefaultReadTimeout    = 0
	DefaultIdleTimeout    = 120 * time.Second
)

type ServerBuilder interface {
	WithSiteID(string) ServerBuilder
	WithDomain(string) ServerBuilder
	DisableOldCiphers(bool) ServerBuilder
	Proxy() ProxyServerBuilder
	ServeStatic() StaticServerBuilder
	ServeWebdav() WebdavServerBuilder
	WithRemoteSpecs(lm.RemoteEndpointSpecs) ServerBuilder
	EnableDirectoryListing() ServerBuilder
}

type serverBuilder struct {
	siteID            string
	domain            string
	disableOldCiphers bool
	specs             lm.RemoteEndpointSpecs
	directoryListing  bool
}

func (sb *serverBuilder) WithSiteID(siteID string) ServerBuilder {
	sb.siteID = strings.ToLower(siteID)
	return sb
}

func (sb *serverBuilder) WithDomain(domain string) ServerBuilder {
	sb.domain = domain
	return sb
}

func (sb *serverBuilder) DisableOldCiphers(setting bool) ServerBuilder {
	sb.disableOldCiphers = setting
	return sb
}

func (sb *serverBuilder) WithRemoteSpecs(specs lm.RemoteEndpointSpecs) ServerBuilder {
	sb.specs = specs
	return sb
}

func (sb *serverBuilder) EnableDirectoryListing() ServerBuilder {
	sb.directoryListing = true
	return sb
}

func (sb *serverBuilder) Proxy() ProxyServerBuilder {
	return &proxyServerBuilder{serverBuilder: sb}
}

func (sb *serverBuilder) ServeStatic() StaticServerBuilder {
	return &staticServerBuilder{serverBuilder: sb}
}

func (sb *serverBuilder) ServeWebdav() WebdavServerBuilder {
	return &webdavServerBuilder{serverBuilder: sb}
}

type ProxyServerBuilder interface {
	ToEndpoint(lm.Endpoint) ProxyServerBuilder
	WithBasicAuth(string, string) ProxyServerBuilder
	DisableProxyErrorPage() ProxyServerBuilder
	EnableInsecureHTTPSBackend() ProxyServerBuilder
	WithDialer(func() (net.Conn, error)) ProxyServerBuilder
	Build() (*http.Server, error)
}

type proxyServerBuilder struct {
	serverBuilder         *serverBuilder
	endpoint              lm.Endpoint
	basicAuthEnabled      bool
	basicAuthUsername     string
	basicAuthPassword     string
	disableProxyErrorPage bool
	disableCertCheck      bool
	dialer                func() (net.Conn, error)
}

func (psb *proxyServerBuilder) ToEndpoint(endpoint lm.Endpoint) ProxyServerBuilder {
	psb.endpoint = endpoint
	return psb
}

func (psb *proxyServerBuilder) WithBasicAuth(username string, password string) ProxyServerBuilder {
	psb.basicAuthEnabled = username != "" || password != ""
	psb.basicAuthUsername = username
	psb.basicAuthPassword = password
	return psb
}

func (psb *proxyServerBuilder) DisableProxyErrorPage() ProxyServerBuilder {
	psb.disableProxyErrorPage = true
	return psb
}

func (psb *proxyServerBuilder) EnableInsecureHTTPSBackend() ProxyServerBuilder {
	psb.disableCertCheck = true
	return psb
}

func (psb *proxyServerBuilder) WithDialer(dialer func() (net.Conn, error)) ProxyServerBuilder {
	psb.dialer = dialer
	return psb
}

func (psb *proxyServerBuilder) Build() (*http.Server, error) {
	scheme := psb.endpoint.Protocol
	if scheme == "" {
		scheme = "http"
	}

	target := &url.URL{
		Scheme: scheme,
		Host:   psb.endpoint.Hostname(),
	}
	if psb.endpoint.Path != "" {
		target.Path = psb.endpoint.Path
	}

	specs := psb.serverBuilder.specs
	publicHost := ""
	if psb.serverBuilder.domain != "" {
		publicHost = urlmaker.GetSiteFQDN(psb.serverBuilder.siteID, psb.serverBuilder.domain)
	}
	proxy := newReverseProxy(target, specs, publicHost)

	proxy.Transport = &http.Transport{
		Proxy:                 nil,
		DialContext:           dialContext(psb.dialer, target, specs),
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: time.Second,
		ResponseHeaderTimeout: 0,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     false,
	}
	if psb.disableCertCheck {
		proxy.Transport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	}

	if !psb.disableProxyErrorPage {
		proxy.ErrorHandler = proxyErrorHandler
	}

	handler := http.Handler(proxy)
	handler = withRequestSanitizer(handler, specs)
	if psb.basicAuthEnabled {
		authHandler, err := getBasicAuthHandler(psb.serverBuilder.siteID, psb.serverBuilder.domain, psb.basicAuthUsername, psb.basicAuthPassword, handler)
		if err != nil {
			return nil, err
		}
		handler = authHandler
	}

	return newServer(handler, psb.serverBuilder.siteID, psb.serverBuilder.domain, psb.serverBuilder.disableOldCiphers, specs), nil
}

func dialContext(dialer func() (net.Conn, error), target *url.URL, specs lm.RemoteEndpointSpecs) func(ctx context.Context, network string, addr string) (net.Conn, error) {
	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		if dialer == nil {
			return (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, addr)
		}
		return dialer()
	}
}

type StaticServerBuilder interface {
	FromDirectory(string) StaticServerBuilder
	WithBasicAuth(string, string) StaticServerBuilder
	WithDirectoryListing(bool) StaticServerBuilder
	Build() (*http.Server, error)
}

type staticServerBuilder struct {
	serverBuilder     *serverBuilder
	directory         string
	directoryListing  bool
	basicAuthEnabled  bool
	basicAuthUsername string
	basicAuthPassword string
}

func (ssb *staticServerBuilder) FromDirectory(directory string) StaticServerBuilder {
	ssb.directory = directory
	return ssb
}

func (ssb *staticServerBuilder) WithBasicAuth(username string, password string) StaticServerBuilder {
	ssb.basicAuthEnabled = username != "" || password != ""
	ssb.basicAuthUsername = username
	ssb.basicAuthPassword = password
	return ssb
}

func (ssb *staticServerBuilder) WithDirectoryListing(enabled bool) StaticServerBuilder {
	ssb.directoryListing = enabled
	return ssb
}

func (ssb *staticServerBuilder) Build() (*http.Server, error) {
	fileServer := newStaticHandler(ssb.directory, ssb.directoryListing)

	var handler http.Handler = fileServer
	if ssb.basicAuthEnabled {
		authHandler, err := getBasicAuthHandler(ssb.serverBuilder.siteID, ssb.serverBuilder.domain, ssb.basicAuthUsername, ssb.basicAuthPassword, handler)
		if err != nil {
			return nil, err
		}
		handler = authHandler
	}

	return newServer(handler, ssb.serverBuilder.siteID, ssb.serverBuilder.domain, ssb.serverBuilder.disableOldCiphers, ssb.serverBuilder.specs), nil
}

type WebdavServerBuilder interface {
	FromDirectory(string) WebdavServerBuilder
	WithBasicAuth(string, string) WebdavServerBuilder
	Build() (*http.Server, error)
}

type webdavServerBuilder struct {
	serverBuilder     *serverBuilder
	directory         string
	basicAuthEnabled  bool
	basicAuthUsername string
	basicAuthPassword string
}

func (wsb *webdavServerBuilder) FromDirectory(directory string) WebdavServerBuilder {
	wsb.directory = directory
	return wsb
}

func (wsb *webdavServerBuilder) WithBasicAuth(username string, password string) WebdavServerBuilder {
	wsb.basicAuthEnabled = username != "" || password != ""
	wsb.basicAuthUsername = username
	wsb.basicAuthPassword = password
	return wsb
}

func (wsb *webdavServerBuilder) Build() (*http.Server, error) {
	handler := &webdav.Handler{
		Prefix:     "/",
		FileSystem: webdav.Dir(wsb.directory),
		LockSystem: webdav.NewMemLS(),
	}

	var final http.Handler = handler
	if wsb.basicAuthEnabled {
		authHandler, err := getBasicAuthHandler(wsb.serverBuilder.siteID, wsb.serverBuilder.domain, wsb.basicAuthUsername, wsb.basicAuthPassword, final)
		if err != nil {
			return nil, err
		}
		final = authHandler
	}

	return newServer(final, wsb.serverBuilder.siteID, wsb.serverBuilder.domain, wsb.serverBuilder.disableOldCiphers, wsb.serverBuilder.specs), nil
}

func New() ServerBuilder {
	return &serverBuilder{}
}

func newServer(handler http.Handler, siteID string, domain string, disableOldCiphers bool, specs lm.RemoteEndpointSpecs) *http.Server {
	maxHeaderBytes := specs.MaxHeaderBytes
	if maxHeaderBytes <= 0 {
		maxHeaderBytes = DefaultMaxHeaderBytes
	}
	return &http.Server{
		Handler:           handler,
		TLSConfig:         getTLSConfig(siteID, domain, disableOldCiphers),
		MaxHeaderBytes:    maxHeaderBytes,
		IdleTimeout:       DefaultIdleTimeout,
		ErrorLog:          tunnelErrorLog(),
		ReadHeaderTimeout: 30 * time.Second,
	}
}

func getBasicAuthHandler(siteID string, domain string, username string, password string, handler http.Handler) (http.Handler, error) {
	if username == "" || password == "" {
		return nil, errors.New("basic auth requires both a username and a password")
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("unable to hash the basic auth password: %w", err)
	}

	secret := func(user string, realm string) string {
		if user == username {
			return string(hashedPassword)
		}
		return ""
	}

	authenticator := auth.NewBasicAuthenticator(urlmaker.GetSiteFQDN(siteID, domain), secret)
	wrapped := auth.JustCheck(authenticator, func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	})
	return http.HandlerFunc(wrapped), nil
}

func proxyErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	message := err.Error()
	if isClientDisconnect(err) {
		communication.TunnelDebug("", "the client closed the connection before the response was complete")
		return
	}
	if r.Context().Err() != nil {
		return
	}
	communication.TunnelWarn("", "the local service could not be reached: "+message)
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprintf(w, proxyErrorTemplate, logoURL, htmlEscape(message))
}

func isClientDisconnect(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"context canceled",
		"broken pipe",
		"connection reset by peer",
		"client disconnected",
		"use of closed network connection",
		"unexpected eof",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func htmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return replacer.Replace(value)
}

func newReverseProxy(target *url.URL, specs lm.RemoteEndpointSpecs, publicHost string) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = nil
	proxy.Rewrite = func(request *httputil.ProxyRequest) {
		originalHost := request.In.Host
		remoteIP := clientIP(request.In)

		request.SetURL(target)
		request.Out.Header.Del("Forwarded")
		request.Out.Header.Del("X-Forwarded-For")
		request.Out.Header.Del("X-Real-Ip")
		request.SetXForwarded()

		if remoteIP != "" {
			forwardedFor := firstNonEmpty(specs.ForwardedForHeader, "X-Forwarded-For")
			request.Out.Header.Set(forwardedFor, remoteIP)
			if realIP := specs.RealIPHeader; realIP != "" {
				request.Out.Header.Set(realIP, remoteIP)
			}
		}

		if specs.Compress {
			negotiateCompression(request)
		}

		switch {
		case specs.OverrideHostHeader != "":
			request.Out.Host = specs.OverrideHostHeader
		case specs.PreserveHostHeader:
			request.Out.Host = originalHost
		case publicHost != "":
			request.Out.Host = publicHost
		default:
			request.Out.Host = originalHost
		}
	}

	proxy.FlushInterval = 100 * time.Millisecond
	return proxy
}

func negotiateCompression(request *httputil.ProxyRequest) {
	accepted := strings.ToLower(request.In.Header.Get("Accept-Encoding"))
	if !strings.Contains(accepted, "gzip") || strings.Contains(accepted, "br") {
		return
	}
	request.Out.Header.Set("Accept-Encoding", "gzip")
}

func clientIP(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
