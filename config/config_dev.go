//go:build dev
// +build dev

package config

import "github.com/name-cho/newhole/internal/app/newhole/models"

var Config = ApplicationConfig{
	Version:         "development",
	CommitHash:      "unknown",
	ClientMode:      "unknown",
	FeedbackFormURL: "https://github.com/name-cho/newhole/issues/new",

	OAuth: OAuthConfig{
		DeviceCodeURL: "https://loophole.eu.auth0.com/oauth/device/code",
		TokenURL:      "https://loophole.eu.auth0.com/oauth/token",
		ClientID:      "9ocnSAnfJSb6C52waL8xcPidCkRhUwBs",
		Scope:         "openid offline_access profile email",
		Audience:      "https://api.loophole.cloud",
	},
	Display: DisplayConfig{
		Verbose: false,
		QR:      false,
	},
	APIEndpoint: models.Endpoint{
		Protocol: "http",
		Host:     "api.loophole.local",
		Port:     3000,
	},
	GatewayEndpoint: models.Endpoint{
		Protocol: "ssh",
		Host:     "gateway.loophole.local",
		Port:     8022,
	},
	TLS: TLSConfig{
		Insecure:   true,
		ServerName: "",
	},
	ACME: ACMEConfig{
		Directory: "",
		Email:     "",
	},
	Tunnels: TunnelConfig{
		KeepAliveInterval: "30s",
		ReconnectMinDelay: "500ms",
		ReconnectMaxDelay: "30s",
		DialTimeout:       "10s",
		ForwardTimeout:    "10s",
		MaxReconnects:     0,
	},
	HostKeyPolicy: "insecure",
}

func init() {
	applyEnv()
}
