package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/name-cho/newhole/internal/app/newhole/models"
)

type OAuthConfig struct {
	DeviceCodeURL string `json:"deviceCodeUrl"`
	TokenURL      string `json:"tokenUrl"`
	ClientID      string `json:"clientId"`
	Scope         string `json:"scope"`
	Audience      string `json:"audience"`
}

type DisplayConfig struct {
	Verbose bool `json:"verbose"`
	QR      bool `json:"qr"`
}

type TLSConfig struct {
	Insecure   bool
	CABundle   string
	ServerName string
}

type ACMEConfig struct {
	Directory string
	Email     string
}

type TunnelConfig struct {
	KeepAliveInterval string
	ReconnectMinDelay string
	ReconnectMaxDelay string
	DialTimeout       string
	ForwardTimeout    string
	MaxReconnects     int
}

type ApplicationConfig struct {
	Version    string `json:"version"`
	CommitHash string `json:"commitHash"`
	ClientMode string `json:"clientMode"`

	FeedbackFormURL string `json:"feedbackFormUrl"`

	OAuth   OAuthConfig   `json:"oauthConfig"`
	Display DisplayConfig `json:"displayConfig"`

	APIEndpoint     models.Endpoint `json:"apiConfig"`
	GatewayEndpoint models.Endpoint `json:"gatewayConfig"`

	AuthToken string `json:"-"`

	TLS  TLSConfig  `json:"tls"`
	ACME ACMEConfig `json:"acme"`

	Tunnels TunnelConfig `json:"tunnels"`

	HostKeyPolicy string `json:"hostKeyPolicy"`
	KnownHosts    string `json:"knownHosts"`
}

func env(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" && strings.HasPrefix(key, "NEWHOLE_") {
		value = strings.TrimSpace(os.Getenv("LOOPHOLE_" + strings.TrimPrefix(key, "NEWHOLE_")))
	}
	return value
}

func envBool(key string) bool {
	value := strings.ToLower(env(key))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func envInt(key string) int {
	value, err := strconv.Atoi(env(key))
	if err != nil {
		return 0
	}
	return value
}

func endpointFromEnv(value string) (models.Endpoint, bool) {
	if value == "" {
		return models.Endpoint{}, false
	}
	protocol := ""
	if index := strings.Index(value, "://"); index > 0 {
		protocol = value[:index]
		value = value[index+3:]
	}
	host := value
	port := 0
	if index := strings.LastIndex(value, ":"); index > 0 && !strings.Contains(value[index:], "]") {
		host = value[:index]
		port, _ = strconv.Atoi(value[index+1:])
	}
	return models.Endpoint{Protocol: protocol, Host: host, Port: int32(port)}, true
}

func applyEnv() {
	if endpoint, ok := endpointFromEnv(env("NEWHOLE_API_ENDPOINT")); ok {
		if endpoint.Protocol == "" {
			endpoint.Protocol = Config.APIEndpoint.Protocol
		}
		if endpoint.Port == 0 {
			endpoint.Port = Config.APIEndpoint.Port
		}
		Config.APIEndpoint = endpoint
	}
	if endpoint, ok := endpointFromEnv(env("NEWHOLE_GATEWAY_ENDPOINT")); ok {
		if endpoint.Protocol == "" {
			endpoint.Protocol = Config.GatewayEndpoint.Protocol
		}
		if endpoint.Port == 0 {
			endpoint.Port = Config.GatewayEndpoint.Port
		}
		Config.GatewayEndpoint = endpoint
	}
	if value := env("NEWHOLE_OAUTH_DEVICE_CODE_URL"); value != "" {
		Config.OAuth.DeviceCodeURL = value
	}
	if value := env("NEWHOLE_OAUTH_TOKEN_URL"); value != "" {
		Config.OAuth.TokenURL = value
	}
	if value := env("NEWHOLE_OAUTH_CLIENT_ID"); value != "" {
		Config.OAuth.ClientID = value
	}
	if value := env("NEWHOLE_OAUTH_SCOPE"); value != "" {
		Config.OAuth.Scope = value
	}
	if value := env("NEWHOLE_OAUTH_AUDIENCE"); value != "" {
		Config.OAuth.Audience = value
	}
	if value := env("NEWHOLE_TOKEN"); value != "" {
		Config.AuthToken = value
	}
	if value := env("NEWHOLE_CA_BUNDLE"); value != "" {
		Config.TLS.CABundle = value
	}
	if value := env("NEWHOLE_TLS_SERVER_NAME"); value != "" {
		Config.TLS.ServerName = value
	}
	if envBool("NEWHOLE_INSECURE_TLS") || envBool("NEWHOLE_INSECURE") {
		Config.TLS.Insecure = true
	}
	if value := env("NEWHOLE_ACME_DIRECTORY"); value != "" {
		Config.ACME.Directory = value
	}
	if value := env("NEWHOLE_ACME_EMAIL"); value != "" {
		Config.ACME.Email = value
	}
	if value := env("NEWHOLE_HOST_KEY_POLICY"); value != "" {
		Config.HostKeyPolicy = value
	}
	if value := env("NEWHOLE_KNOWN_HOSTS"); value != "" {
		Config.KnownHosts = value
	}
	if value := env("NEWHOLE_KEEPALIVE_INTERVAL"); value != "" {
		Config.Tunnels.KeepAliveInterval = value
	}
	if value := env("NEWHOLE_RECONNECT_MAX_DELAY"); value != "" {
		Config.Tunnels.ReconnectMaxDelay = value
	}
	if value := envInt("NEWHOLE_MAX_RECONNECTS"); value > 0 {
		Config.Tunnels.MaxReconnects = value
	}
	if envBool("NEWHOLE_VERBOSE") {
		Config.Display.Verbose = true
	}
}
