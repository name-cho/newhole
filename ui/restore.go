//go:build desktop
// +build desktop

package ui

import (
	"encoding/json"
	"fmt"
	"runtime/debug"

	"golang.org/x/crypto/ssh"

	"github.com/name-cho/newhole/internal/app/newhole"
	lm "github.com/name-cho/newhole/internal/app/newhole/models"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/state"
)

func restoreActiveTunnels() {
	saved := state.ActiveTunnels()
	if len(saved) == 0 {
		return
	}
	communication.Debug(fmt.Sprintf("restoring %d saved tunnel(s)", len(saved)))
	for _, tunnel := range saved {
		if tunnel.Mode == "http" {
			var config lm.ExposeHTTPConfig
			if err := json.Unmarshal(tunnel.Payload, &config); err == nil {
				communication.Debug(fmt.Sprintf("restore payload port=%d host=%q https=%v siteId=%q", config.Local.Port, config.Local.Host, config.Local.HTTPS, config.Remote.SiteID))
			}
		}
		restoreOne(tunnel)
	}
}

func restoreOne(tunnel state.ActiveTunnel) {
	defer func() {
		if recovered := recover(); recovered != nil {
			communication.Warn(fmt.Sprintf("failed to restore tunnel %s: %v\n%s", tunnel.TunnelID, recovered, debug.Stack()))
		}
	}()

	communication.TunnelInfo(tunnel.TunnelID, "restoring saved tunnel "+tunnel.Mode)
	switch tunnel.Mode {
	case "http":
		var config lm.ExposeHTTPConfig
		if err := json.Unmarshal(tunnel.Payload, &config); err != nil {
			communication.Warn("unable to restore the HTTP tunnel: " + err.Error())
			return
		}
		startTunnel(&config.Remote, func(authMethod ssh.AuthMethod) error {
			return newhole.ForwardPort(config, authMethod, quit(config.Remote.TunnelID))
		})
	case "path":
		var config lm.ExposeDirectoryConfig
		if err := json.Unmarshal(tunnel.Payload, &config); err != nil {
			communication.Warn("unable to restore the directory tunnel: " + err.Error())
			return
		}
		startTunnel(&config.Remote, func(authMethod ssh.AuthMethod) error {
			return newhole.ForwardDirectory(config, authMethod, quit(config.Remote.TunnelID))
		})
	case "webdav":
		var config lm.ExposeWebdavConfig
		if err := json.Unmarshal(tunnel.Payload, &config); err != nil {
			communication.Warn("unable to restore the WebDAV tunnel: " + err.Error())
			return
		}
		startTunnel(&config.Remote, func(authMethod ssh.AuthMethod) error {
			return newhole.ForwardDirectoryViaWebdav(config, authMethod, quit(config.Remote.TunnelID))
		})
	default:
		communication.Warn("unknown saved tunnel mode: " + tunnel.Mode)
	}
}
