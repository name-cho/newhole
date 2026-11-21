//go:build dev
// +build dev

package httpserver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"strings"
	"sync"
	"time"
)

var devCertificates sync.Map

func getTLSConfig(siteID string, domain string, disableOldCiphers bool) *tls.Config {
	host := fmt.Sprintf("%s.%s", siteID, domain)
	value, _ := devCertificates.LoadOrStore(host, newDevelopmentCertificate(host))
	certificate := value.(*tls.Certificate)
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2", "http/1.1"},
		GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return certificate, nil
		},
	}
	if disableOldCiphers {
		config.MinVersion = tls.VersionTLS12
	}
	return config
}

func newDevelopmentCertificate(host string) *tls.Certificate {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(err)
	}
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{"Newhole Development"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(30 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, candidate := range strings.Split(host, ",") {
		if ip := net.ParseIP(candidate); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
			continue
		}
		template.DNSNames = append(template.DNSNames, candidate)
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &private.PublicKey, private)
	if err != nil {
		panic(err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: private}
}
