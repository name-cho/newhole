package newhole

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"golang.org/x/crypto/ssh"

	"github.com/blang/semver/v4"
	lm "github.com/name-cho/newhole/internal/app/newhole/models"
	"github.com/name-cho/newhole/internal/pkg/apiclient"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/httpserver"
)

func newForwarder(remoteConfig lm.RemoteEndpointSpecs, authMethod ssh.AuthMethod, dial func() (net.Conn, error), displayAddr string) *forwarder {
	return &forwarder{
		remoteConfig: remoteConfig,
		authMethod:   authMethod,
		dial:         dial,
		displayAddr:  displayAddr,
	}
}

func localHTTPDialer(specs lm.LocalHTTPEndpointSpecs, tunnelID string) func() (net.Conn, error) {
	if specs.UnixSocket != "" {
		return func() (net.Conn, error) {
			conn, err := net.Dial("unix", specs.UnixSocket)
			if err != nil {
				communication.TunnelError(tunnelID, fmt.Sprintf("unable to reach the unix socket %s: %s", specs.UnixSocket, err.Error()))
			}
			return conn, err
		}
	}
	host := specs.Host
	if host == "" {
		host = "127.0.0.1"
	}
	endpoint := lm.Endpoint{Host: host, Port: specs.Port}
	address := endpoint.Hostname()
	return func() (net.Conn, error) {
		conn, err := net.Dial("tcp", address)
		if err != nil {
			communication.TunnelError(tunnelID, fmt.Sprintf("unable to reach the local service at %s: %s", address, err.Error()))
		}
		return conn, err
	}
}

func createTLSReverseProxy(localEndpoint lm.Endpoint, remoteConfig lm.RemoteEndpointSpecs, dialer func() (net.Conn, error)) (*http.Server, error) {
	communication.LoadingStart(remoteConfig.TunnelID, "Starting local TLS proxy server")
	serverBuilder := httpserver.New().
		WithSiteID(remoteConfig.SiteID).
		WithDomain(remoteConfig.Domain).
		DisableOldCiphers(remoteConfig.DisableOldCiphers).
		WithRemoteSpecs(remoteConfig).
		Proxy().
		ToEndpoint(localEndpoint).
		WithDialer(dialer)

	if remoteConfig.BasicAuthUsername != "" || remoteConfig.BasicAuthPassword != "" {
		serverBuilder = serverBuilder.WithBasicAuth(remoteConfig.BasicAuthUsername, remoteConfig.BasicAuthPassword)
	}
	if remoteConfig.DisableProxyErrorPage {
		serverBuilder = serverBuilder.DisableProxyErrorPage()
	}
	if localEndpoint.Protocol == "https" {
		serverBuilder = serverBuilder.EnableInsecureHTTPSBackend()
	}

	server, err := serverBuilder.Build()
	if err != nil {
		communication.LoadingFailure(remoteConfig.TunnelID, err)
		communication.TunnelError(remoteConfig.TunnelID, "unable to build the HTTPS proxy")
		communication.TunnelStartFailure(remoteConfig.TunnelID, err)
		return nil, err
	}
	communication.LoadingSuccess(remoteConfig.TunnelID)
	communication.TunnelDebug(remoteConfig.TunnelID, fmt.Sprintf("proxy to %s created", localEndpoint.String()))
	return server, nil
}

func startLocalHTTPServer(tunnelID string, server *http.Server) (*lm.Endpoint, error) {
	communication.LoadingStart(tunnelID, "Starting local proxy server...")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		communication.LoadingFailure(tunnelID, err)
		communication.TunnelError(tunnelID, "unable to listen on a local port for the TLS proxy: "+err.Error())
		return nil, err
	}
	endpoint := &lm.Endpoint{
		Host: "127.0.0.1",
		Port: int32(listener.Addr().(*net.TCPAddr).Port),
	}

	go func() {
		if err := server.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
			communication.TunnelDebug(tunnelID, "local TLS proxy stopped: "+err.Error())
		}
	}()

	communication.LoadingSuccess(tunnelID)
	communication.TunnelDebug(tunnelID, fmt.Sprintf("proxy listener for HTTPS started on port %d", endpoint.Port))
	return endpoint, nil
}

func startServerAndForward(remoteConfig lm.RemoteEndpointSpecs, authMethod ssh.AuthMethod, server *http.Server, displayAddr string, quitChannel <-chan bool) error {
	endpoint, err := startLocalHTTPServer(remoteConfig.TunnelID, server)
	if err != nil {
		return err
	}
	fwd := newForwarder(remoteConfig, authMethod, localDialerTo(endpoint), displayAddr)
	return fwd.run(quitChannel)
}

func localDialerTo(endpoint *lm.Endpoint) func() (net.Conn, error) {
	return func() (net.Conn, error) {
		return net.Dial("tcp", endpoint.Hostname())
	}
}

