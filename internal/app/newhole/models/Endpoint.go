package models

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type Endpoint struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int32  `json:"port"`
	Path     string `json:"path"`
}

func (endpoint *Endpoint) URI() string {
	return endpoint.String()
}

func (endpoint Endpoint) String() string {
	if endpoint.Protocol != "" {
		return fmt.Sprintf("%s://%s%s", endpoint.Protocol, endpoint.Hostname(), endpoint.Path)
	}
	return endpoint.Hostname()
}

func (endpoint Endpoint) IsIP() bool {
	host := endpoint.Host
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	return net.ParseIP(host) != nil
}

func (endpoint Endpoint) Hostname() string {
	host := strings.TrimSpace(endpoint.Host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if endpoint.Port <= 0 {
		return host
	}
	if endpoint.Protocol == "http" && endpoint.Port == 80 {
		return host
	}
	if endpoint.Protocol == "https" && endpoint.Port == 443 {
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(int(endpoint.Port)))
}

func (endpoint Endpoint) URL(siteID string, domain string) string {
	scheme := endpoint.Protocol
	if scheme == "" {
		scheme = "https"
	}
	fqdn := fmt.Sprintf("%s.%s", siteID, domain)
	if endpoint.Path != "" {
		return fmt.Sprintf("%s://%s%s", scheme, fqdn, endpoint.Path)
	}
	return fmt.Sprintf("%s://%s", scheme, fqdn)
}
