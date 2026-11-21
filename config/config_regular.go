//go:build !dev
// +build !dev

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
		Protocol: "https",
		Host:     "api.loophole.cloud",
		Port:     443,
	},
	GatewayEndpoint: models.Endpoint{
		Protocol: "ssh",
		Host:     "gateway.loophole.host",
		Port:     8022,
	},
	TLS: TLSConfig{
		Insecure:   false,
		ServerName: "",
	},
	ACME: ACMEConfig{
		Directory: "",
		Email:     "",
	},
	Tunnels: TunnelConfig{
		KeepAliveInterval: "30s",
		ReconnectMinDelay: "1s",
		ReconnectMaxDelay: "60s",
		DialTimeout:       "20s",
		ForwardTimeout:    "20s",
		MaxReconnects:     0,
	},
	HostKeyPolicy: "insecure",
}

func init() {
	applyEnv()
}
