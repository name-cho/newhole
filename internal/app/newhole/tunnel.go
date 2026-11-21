package newhole

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/name-cho/newhole/config"
	lm "github.com/name-cho/newhole/internal/app/newhole/models"
	"github.com/name-cho/newhole/internal/pkg/apiclient"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/keys"
	"github.com/name-cho/newhole/internal/pkg/state"
	"golang.org/x/crypto/ssh"
)

type TunnelType string

const (
	HTTP      TunnelType = "Tunnel_HTTP"
	Directory TunnelType = "Tunnel_Directory"
	WebDav    TunnelType = "Tunnel_WebDav"
)

var remoteEndpoint = lm.Endpoint{
	Host: "127.0.0.1",
	Port: 80,
}

const (
	defaultReconnectMinDelay = time.Second
	defaultReconnectMaxDelay = 60 * time.Second
	defaultKeepAliveInterval = 30 * time.Second
	defaultDialTimeout       = 20 * time.Second
	copyBufferSize           = 32 * 1024
	listenAttempts           = 6
	certificateAttempts      = 4
	apiRegistrationAttempts  = 5
	initialDialAttempts      = 5
)

func durationFromConfig(value string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

type forwarder struct {
	remoteConfig lm.RemoteEndpointSpecs
	authMethod   ssh.AuthMethod
	dial         func() (net.Conn, error)
	displayAddr  string

	client   *ssh.Client
	listener net.Listener
	closed   atomic.Bool
}

func (f *forwarder) logDebug(format string, args ...interface{}) {
	communication.TunnelDebug(f.remoteConfig.TunnelID, fmt.Sprintf(format, args...))
}

func (f *forwarder) logInfo(format string, args ...interface{}) {
	communication.TunnelInfo(f.remoteConfig.TunnelID, fmt.Sprintf(format, args...))
}

func (f *forwarder) logWarn(format string, args ...interface{}) {
	communication.TunnelWarn(f.remoteConfig.TunnelID, fmt.Sprintf(format, args...))
}

func sshClientConfig(siteID string, authMethod ssh.AuthMethod) (*ssh.ClientConfig, error) {
	hostKeyCallback, err := keys.HostKeyCallback()
	if err != nil {
		return nil, err
	}
	return &ssh.ClientConfig{
		User:            siteID,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: hostKeyCallback,
		Timeout:         durationFromConfig(config.Config.Tunnels.DialTimeout, defaultDialTimeout),
		ClientVersion:   "SSH-2.0-newhole",
	}, nil
}

func dialSSH(remoteConfig lm.RemoteEndpointSpecs, authMethod ssh.AuthMethod, attempts int, stop func() bool) (*ssh.Client, error) {
	address := config.Config.GatewayEndpoint.Hostname()
	clientConfig, err := sshClientConfig(remoteConfig.SiteID, authMethod)
	if err != nil {
		return nil, err
	}

	minDelay := durationFromConfig(config.Config.Tunnels.ReconnectMinDelay, defaultReconnectMinDelay)
	maxDelay := durationFromConfig(config.Config.Tunnels.ReconnectMaxDelay, defaultReconnectMaxDelay)
	if maxDelay < minDelay {
		maxDelay = minDelay
	}
	maxAttempts := attempts

	delay := minDelay
	attempt := 0
	for {
		if stop != nil && stop() {
			return nil, errors.New("tunnel was closed")
		}
		attempt++
		communication.LoadingStartWithRequest(remoteConfig.TunnelID, "Initializing secure tunnel...")
		client, err := ssh.Dial("tcp", address, clientConfig)
		if err == nil {
			communication.LoadingSuccessWithRequest(remoteConfig.TunnelID)
			communication.TunnelDebug(remoteConfig.TunnelID, "Dialing SSH Gateway for HTTPS succeeded")
			return client, nil
		}
		communication.LoadingFailureWithRequest(remoteConfig.TunnelID, err)

		if maxAttempts > 0 && attempt >= maxAttempts {
			return nil, fmt.Errorf("unable to reach the SSH gateway at %s: %s", address, explainSSHError(err))
		}

		if maxAttempts <= 0 {
			communication.TunnelWarn(remoteConfig.TunnelID, fmt.Sprintf("%s. Retrying in %s (attempt %d)", explainSSHError(err), delay.Round(time.Millisecond), attempt))
		} else {
			communication.TunnelWarn(remoteConfig.TunnelID, fmt.Sprintf("%s. Retrying in %s (attempt %d of %d)", explainSSHError(err), delay.Round(time.Millisecond), attempt, maxAttempts))
		}
		if !sleepUnlessStopped(delay, stop) {
			return nil, errors.New("tunnel was closed")
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func sleepUnlessStopped(delay time.Duration, stop func() bool) bool {
	if stop == nil {
		time.Sleep(delay)
		return true
	}
	deadline := time.Now().Add(delay)
	for time.Now().Before(deadline) {
		if stop() {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	return !stop()
}

func explainSSHError(err error) string {
	if err == nil {
		return "unknown error"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unable to authenticate"):
		return "the gateway rejected the SSH key, log out and log in again with `newhole account login`"
	case strings.Contains(message, "connection refused"):
		return "the gateway refused the connection"
	case strings.Contains(message, "i/o timeout"), strings.Contains(message, "operation timed out"):
		return "the gateway is unreachable (timeout)"
	case strings.Contains(message, "no such host"), strings.Contains(message, "server misbehaving"):
		return "the gateway hostname cannot be resolved"
	case strings.Contains(message, "connection reset"), strings.Contains(message, "broken pipe"), strings.HasSuffix(message, "eof"):
		return "the connection to the gateway was dropped"
	case strings.Contains(message, "tcpip-forward request denied"):
		return "the gateway denied the port forwarding request"
	case strings.Contains(message, "unknown host key"), strings.Contains(message, "knownhosts"):
		return "the gateway host key is not trusted, update your known_hosts file"
	}
	return err.Error()
}

func handleClient(tunnelID string, client net.Conn, local net.Conn) {
	defer client.Close()
	defer local.Close()

	done := make(chan struct{}, 2)
	copyBoth := func(dst net.Conn, src net.Conn, direction string) {
		defer func() {
			done <- struct{}{}
		}()
		_, err := io.CopyBuffer(dst, src, make([]byte, copyBufferSize))
		if err != nil && !errors.Is(err, io.EOF) && !isClosedError(err) {
			communication.TunnelDebug(tunnelID, fmt.Sprintf("connection closed while copying %s: %s", direction, err.Error()))
		}
	}
	go copyBoth(client, local, "local -> client")
	go copyBoth(local, client, "client -> local")

	<-done
}

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "use of closed network connection") ||
		strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "aborted by the software in your host machine") ||
		strings.Contains(message, "forcibly closed")
}

func registerDomain(publicKey *ssh.PublicKey, requestedSiteID string, tunnelID string) (*apiclient.RegistrationSuccessResponse, error) {
	communication.LoadingStart(tunnelID, "Registering your domain...")
	registrationResult, err := apiclient.RegisterSite(*publicKey, requestedSiteID)
	if err != nil {
		communication.LoadingFailure(tunnelID, err)
		var requestErr apiclient.RequestError
		if errors.As(err, &requestErr) {
			if requestErr.Details != "" && requestErr.Details != requestErr.Message {
				communication.TunnelError(tunnelID, "details: "+requestErr.Details)
			}
			if requestErr.StatusCode >= 500 && requestErr.StatusCode != 0 {
				communication.TunnelError(tunnelID, "the Newhole API failed on its side, retrying the registration")
				return registerDomainWithRetry(publicKey, requestedSiteID, tunnelID)
			}
			if requestErr.Hint != "" {
				communication.TunnelError(tunnelID, requestErr.Hint)
			}
		}
		return nil, err
	}
	communication.LoadingSuccess(tunnelID)
	return registrationResult, nil
}

func registerDomainWithRetry(publicKey *ssh.PublicKey, requestedSiteID string, tunnelID string) (*apiclient.RegistrationSuccessResponse, error) {
	minDelay := durationFromConfig(config.Config.Tunnels.ReconnectMinDelay, defaultReconnectMinDelay)
	delay := minDelay
	var lastErr error
	for attempt := 1; attempt <= apiRegistrationAttempts; attempt++ {
		registrationResult, err := apiclient.RegisterSite(*publicKey, requestedSiteID)
		if err == nil {
			communication.LoadingSuccess(tunnelID)
			return registrationResult, nil
		}
		var requestErr apiclient.RequestError
		if !errors.As(err, &requestErr) || requestErr.StatusCode < 500 || requestErr.StatusCode == 0 {
			return nil, err
		}
		lastErr = err
		if attempt < apiRegistrationAttempts {
			communication.TunnelWarn(tunnelID, fmt.Sprintf("the registration failed with status %d, retrying in %s (attempt %d of %d)", requestErr.StatusCode, delay.Round(time.Millisecond), attempt, apiRegistrationAttempts))
			time.Sleep(delay)
			delay *= 2
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
		}
	}
	return nil, lastErr
}

func parsePublicKey(tunnelID string, identityFile string) (ssh.AuthMethod, ssh.PublicKey, error) {
	publicKeyAuthMethod, publicKey, err := keys.ParsePublicKey(identityFile)
	if err != nil {
		communication.LoadingFailure(tunnelID, err)
		communication.TunnelError(tunnelID, "no usable SSH key available: "+err.Error())
		return nil, nil, err
	}
	return publicKeyAuthMethod, publicKey, nil
}

func RegisterTunnel(remoteConfig *lm.RemoteEndpointSpecs) (ssh.AuthMethod, error) {
	if remoteConfig == nil {
		return nil, errors.New("missing remote endpoint configuration")
	}
	publicKeyAuthMethod, publicKey, err := parsePublicKey(remoteConfig.TunnelID, remoteConfig.IdentityFile)
	if err != nil {
		return nil, err
	}
	registrationResult, err := registerDomain(&publicKey, remoteConfig.SiteID, remoteConfig.TunnelID)
	if err != nil {
		return nil, err
	}
	if registrationResult.SiteID == "" {
		return nil, errors.New("the registration response did not contain a site id")
	}
	remoteConfig.SiteID = strings.ToLower(registrationResult.SiteID)
	if registrationResult.Domain != "" {
		remoteConfig.Domain = registrationResult.Domain
	}
	if err := state.RecordHostname(remoteConfig.SiteID, remoteConfig.Domain, remoteConfig.ExposeMode); err != nil {
		communication.TunnelDebug(remoteConfig.TunnelID, "unable to remember the hostname: "+err.Error())
	}
	communication.TunnelStart(remoteConfig.TunnelID)
	return publicKeyAuthMethod, nil
}

func startListener(client *ssh.Client, tunnelID string) (net.Listener, error) {
	requestedAddress := remoteEndpoint.Hostname()
	minDelay := durationFromConfig(config.Config.Tunnels.ReconnectMinDelay, defaultReconnectMinDelay)
	delay := minDelay
	var lastErr error
	timeout := durationFromConfig(config.Config.Tunnels.DialTimeout, defaultDialTimeout)
	for attempt := 1; attempt <= listenAttempts; attempt++ {
		listener, err := listenWithTimeout(client, requestedAddress, timeout)
		if err == nil {
			communication.TunnelDebug(tunnelID, "Listening on remote endpoint for HTTPS succeeded")
			return listener, nil
		}
		lastErr = err
		message := err.Error()
		switch {
		case strings.Contains(message, "tcpip-forward request denied"):
			communication.TunnelWarn(tunnelID, "the gateway denied the forwarded port, another copy of the tunnel may still hold it, retrying...")
		case strings.Contains(message, "address already in use"):
			communication.TunnelWarn(tunnelID, "the remote port is busy, retrying...")
		default:
			communication.TunnelWarn(tunnelID, "listening on the remote endpoint failed: "+message)
		}
		time.Sleep(delay)
		delay *= 2
		if delay > 10*time.Second {
			delay = 10 * time.Second
		}
	}
	if lastErr == nil {
		lastErr = errors.New("unknown error")
	}
	communication.TunnelError(tunnelID, "unable to listen on the gateway: "+explainSSHError(lastErr))
	return nil, lastErr
}

type listenResult struct {
	listener net.Listener
	err      error
}

func listenWithTimeout(client *ssh.Client, address string, timeout time.Duration) (net.Listener, error) {
	results := make(chan listenResult, 1)
	go func() {
		listener, err := client.Listen("tcp", address)
		results <- listenResult{listener: listener, err: err}
	}()

	select {
	case result := <-results:
		return result.listener, result.err
	case <-time.After(timeout):
		go func() {
			result := <-results
			if result.listener != nil {
				result.listener.Close()
			}
		}()
		return nil, fmt.Errorf("the gateway did not answer the port forwarding request within %s", timeout)
	}
}

func (f *forwarder) connect() error {
	attempts := config.Config.Tunnels.MaxReconnects
	if attempts <= 0 {
		attempts = initialDialAttempts
	}
	return f.connectWithAttempts(attempts)
}

func (f *forwarder) connectForever() error {
	return f.connectWithAttempts(0)
}

func (f *forwarder) connectWithAttempts(attempts int) error {
	client, err := dialSSH(f.remoteConfig, f.authMethod, attempts, f.closed.Load)
	if err != nil {
		return err
	}
	listener, err := startListener(client, f.remoteConfig.TunnelID)
	if err != nil {
		client.Close()
		return err
	}
	f.client = client
	f.listener = listener
	return nil
}

func (f *forwarder) teardown() {
	if f.listener != nil {
		f.listener.Close()
		f.listener = nil
	}
	if f.client != nil {
		f.client.Close()
		f.client = nil
	}
}

func (f *forwarder) keepAlive() {
	interval := durationFromConfig(config.Config.Tunnels.KeepAliveInterval, defaultKeepAliveInterval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if f.closed.Load() {
			return
		}
		client := f.client
		if client == nil {
			continue
		}
		if _, _, err := client.SendRequest("keepalive@openssh.com", false, nil); err != nil {
			f.logWarn("keepalive failed: %s", err.Error())
			client.Close()
		}
	}
}

func (f *forwarder) acceptLoop() {
	for {
		if f.closed.Load() {
			return
		}
		listener := f.listener
		if listener == nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		conn, err := listener.Accept()
		if err != nil {
			if f.closed.Load() {
				return
			}
			if errors.Is(err, io.EOF) || isClosedError(err) || strings.Contains(strings.ToLower(err.Error()), "eof") {
				f.logInfo("connection dropped by the gateway, reconnecting...")
			} else {
				f.logWarn("failed to accept a connection over HTTPS: %s", err.Error())
			}
			f.teardown()
			if f.reconnectWithBackoff() != nil {
				return
			}
			continue
		}
		go f.serve(conn)
	}
}

func (f *forwarder) reconnectWithBackoff() error {
	minDelay := durationFromConfig(config.Config.Tunnels.ReconnectMinDelay, defaultReconnectMinDelay)
	maxDelay := durationFromConfig(config.Config.Tunnels.ReconnectMaxDelay, defaultReconnectMaxDelay)
	delay := minDelay
	for {
		if f.closed.Load() {
			return errors.New("tunnel was closed")
		}
		if err := f.connectForever(); err == nil {
			f.logInfo("tunnel re-established")
			return nil
		} else if f.closed.Load() {
			return err
		} else {
			f.logWarn("reconnect attempt failed: %s", explainSSHError(err))
		}
		time.Sleep(delay)
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

func (f *forwarder) serve(conn net.Conn) {
	timeout := durationFromConfig(config.Config.Tunnels.DialTimeout, defaultDialTimeout)
	_ = conn.SetDeadline(time.Now().Add(timeout))
	local, err := f.dial()
	if err != nil {
		f.logWarn("unable to reach the local service: %s", err.Error())
		conn.Close()
		return
	}
	_ = conn.SetDeadline(time.Time{})
	handleClient(f.remoteConfig.TunnelID, conn, local)
}

func (f *forwarder) run(quitChannel <-chan bool) error {
	stopped := make(chan struct{})
	go func() {
		<-quitChannel
		f.closed.Store(true)
		f.teardown()
		close(stopped)
	}()

	if err := f.connect(); err != nil {
		if f.closed.Load() {
			<-stopped
			communication.TunnelStopSuccess(f.remoteConfig.TunnelID)
			return nil
		}
		return err
	}
	if f.closed.Load() {
		<-stopped
		return nil
	}

	go f.keepAlive()
	go f.acceptLoop()

	communication.TunnelStartSuccess(f.remoteConfig, f.displayAddr)

	go func() {
		provisionCertificate(f.remoteConfig)
	}()

	<-stopped
	communication.TunnelStopSuccess(f.remoteConfig.TunnelID)
	return nil
}

func provisionCertificate(remoteConfig lm.RemoteEndpointSpecs) {
	communication.TunnelDebug(remoteConfig.TunnelID, "issuing a request to provision the certificate")
	target := fmt.Sprintf("https://%s.%s", remoteConfig.SiteID, remoteConfig.Domain)

	var lastErr error
	for attempt := 1; attempt <= certificateAttempts; attempt++ {
		client := communication.HTTPClient(30 * time.Second)
		response, err := client.Get(target)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<16))
			response.Body.Close()
			communication.TunnelInfo(remoteConfig.TunnelID, "TLS certificate successfully provisioned")
			return
		}
		lastErr = err
		if !isCertificateError(err) {
			break
		}
		communication.TunnelDebug(remoteConfig.TunnelID, fmt.Sprintf("certificate attempt %d failed: %s", attempt, err.Error()))
		if attempt < certificateAttempts {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
	}

	if lastErr != nil && isCertificateError(lastErr) {
		communication.TunnelInfo(remoteConfig.TunnelID, "the TLS certificate is still being issued, the first client request may be slower")
		communication.TunnelDebug(remoteConfig.TunnelID, "give the certificate authority a minute, then open "+target+" in a browser; if the browser keeps reporting an invalid certificate the hostname does not resolve to the Newhole gateway")
		return
	}
	communication.TunnelDebug(remoteConfig.TunnelID, "provisioning request failed: "+lastErr.Error())
	communication.TunnelInfo(remoteConfig.TunnelID, "the TLS certificate is still being issued ("+lastErr.Error()+"), the first client request may be slower")
}

func isCertificateError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "certificate") || strings.Contains(message, "x509") || strings.Contains(message, "tls")
}
