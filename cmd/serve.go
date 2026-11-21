//go:build !desktop
// +build !desktop

package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/beevik/guid"
	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/internal/app/newhole"
	lm "github.com/name-cho/newhole/internal/app/newhole/models"
	"github.com/name-cho/newhole/internal/pkg/cache"
	"github.com/name-cho/newhole/internal/pkg/closehandler"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

var remoteEndpointSpecs = defaultRemoteSpecs()

const (
	basicAuthUsernameFlagName = "basic-auth-username"
	basicAuthPasswordFlagName = "basic-auth-password"
)

func defaultRemoteSpecs() lm.RemoteEndpointSpecs {
	return lm.RemoteEndpointSpecs{
		ForwardedForHeader: "X-Forwarded-For",
		RealIPHeader:       "X-Real-IP",
		HostKeyPolicy:      "insecure",
		TunnelID:           guid.NewString(),
	}
}

func initServeCommand(serveCmd *cobra.Command) {
	sshDir := cache.GetLocalStorageDir(".ssh")

	flags := serveCmd.PersistentFlags()
	flags.StringVarP(&remoteEndpointSpecs.IdentityFile, "identity-file", "i", fmt.Sprintf("%s/id_rsa", sshDir), "private SSH key used to authenticate against the gateway")
	_ = serveCmd.MarkFlagFilename("identity-file")

	flags.StringVar(&remoteEndpointSpecs.SiteID, "hostname", "", "custom hostname (lowercased automatically)")
	flags.BoolVar(&config.Config.Display.QR, "qr", false, "print a QR code of the public URL")
	flags.StringVarP(&remoteEndpointSpecs.BasicAuthUsername, basicAuthUsernameFlagName, "u", "", "basic authentication username")
	flags.StringVarP(&remoteEndpointSpecs.BasicAuthPassword, basicAuthPasswordFlagName, "p", "", "basic authentication password")
	flags.BoolVar(&remoteEndpointSpecs.DisableOldCiphers, "disable-old-ciphers", false, "disable TLS versions and ciphers older than TLS 1.2")
	flags.BoolVar(&remoteEndpointSpecs.DisableProxyErrorPage, "disable-proxy-error-page", false, "return a plain 502 instead of the built-in error page")
	flags.StringVar(&remoteEndpointSpecs.OverrideHostHeader, "override-host", "", "host header sent to the local service")
	flags.BoolVar(&remoteEndpointSpecs.PreserveHostHeader, "preserve-host", false, "forward the original host header untouched")
	flags.StringVar(&remoteEndpointSpecs.ForwardedForHeader, "forwarded-for-header", "X-Forwarded-For", "header carrying the real client IP")
	flags.StringVar(&remoteEndpointSpecs.RealIPHeader, "real-ip-header", "X-Real-IP", "additional header carrying the real client IP")
	flags.StringSliceVar(&remoteEndpointSpecs.CORSAllowedOrigins, "cors", nil, "allowed CORS origins (use '*' to allow any)")
	flags.IntVar(&remoteEndpointSpecs.MaxHeaderBytes, "max-header-bytes", 0, "maximum size of request headers in bytes (default 1MiB)")
	flags.Int64Var(&remoteEndpointSpecs.MaxRequestBody, "max-body-bytes", 0, "maximum size of a request body in bytes (0 means unlimited)")
	flags.IntVar(&remoteEndpointSpecs.RateLimitRequests, "rate-limit", 0, "maximum requests per client IP per window (0 disables the limiter)")
	flags.IntVar(&remoteEndpointSpecs.RateLimitWindow, "rate-limit-window", 1, "rate limit window in seconds")
	flags.BoolVar(&remoteEndpointSpecs.Compress, "compress", false, "let the local service compress responses, so less data travels through the tunnel")
	flags.BoolVar(&remoteEndpointSpecs.InsecureGateway, "insecure-gateway", false, "skip SSH host key verification")
	flags.StringVar(&remoteEndpointSpecs.HostKeyPolicy, "host-key-policy", "insecure", "SSH host key policy: insecure or known_hosts")
}

func prepareRemote(maxHeaderBytes int) {
	remoteEndpointSpecs.NormalizeHostname()
	if remoteEndpointSpecs.OverrideHostHeader != "" && remoteEndpointSpecs.PreserveHostHeader {
		communication.Warn("--override-host takes precedence over --preserve-host")
	}
	if remoteEndpointSpecs.InsecureGateway {
		config.Config.HostKeyPolicy = "insecure"
	} else if remoteEndpointSpecs.HostKeyPolicy != "" {
		config.Config.HostKeyPolicy = remoteEndpointSpecs.HostKeyPolicy
	}
	if maxHeaderBytes > 0 {
		remoteEndpointSpecs.MaxHeaderBytes = maxHeaderBytes
	}
}

func parsePort(value string) (int32, error) {
	port, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: %w", value, err)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %d: it must be between 1 and 65535", port)
	}
	return int32(port), nil
}

func parseBasicAuthFlags(flagset *pflag.FlagSet) error {
	usernameProvided := false
	passwordProvided := false
	var passwordFlag *pflag.Flag

	flagset.VisitAll(func(flag *pflag.Flag) {
		switch flag.Name {
		case basicAuthUsernameFlagName:
			usernameProvided = flag.Value.String() != ""
		case basicAuthPasswordFlagName:
			passwordFlag = flag
			passwordProvided = flag.Value.String() != ""
		}
	})

	if !usernameProvided && !passwordProvided {
		return nil
	}
	if usernameProvided && !passwordProvided {
		password, err := readPassword()
		if err != nil {
			return err
		}
		if passwordFlag != nil {
			return passwordFlag.Value.Set(password)
		}
		return nil
	}
	if passwordProvided && !usernameProvided {
		return fmt.Errorf("when using basic auth both --%s and --%s have to be provided", basicAuthUsernameFlagName, basicAuthPasswordFlagName)
	}
	return nil
}

func readPassword() (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Print("Enter basic auth password: ")
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return string(password), nil
	}

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("unable to read the password from stdin: %w", err)
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return "", errors.New("the basic auth password read from stdin is empty")
	}
	return password, nil
}

func checkVersion() {
	newhole.CheckVersion()
}

func quitChannel() <-chan bool {
	return closehandler.QuitChannel()
}
