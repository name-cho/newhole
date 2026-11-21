package httpserver

import (
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	lm "github.com/name-cho/newhole/internal/app/newhole/models"
	"github.com/name-cho/newhole/internal/pkg/communication"
)

func withRequestSanitizer(next http.Handler, specs lm.RemoteEndpointSpecs) http.Handler {
	handler := next

	if len(specs.CORSAllowedOrigins) > 0 {
		handler = withCORS(handler, specs.CORSAllowedOrigins)
	}
	if specs.RateLimitRequests > 0 {
		window := time.Duration(specs.RateLimitWindow) * time.Second
		if window <= 0 {
			window = time.Second
		}
		handler = withRateLimit(handler, specs.RateLimitRequests, window)
	}
	if specs.MaxRequestBody > 0 {
		handler = withBodyLimit(handler, specs.MaxRequestBody)
	}
	return withPanicRecovery(handler)
}

func withPanicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				communication.TunnelError("", "Recovered from a panic while serving a request")
				http.Error(w, "internal proxy error", http.StatusBadGateway)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func withBodyLimit(next http.Handler, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > limit {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

func withCORS(next http.Handler, origins []string) http.Handler {
	allowAll := false
	allowed := map[string]bool{}
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "*" {
			allowAll = true
			continue
		}
		allowed[origin] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if allowAll || allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				if r.Method == http.MethodOptions {
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD")
					requested := r.Header.Get("Access-Control-Request-Headers")
					if requested == "" {
						requested = "Content-Type, Authorization"
					}
					w.Header().Set("Access-Control-Allow-Headers", requested)
					w.Header().Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

type rateLimiter struct {
	mutex    sync.Mutex
	counters map[string]*counter
	limit    int
	window   time.Duration
}

type counter struct {
	count      int
	windowFrom time.Time
}

func withRateLimit(next http.Handler, limit int, window time.Duration) http.Handler {
	limiter := &rateLimiter{
		counters: map[string]*counter{},
		limit:    limit,
		window:   window,
	}

	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			limiter.mutex.Lock()
			for key, value := range limiter.counters {
				if now.Sub(value.windowFrom) > 2*window {
					delete(limiter.counters, key)
				}
			}
			limiter.mutex.Unlock()
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limiter.allow(clientIP(r)) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
		http.Error(w, "too many requests", http.StatusTooManyRequests)
	})
}

func (l *rateLimiter) allow(key string) bool {
	now := time.Now()
	l.mutex.Lock()
	defer l.mutex.Unlock()

	value, ok := l.counters[key]
	if !ok || now.Sub(value.windowFrom) > l.window {
		l.counters[key] = &counter{count: 1, windowFrom: now}
		return true
	}
	value.count++
	return value.count <= l.limit
}

func tunnelErrorLog() *log.Logger {
	return log.New(tlsErrorWriter{}, "", 0)
}

type tlsErrorWriter struct{}

func (tlsErrorWriter) Write(data []byte) (int, error) {
	message := strings.TrimSpace(string(data))
	if message == "" {
		return len(data), nil
	}
	level, explanation := classifyTLSError(message)
	if level == "debug" {
		communication.TunnelDebug("", message+" ("+explanation+")")
	} else {
		communication.TunnelWarn("", message+" ("+explanation+")")
	}
	return len(data), nil
}

func classifyTLSError(message string) (string, string) {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "first record does not look like a tls handshake"),
		strings.Contains(lower, "bad record mac"),
		strings.Contains(lower, "unknown protocol"),
		strings.Contains(lower, "record header"),
		strings.Contains(lower, "received record with version"):
		return "debug", "a client spoke plain HTTP or used a broken TLS stack, the connection was dropped and the tunnel is still healthy"
	case strings.Contains(lower, "unsupported application protocols"),
		strings.Contains(lower, "client requested unsupported protocol"):
		return "debug", "client offered only non-supported ALPN protocols, retrying without negotiation"
	case strings.Contains(lower, "no cipher suite"),
		strings.Contains(lower, "unsupported versions"),
		strings.Contains(lower, "protocol version"):
		return "debug", "client uses an outdated TLS version, ask it to upgrade to TLS 1.2 or newer"
	case strings.Contains(lower, "missing certificate"),
		strings.Contains(lower, "certificate is not valid yet"),
		strings.Contains(lower, "acme"):
		return "debug", "the certificate for this hostname has not been issued yet, it will appear after the first successful validation"
	case strings.Contains(lower, "handshake failure"):
		return "debug", "TLS handshake failed, most likely a probe or a client without a shared cipher"
	case strings.Contains(lower, "too many"),
		strings.Contains(lower, "eof"):
		return "debug", "client closed the connection early"
	}
	return "warn", "unexpected TLS level error"
}

var _ = net.IPv4len