func ForwardPort(exposeHTTPConfig lm.ExposeHTTPConfig, publicKeyAuthMethod ssh.AuthMethod, quitChannel <-chan bool) error {
	protocol := "http"
	if exposeHTTPConfig.Local.HTTPS {
		protocol = "https"
	}
	localEndpoint := lm.Endpoint{
		Protocol: protocol,
		Host:     exposeHTTPConfig.Local.Host,
		Port:     exposeHTTPConfig.Local.Port,
		Path:     exposeHTTPConfig.Local.Path,
	}
	if localEndpoint.Host == "" {
		localEndpoint.Host = "127.0.0.1"
	}

	dialer := localHTTPDialer(exposeHTTPConfig.Local, exposeHTTPConfig.Remote.TunnelID)
	server, err := createTLSReverseProxy(localEndpoint, exposeHTTPConfig.Remote, dialer)
	if err != nil {
		return err
	}
	return startServerAndForward(exposeHTTPConfig.Remote, publicKeyAuthMethod, server, localEndpoint.String(), quitChannel)
}

func ForwardDirectory(exposeDirectoryConfig lm.ExposeDirectoryConfig, publicKeyAuthMethod ssh.AuthMethod, quitChannel <-chan bool) error {
	return forwardDirectory(exposeDirectoryConfig, publicKeyAuthMethod, quitChannel, exposeDirectoryConfig.Local.DirectoryListing)
}

func ForwardDirectoryViaWebdav(exposeWebdavConfig lm.ExposeWebdavConfig, publicKeyAuthMethod ssh.AuthMethod, quitChannel <-chan bool) error {
	communication.LoadingStart(exposeWebdavConfig.Remote.TunnelID, "Starting WebDAV server")
	serverBuilder := httpserver.New().
		WithSiteID(exposeWebdavConfig.Remote.SiteID).
		WithDomain(exposeWebdavConfig.Remote.Domain).
		DisableOldCiphers(exposeWebdavConfig.Remote.DisableOldCiphers).
		WithRemoteSpecs(exposeWebdavConfig.Remote).
		ServeWebdav().
		FromDirectory(exposeWebdavConfig.Local.Path)

	if exposeWebdavConfig.Remote.BasicAuthUsername != "" || exposeWebdavConfig.Remote.BasicAuthPassword != "" {
		serverBuilder = serverBuilder.WithBasicAuth(exposeWebdavConfig.Remote.BasicAuthUsername, exposeWebdavConfig.Remote.BasicAuthPassword)
	}

	server, err := serverBuilder.Build()
	if err != nil {
		communication.LoadingFailure(exposeWebdavConfig.Remote.TunnelID, err)
		communication.TunnelError(exposeWebdavConfig.Remote.TunnelID, "unable to build the WebDAV server: "+err.Error())
		return err
	}
	communication.LoadingSuccess(exposeWebdavConfig.Remote.TunnelID)
	return startServerAndForward(exposeWebdavConfig.Remote, publicKeyAuthMethod, server, exposeWebdavConfig.Local.Path, quitChannel)
}

func forwardDirectory(config lm.ExposeDirectoryConfig, publicKeyAuthMethod ssh.AuthMethod, quitChannel <-chan bool, listing bool) error {
	communication.LoadingStart(config.Remote.TunnelID, "Starting local file server")
	serverBuilder := httpserver.New().
		WithSiteID(config.Remote.SiteID).
		WithDomain(config.Remote.Domain).
		DisableOldCiphers(config.Remote.DisableOldCiphers).
		WithRemoteSpecs(config.Remote).
		ServeStatic().
		FromDirectory(config.Local.Path).
		WithDirectoryListing(listing)

	if config.Remote.BasicAuthUsername != "" || config.Remote.BasicAuthPassword != "" {
		serverBuilder = serverBuilder.WithBasicAuth(config.Remote.BasicAuthUsername, config.Remote.BasicAuthPassword)
	}

	server, err := serverBuilder.Build()
	if err != nil {
		communication.LoadingFailure(config.Remote.TunnelID, err)
		communication.TunnelError(config.Remote.TunnelID, "unable to build the file server: "+err.Error())
		return err
	}
	communication.LoadingSuccess(config.Remote.TunnelID)
	return startServerAndForward(config.Remote, publicKeyAuthMethod, server, config.Local.Path, quitChannel)
}

func CheckVersion() {
	available, err := apiclient.GetLatestAvailableVersion()
	if err != nil {
		communication.Debug("unable to check for updates: " + err.Error())
		return
	}
	current, err := semver.Make(normalizeVersion(configVersion))
	if err != nil {
		communication.Debug(fmt.Sprintf("current version %q is not semver, skipping the update check", configVersion))
		return
	}
	availableVersion, err := semver.Make(normalizeVersion(available.Version))
	if err != nil {
		communication.Debug(fmt.Sprintf("available version %q is not semver, skipping the update check", available.Version))
		return
	}
	if current.LT(availableVersion) {
		communication.NewVersionAvailable(available.Version)
	}
}

var configVersion = "0.0.0"

func SetVersion(version string) {
	configVersion = version
}

func normalizeVersion(value string) string {
	cleaned := value
	if len(cleaned) > 0 && (cleaned[0] == 'v' || cleaned[0] == 'V') {
		cleaned = cleaned[1:]
	}
	if cleaned == "" {
		return value
	}
	if _, err := strconv.Atoi(string(cleaned[0])); err != nil {
		return cleaned
	}
	return cleaned
}
