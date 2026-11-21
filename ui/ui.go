//go:build desktop
// +build desktop

package ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ncruces/zenity"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/skratchdot/open-golang/open"
	"github.com/zserge/lorca"

	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/internal/app/newhole"
	lm "github.com/name-cho/newhole/internal/app/newhole/models"
	"github.com/name-cho/newhole/internal/pkg/cache"
	"github.com/name-cho/newhole/internal/pkg/closehandler"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/state"
	"github.com/name-cho/newhole/internal/pkg/token"
	"golang.org/x/crypto/ssh"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return origin == "http://"+r.Host || origin == "https://"+r.Host
	},
}

var tunnelsMutex sync.Mutex

var tunnelQuitChannels = make(map[string]chan bool)
var siteToRequestMapping = make(map[string]string)

var loginMutex sync.Mutex
var loginCancel chan bool

func lockTunnels() func() {
	tunnelsMutex.Lock()
	return tunnelsMutex.Unlock
}

func reserveSite(siteID string, tunnelID string) error {
	if siteID == "" {
		return nil
	}
	if _, ok := siteToRequestMapping[siteID]; ok {
		return fmt.Errorf("a tunnel for hostname %q is already running", siteID)
	}
	siteToRequestMapping[siteID] = tunnelID
	return nil
}

func releaseTunnel(tunnelID string) {
	for siteID, mapped := range siteToRequestMapping {
		if mapped == tunnelID {
			delete(siteToRequestMapping, siteID)
		}
	}
	delete(tunnelQuitChannels, tunnelID)
}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	communication.SetCommunicationMechanism(communication.NewWebsocketLogger(c))
	defer communication.SetCommunicationMechanism(communication.NewStdOutLogger())

	communication.ApplicationStart(token.IsTokenSaved(), token.GetIdToken())
	communication.HostnameHistory()
	communication.ActiveTunnels()

	closed := make(chan struct{})
	go func() {
		<-closed
		tunnelsMutex.Lock()
		pending := make([]string, 0, len(tunnelQuitChannels))
		for tunnelID := range tunnelQuitChannels {
			pending = append(pending, tunnelID)
		}
		tunnelsMutex.Unlock()
		for _, tunnelID := range pending {
			stopTunnel(tunnelID)
		}
	}()
	defer close(closed)

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			break
		}
		var decodedMessage Message
		if err := json.Unmarshal(message, &decodedMessage); err != nil {
			communication.Warn("Unable to decode the incoming message: " + err.Error())
			continue
		}
		handleMessage(decodedMessage)
	}
	communication.ApplicationStop()
}

