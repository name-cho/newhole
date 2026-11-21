package models

import "strings"

type RemoteEndpointSpecs struct {
	GatewayEndpoint       Endpoint `json:"gatewayEndpoint"`
	APIEndpoint           Endpoint `json:"apiEndpoint"`
	IdentityFile          string   `json:"identityFile"`
	SiteID                string   `json:"siteId"`
	Domain                string   `json:"domain"`
	TunnelID              string   `json:"tunnelId"`
	BasicAuthUsername     string   `json:"basicAuthUsername"`
	BasicAuthPassword     string   `json:"basicAuthPassword"`
	DisableProxyErrorPage bool     `json:"disableProxyErrorPage"`
	DisableOldCiphers     bool     `json:"disableOldCiphers"`

	OverrideHostHeader string `json:"overrideHostHeader"`
	PreserveHostHeader bool   `json:"preserveHostHeader"`

	ForwardedForHeader string `json:"forwardedForHeader"`
	RealIPHeader       string `json:"realIpHeader"`

	CORSAllowedOrigins []string `json:"corsAllowedOrigins"`

	MaxHeaderBytes  int   `json:"maxHeaderBytes"`
	MaxRequestBody  int64 `json:"maxRequestBody"`
	ReadHeaderBytes int64 `json:"readHeaderBytes"`

	Compress bool `json:"compress"`

	RateLimitRequests int `json:"rateLimitRequests"`
	RateLimitWindow   int `json:"rateLimitWindow"`

	LocalTLS bool `json:"localTls"`

	InsecureGateway bool   `json:"insecureGateway"`
	HostKeyPolicy   string `json:"hostKeyPolicy"`
	KnownHostsFile  string `json:"knownHostsFile"`

	Verbose bool `json:"verbose"`

	ExposeMode string `json:"exposeMode"`
}

func (remote *RemoteEndpointSpecs) NormalizeHostname() {
	remote.SiteID = strings.ToLower(strings.TrimSpace(remote.SiteID))
}
