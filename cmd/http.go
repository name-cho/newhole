//go:build !desktop
// +build !desktop

package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/internal/app/newhole"
	lm "github.com/name-cho/newhole/internal/app/newhole/models"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/token"
	"github.com/spf13/cobra"
)

var localEndpointSpecs lm.LocalHTTPEndpointSpecs

var httpCmd = &cobra.Command{
	Use:   "http <port|host:port|socket> [host]",
	Short: "Expose an HTTP server running locally",
	Long: `Exposes an HTTP server running locally, or on a machine reachable from this host.

  newhole http 3000
  newhole http 3000 192.168.1.20
  newhole http unix:/var/run/app.sock`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return errors.New("missing argument: port")
		}

		prepareRemote(0)
		remoteEndpointSpecs.ExposeMode = "http"
		if err := resolveLocalTarget(args); err != nil {
			return err
		}
		if err := lm.Validate(&localEndpointSpecs); err != nil {
			return err
		}
		communication.ApplicationStart(token.IsTokenSaved(), token.GetIdToken())

		checkVersion()

		exposeConfig := lm.ExposeHTTPConfig{
			Local:  localEndpointSpecs,
			Remote: remoteEndpointSpecs,
		}

		authMethod, err := newhole.RegisterTunnel(&exposeConfig.Remote)
		if err != nil {
			return err
		}

		return newhole.ForwardPort(exposeConfig, authMethod, quitChannel())
	},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if daemonRequested(cmd, args) {
			return respawnAsDaemon(cmd, args)
		}
		return nil
	},
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("missing argument: port")
		}
		return nil
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return parseBasicAuthFlags(cmd.Flags())
	},
}

func resolveLocalTarget(args []string) error {
	target := args[0]
	switch {
	case strings.HasPrefix(target, "unix:"):
		localEndpointSpecs.UnixSocket = strings.TrimPrefix(target, "unix:")
		return nil
	case strings.Contains(target, "/") && !strings.Contains(target, ":"):
		localEndpointSpecs.UnixSocket = target
		return nil
	case strings.Contains(target, "://"):
		return fmt.Errorf("use `newhole http <port>`; %q looks like a URL", target)
	case strings.Count(target, ":") == 1 && !strings.HasPrefix(target, "["):
		parts := strings.SplitN(target, ":", 2)
		port, err := parsePort(parts[1])
		if err != nil {
			return err
		}
		localEndpointSpecs.Host = parts[0]
		localEndpointSpecs.Port = port
	default:
		port, err := parsePort(target)
		if err != nil {
			return err
		}
		localEndpointSpecs.Port = port
		localEndpointSpecs.Host = "127.0.0.1"
	}

	if len(args) > 1 {
		if localEndpointSpecs.UnixSocket != "" {
			return errors.New("a unix socket target does not accept a host argument")
		}
		localEndpointSpecs.Host = args[1]
	}
	if localEndpointSpecs.Host == "" {
		localEndpointSpecs.Host = "127.0.0.1"
	}
	return nil
}

func init() {
	initServeCommand(httpCmd)
	httpCmd.Flags().BoolVar(&localEndpointSpecs.HTTPS, "https", false, "the local server speaks HTTPS")
	httpCmd.Flags().StringVar(&localEndpointSpecs.Path, "path", "", "path to expose on the local server")
	httpCmd.Flags().StringVar(&config.Config.TLS.ServerName, "tls-server-name", config.Config.TLS.ServerName, "server name to verify when the upstream uses TLS")
	rootCmd.AddCommand(httpCmd)
}