func handleMessage(decodedMessage Message) {
	switch decodedMessage.Type {
	case MessageTypeStartTunnelHTTP:
		var exposeHTTPConfig lm.ExposeHTTPConfig
		if err := json.Unmarshal(decodedMessage.Payload, &exposeHTTPConfig); err != nil {
			communication.Warn("Unable to decode the HTTP tunnel configuration: " + err.Error())
			return
		}
		rememberTunnel("http", exposeHTTPConfig.Remote.TunnelID, exposeHTTPConfig)
		startTunnel(&exposeHTTPConfig.Remote, func(authMethod ssh.AuthMethod) error {
			updateRememberedTunnel("http", exposeHTTPConfig, &exposeHTTPConfig.Remote)
			return newhole.ForwardPort(exposeHTTPConfig, authMethod, quit(exposeHTTPConfig.Remote.TunnelID))
		})
	case MessageTypeStartTunnelDirectory:
		var exposeDirectoryConfig lm.ExposeDirectoryConfig
		if err := json.Unmarshal(decodedMessage.Payload, &exposeDirectoryConfig); err != nil {
			communication.Warn("Unable to decode the directory tunnel configuration: " + err.Error())
			return
		}
		rememberTunnel("path", exposeDirectoryConfig.Remote.TunnelID, exposeDirectoryConfig)
		startTunnel(&exposeDirectoryConfig.Remote, func(authMethod ssh.AuthMethod) error {
			updateRememberedTunnel("path", exposeDirectoryConfig, &exposeDirectoryConfig.Remote)
			return newhole.ForwardDirectory(exposeDirectoryConfig, authMethod, quit(exposeDirectoryConfig.Remote.TunnelID))
		})
	case MessageTypeStartTunnelWebDav:
		var exposeWebdavConfig lm.ExposeWebdavConfig
		if err := json.Unmarshal(decodedMessage.Payload, &exposeWebdavConfig); err != nil {
			communication.Warn("Unable to decode the WebDAV tunnel configuration: " + err.Error())
			return
		}
		rememberTunnel("webdav", exposeWebdavConfig.Remote.TunnelID, exposeWebdavConfig)
		startTunnel(&exposeWebdavConfig.Remote, func(authMethod ssh.AuthMethod) error {
			updateRememberedTunnel("webdav", exposeWebdavConfig, &exposeWebdavConfig.Remote)
			return newhole.ForwardDirectoryViaWebdav(exposeWebdavConfig, authMethod, quit(exposeWebdavConfig.Remote.TunnelID))
		})
	case MessageTypeActiveTunnels:
		communication.ActiveTunnels()
	case MessageTypeRefreshHistory:
		communication.HostnameHistory()
	case MessageTypeForgetHostname:
		var forgetHostnameMessage ForgetHostnameMessage
		if err := json.Unmarshal(decodedMessage.Payload, &forgetHostnameMessage); err != nil {
			communication.Warn("Unable to decode the forget hostname message: " + err.Error())
			return
		}
		if err := state.ForgetHostname(forgetHostnameMessage.Hostname); err != nil {
			communication.Warn("Unable to forget " + forgetHostnameMessage.Hostname + ": " + err.Error())
			return
		}
		communication.HostnameHistory()
	case MessageTypeStopTunnel:
		var stopTunnelMessage StopTunnelMessage
		if err := json.Unmarshal(decodedMessage.Payload, &stopTunnelMessage); err != nil {
			communication.Warn("Unable to decode the stop tunnel message: " + err.Error())
			return
		}
		stopTunnel(stopTunnelMessage.TunnelID)
	case MessageTypeAuthorization:
		startLogin()
	case MessageTypeLogout:
		startLogout()
	case MessageTypeOpenBrowser:
		var openInBrowserMessage OpenInBrowserMessage
		if err := json.Unmarshal(decodedMessage.Payload, &openInBrowserMessage); err != nil {
			communication.Warn("Unable to decode the open browser message: " + err.Error())
			return
		}
		go func() {
			if err := open.Run(openInBrowserMessage.URL); err != nil {
				communication.Warn("Unable to open " + openInBrowserMessage.URL + ": " + err.Error())
			}
		}()
	case MessageTypePing:
		communication.Debug("ping")
	default:
		communication.Warn(fmt.Sprintf("Unrecognized message type %q", decodedMessage.Type))
	}
}

func quit(tunnelID string) <-chan bool {
	tunnelsMutex.Lock()
	defer tunnelsMutex.Unlock()
	channel, ok := tunnelQuitChannels[tunnelID]
	if !ok {
		channel = make(chan bool, 1)
		tunnelQuitChannels[tunnelID] = channel
	}
	return channel
}

func rememberTunnel(mode string, tunnelID string, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		communication.Debug("unable to encode the tunnel: " + err.Error())
		return
	}
	rememberTunnelRaw(mode, tunnelID, encoded)
}

func rememberTunnelRaw(mode string, tunnelID string, payload json.RawMessage) {
	if err := state.SaveActiveTunnel(state.ActiveTunnel{TunnelID: tunnelID, Mode: mode, Payload: payload}); err != nil {
		communication.Debug("unable to remember the tunnel: " + err.Error())
	}
}

func updateRememberedTunnel(mode string, payload any, remote *lm.RemoteEndpointSpecs) {
	if remote == nil {
		return
	}
	if data, err := json.Marshal(struct {
		Local  json.RawMessage        `json:"local"`
		Remote lm.RemoteEndpointSpecs `json:"remote"`
	}{Local: localPayload(payload), Remote: *remote}); err == nil {
		rememberTunnelRaw(mode, remote.TunnelID, data)
	}
}

func localPayload(payload any) json.RawMessage {
	var wrapper struct {
		Local json.RawMessage `json:"local"`
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage("{}")
	}
	if err := json.Unmarshal(encoded, &wrapper); err != nil {
		return json.RawMessage("{}")
	}
	return wrapper.Local
}

func forgetTunnel(tunnelID string) {
	if err := state.RemoveActiveTunnel(tunnelID); err != nil {
		communication.Debug("unable to forget the tunnel: " + err.Error())
	}
}

