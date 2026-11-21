//go:build !dev
// +build !dev

package httpserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"

	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/internal/pkg/cache"
	"github.com/name-cho/newhole/internal/pkg/urlmaker"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

var certManagers = struct {
	managers map[string]*autocert.Manager
}{managers: map[string]*autocert.Manager{}}

func certCacheDirectory() string {
	return cache.GetLocalStorageDir("certs")
}

func newCertManager(siteID string, domain string) *autocert.Manager {
	host := urlmaker.GetSiteFQDN(siteID, domain)
	key := host
	if manager, ok := certManagers.managers[key]; ok {
		return manager
	}

	email := config.Config.ACME.Email
	if email == "" {
		email = fmt.Sprintf("newhole-%s@invalid.newhole.local", siteID)
	}

	directory := config.Config.ACME.Directory
	if directory == "" {
		directory = acme.LetsEncryptURL
	}

	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(host),
		Cache:      certCacheFor(siteID, domain),
		Email:      email,
		Client:     &acme.Client{DirectoryURL: directory},
	}
	certManagers.managers[key] = manager
	return manager
}

type fallbackCache struct {
	primary  autocert.Cache
	fallback autocert.Cache
}

func (c fallbackCache) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := c.primary.Get(ctx, key)
	if err == nil {
		return data, nil
	}
	return c.fallback.Get(ctx, key)
}

func (c fallbackCache) Put(ctx context.Context, key string, data []byte) error {
	err := c.primary.Put(ctx, key, data)
	if err != nil {
		_ = c.fallback.Put(ctx, key, data)
	}
	return err
}

func (c fallbackCache) Delete(ctx context.Context, key string) error {
	_ = c.fallback.Delete(ctx, key)
	return c.primary.Delete(ctx, key)
}

func certCacheFor(siteID string, domain string) autocert.Cache {
	primary := autocert.DirCache(filepath.Join(certCacheDirectory(), urlmaker.GetSiteFQDN(siteID, domain)))
	home, err := os.UserHomeDir()
	if err != nil {
		return primary
	}
	legacy := filepath.Join(home, ".loophole", "certs")
	if _, statErr := os.Stat(legacy); statErr != nil {
		return primary
	}
	return fallbackCache{primary: primary, fallback: autocert.DirCache(legacy)}
}

func getTLSConfig(siteID string, domain string, disableOldCiphers bool) *tls.Config {
	manager := newCertManager(siteID, domain)
	tlsConfig := manager.TLSConfig()
	tlsConfig.NextProtos = []string{"h2", "http/1.1", acme.ALPNProto}
	tlsConfig.MinVersion = tls.VersionTLS12
	if !disableOldCiphers {
		tlsConfig.CipherSuites = defaultCipherSuites()
	}
	return tlsConfig
}

func defaultCipherSuites() []uint16 {
	return []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}
}
