package communication

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/name-cho/newhole/config"
	coreModels "github.com/name-cho/newhole/internal/app/newhole/models"
	authModels "github.com/name-cho/newhole/internal/pkg/token/models"
)

var defaultLogger = NewStdOutLogger()
var communicationMechanism Mechanism = defaultLogger

type Mechanism interface {
	Debug(message string)
	Info(message string)
	Warn(message string)
	Error(message string)
	Fatal(message string)

	TunnelDebug(tunnelID string, message string)
	TunnelInfo(tunnelID string, message string)
	TunnelWarn(tunnelID string, message string)
	TunnelError(tunnelID string, message string)

	ApplicationStart(loggedIn bool, idToken string)
	ApplicationStop()

	TunnelStart(tunnelID string)

	TunnelStartSuccess(remoteConfig coreModels.RemoteEndpointSpecs, localEndpoint string)
	TunnelStartFailure(tunnelID string, err error)

	TunnelStopSuccess(tunnelID string)

	LoginStart(authModels.DeviceCodeSpec)
	LoginSuccess(idToken string)
	LoginFailure(err error)

	LogoutSuccess()
	LogoutFailure(err error)

	LoadingStart(tunnelID string, loaderMessage string)
	LoadingStartWithRequest(tunnelID string, loaderMessage string)
	LoadingSuccess(tunnelID string)
	LoadingSuccessWithRequest(tunnelID string)
	LoadingFailure(tunnelID string, err error)
	LoadingFailureWithRequest(tunnelID string, err error)

	NewVersionAvailable(availableVersion string)
}

func SetCommunicationMechanism(mechanism Mechanism) {
	if mechanism == nil {
		communicationMechanism = defaultLogger
		return
	}
	communicationMechanism = mechanism
}

func TunnelDebug(tunnelID string, message string) {
	communicationMechanism.TunnelDebug(tunnelID, message)
}

func TunnelInfo(tunnelID string, message string) {
	communicationMechanism.TunnelInfo(tunnelID, message)
}

func TunnelWarn(tunnelID string, message string) {
	communicationMechanism.TunnelWarn(tunnelID, message)
}

func TunnelError(tunnelID string, message string) {
	communicationMechanism.TunnelError(tunnelID, message)
}

func Debug(message string) {
	communicationMechanism.Debug(message)
}

func Info(message string) {
	communicationMechanism.Info(message)
}

func Warn(message string) {
	communicationMechanism.Warn(message)
}

func Error(message string) {
	communicationMechanism.Error(message)
}

func Fatal(message string) {
	communicationMechanism.Fatal(message)
}

func ApplicationStart(loggedIn bool, idToken string) {
	communicationMechanism.ApplicationStart(loggedIn, idToken)
}

func ApplicationStop() {
	communicationMechanism.ApplicationStop()
}

func ActiveTunnels() {
	if mechanism, ok := communicationMechanism.(interface{ ActiveTunnels() }); ok {
		mechanism.ActiveTunnels()
	}
}

func HostnameHistory() {
	if historyMechanism, ok := communicationMechanism.(interface{ HostnameHistory() }); ok {
		historyMechanism.HostnameHistory()
	}
}

func TunnelStart(tunnelID string) {
	communicationMechanism.TunnelStart(tunnelID)
}

func TunnelStartSuccess(remoteConfig coreModels.RemoteEndpointSpecs, localEndpoint string) {
	communicationMechanism.TunnelStartSuccess(remoteConfig, localEndpoint)
}

func TunnelStartFailure(tunnelID string, err error) {
	communicationMechanism.TunnelStartFailure(tunnelID, err)
}

func TunnelStopSuccess(tunnelID string) {
	communicationMechanism.TunnelStopSuccess(tunnelID)
}

func LoginStart(deviceCodeSpec authModels.DeviceCodeSpec) {
	communicationMechanism.LoginStart(deviceCodeSpec)
}

func LoginSuccess(idToken string) {
	communicationMechanism.LoginSuccess(idToken)
}

func LoginFailure(err error) {
	communicationMechanism.LoginFailure(err)
}

func LogoutSuccess() {
	communicationMechanism.LogoutSuccess()
}

func LogoutFailure(err error) {
	communicationMechanism.LogoutFailure(err)
}

func LoadingStart(tunnelID string, loaderMessage string) {
	communicationMechanism.LoadingStart(tunnelID, loaderMessage)
}

func LoadingStartWithRequest(tunnelID string, loaderMessage string) {
	communicationMechanism.LoadingStartWithRequest(tunnelID, loaderMessage)
}

func LoadingSuccess(tunnelID string) {
	communicationMechanism.LoadingSuccess(tunnelID)
}

func LoadingSuccessWithRequest(tunnelID string) {
	communicationMechanism.LoadingSuccessWithRequest(tunnelID)
}

func LoadingFailure(tunnelID string, err error) {
	communicationMechanism.LoadingFailure(tunnelID, err)
}

func LoadingFailureWithRequest(tunnelID string, err error) {
	communicationMechanism.LoadingFailureWithRequest(tunnelID, err)
}

func NewVersionAvailable(availableVersion string) {
	communicationMechanism.NewVersionAvailable(availableVersion)
}

var cachedRootCAs *x509.CertPool
var cachedRootCAsErr error
var rootCAsOnce sync.Once

func loadRootCAs() {
	bundle := config.Config.TLS.CABundle
	if bundle == "" {
		pool, err := x509.SystemCertPool()
		cachedRootCAs = pool
		cachedRootCAsErr = err
		return
	}
	data, err := os.ReadFile(bundle)
	if err != nil {
		cachedRootCAsErr = fmt.Errorf("unable to read CA bundle %s: %w", bundle, err)
		return
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		cachedRootCAsErr = fmt.Errorf("no certificates found in %s", bundle)
		return
	}
	cachedRootCAs = pool
}

func rootCAs() (*x509.CertPool, error) {
	rootCAsOnce.Do(loadRootCAs)
	return cachedRootCAs, cachedRootCAsErr
}

func TLSConfig(serverName string) *tls.Config {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
	}
	if config.Config.TLS.ServerName != "" {
		tlsConfig.ServerName = config.Config.TLS.ServerName
	}
	if config.Config.TLS.Insecure {
		tlsConfig.InsecureSkipVerify = true
	}
	if pool, err := rootCAs(); err == nil && pool != nil {
		tlsConfig.RootCAs = pool
	}
	return tlsConfig
}

func HTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       TLSConfig(""),
	}
}

func HTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: HTTPTransport(),
	}
}

func ExplainNetworkError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "no such host"):
		return "DNS lookup failed, check your network connection and DNS settings"
	case strings.Contains(message, "certificate has expired"), strings.Contains(message, "not yet valid"):
		return "The remote certificate is not valid right now, check your system clock or retry in a few minutes"
	case strings.Contains(message, "certificate signed by unknown authority"), strings.Contains(message, "x509: certificate"):
		return "TLS verification failed. If you are behind an intercepting proxy export NEWHOLE_CA_BUNDLE with its root CA"
	case strings.Contains(message, "connection refused"):
		return "Connection refused, the service is not listening on that address"
	case strings.Contains(message, "i/o timeout"), strings.Contains(message, "context deadline exceeded"):
		return "Request timed out, the network or the service is unresponsive"
	case strings.Contains(message, "connection reset"), strings.Contains(message, "unexpected eof"):
		return "Connection was reset by the peer"
	}
	return ""
}