func startTunnel(remote *lm.RemoteEndpointSpecs, forward func(ssh.AuthMethod) error) {
	go func() {
		unlock := lockTunnels()
		sshDir := cache.GetLocalStorageDir(".ssh")
		remote.IdentityFile = fmt.Sprintf("%s/id_rsa", sshDir)
		remote.NormalizeHostname()
		if err := reserveSite(remote.SiteID, remote.TunnelID); err != nil {
			unlock()
			communication.TunnelStartFailure(remote.TunnelID, err)
			return
		}
		quit(remote.TunnelID)
		unlock()

		authMethod, err := newhole.RegisterTunnel(remote)
		if err != nil {
			communication.TunnelStartFailure(remote.TunnelID, err)
			tunnelsMutex.Lock()
			releaseTunnel(remote.TunnelID)
			tunnelsMutex.Unlock()
			return
		}

		tunnelsMutex.Lock()
		if existing, ok := siteToRequestMapping[remote.SiteID]; ok && existing != remote.TunnelID {
			tunnelsMutex.Unlock()
			communication.TunnelStartFailure(remote.TunnelID, fmt.Errorf("a tunnel for hostname %q is already running", remote.SiteID))
			return
		}
		siteToRequestMapping[remote.SiteID] = remote.TunnelID
		tunnelsMutex.Unlock()

		if err := forward(authMethod); err != nil {
			communication.TunnelStartFailure(remote.TunnelID, err)
		}

		forgetTunnel(remote.TunnelID)
	}()
}

func stopTunnel(tunnelID string) {
	forgetTunnel(tunnelID)
	tunnelsMutex.Lock()
	channel, ok := tunnelQuitChannels[tunnelID]
	tunnelsMutex.Unlock()
	if !ok {
		communication.ActiveTunnels()
		return
	}
	select {
	case channel <- true:
	default:
	}
}

func startLogin() {
	loginMutex.Lock()
	if loginCancel != nil {
		select {
		case loginCancel <- true:
		default:
		}
	}
	loginCancel = make(chan bool, 1)
	cancel := loginCancel
	loginMutex.Unlock()

	go func() {
		deviceCodeSpec, err := token.RegisterDevice()
		if err != nil {
			communication.LoginFailure(err)
			return
		}
		communication.LoginStart(*deviceCodeSpec)
		tokens, err := token.PollForToken(deviceCodeSpec.DeviceCode, deviceCodeSpec.Interval, cancel)
		if err != nil {
			communication.LoginFailure(err)
			return
		}
		if err := token.SaveToken(tokens); err != nil {
			communication.LoginFailure(err)
			return
		}
		communication.LoginSuccess(tokens.IDToken)
	}()
}

func startLogout() {
	go func() {
		if err := token.DeleteTokens(); err != nil {
			communication.LogoutFailure(err)
			return
		}
		communication.LogoutSuccess()
	}()
}

//go:embed desktop/build/*
var box embed.FS

func Display() {
	setupDesktopLogging()

	subFS, err := fs.Sub(box, "desktop/build")
	if err != nil {
		communication.Fatal("Unable to read the bundled UI: " + err.Error())
	}

	router := http.NewServeMux()
	router.Handle("/", spaHandler(subFS))
	router.HandleFunc("/ws", websocketHandler)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		communication.Fatal("Unable to start the local UI server: " + err.Error())
	}
	server := &http.Server{Handler: router, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			communication.Error("Local UI server stopped: " + err.Error())
		}
	}()

	address := fmt.Sprintf("http://%s", listener.Addr().String())
	closehandler.SetupCloseHandler(config.Config.FeedbackFormURL)
	restoreActiveTunnels()

	if openNativeWindow(address) {
		return
	}

	if lorca.LocateChrome() == "" {
		openInBrowserFallback(address, "Chrome/Chromium not found")
		return
	}

	window, err := lorca.New(address, "", 980, 800,
		"--remote-allow-origins="+address,
		"--disable-features=Translate",
		"--no-first-run",
	)
	if err != nil {
		openInBrowserFallback(address, err.Error())
		return
	}
	defer window.Close()

	<-window.Done()
	_ = server.Close()
}

func setupDesktopLogging() {
	logPath := cache.GetLocalStorageFile("desktop-"+time.Now().Format("2006-01-02")+".log", "logs")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	communication.SetLogFile(logFile)
	log.Logger = zerolog.New(logFile).With().Timestamp().Logger()
}

func spaHandler(files fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(files))
	index, indexErr := fs.ReadFile(files, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath != "" && cleanPath != "." {
			if info, err := fs.Stat(files, cleanPath); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		if indexErr != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})
}

func openInBrowserFallback(address string, reason string) {
	communication.Warn("Desktop window is unavailable, opening the default browser: " + reason)
	if err := open.Run(address); err != nil {
		zenity.Error("Unable to start a browser window (" + reason + "), and the default browser did not open: " + err.Error())
		return
	}
	zenity.Info("Newhole is running in your browser. Close this window to stop it.",
		zenity.Title("Newhole"),
		zenity.Icon(zenity.InfoIcon))
}
