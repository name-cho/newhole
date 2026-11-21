//go:build !desktop
// +build !desktop

package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/name-cho/newhole/config"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/name-cho/newhole/internal/pkg/state"
	"github.com/name-cho/newhole/internal/pkg/token"
	"github.com/spf13/cobra"
)

type interactiveChoice struct {
	mode   string
	target string
}

func newReader() *bufio.Reader {
	return bufio.NewReader(os.Stdin)
}

func prompt(reader *bufio.Reader, question string, fallback string) (string, error) {
	if fallback != "" {
		fmt.Printf("%s [%s]: ", question, fallback)
	} else {
		fmt.Printf("%s: ", question)
	}
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		answer = fallback
	}
	if answer == "" {
		return "", errors.New("an answer is required")
	}
	return answer, nil
}

func chooseMode(reader *bufio.Reader) (string, error) {
	fmt.Println("What would you like to expose?")
	fmt.Println("  1) an HTTP server running locally (default)")
	fmt.Println("  2) a directory over HTTP")
	fmt.Println("  3) a directory over WebDAV")
	answer, err := prompt(reader, "Choice", "1")
	if err != nil {
		return "", err
	}
	switch answer {
	case "1", "http":
		return "http", nil
	case "2", "path", "directory":
		return "path", nil
	case "3", "webdav":
		return "webdav", nil
	}
	return "", fmt.Errorf("unknown choice %q", answer)
}

func chooseTarget(reader *bufio.Reader, mode string) (string, error) {
	switch mode {
	case "http":
		return prompt(reader, "Local port or unix socket", "3000")
	case "path", "webdav":
		directory, err := os.Getwd()
		if err != nil {
			directory = "."
		}
		return prompt(reader, "Local directory", directory)
	}
	return "", fmt.Errorf("unknown mode %q", mode)
}

func chooseHostname(reader *bufio.Reader) (string, error) {
	records := state.Hostnames()
	if len(records) == 0 {
		answer, err := prompt(reader, "Custom hostname (leave empty for a random one)", "random")
		if err != nil {
			return "", err
		}
		if answer == "random" {
			return "", nil
		}
		return answer, nil
	}

	fmt.Println("Previously used hostnames:")
	for index, record := range records {
		if index >= 9 {
			break
		}
		fmt.Printf("  %d) %s (last used %s)\n", index+1, record.Hostname, humanizeTime(record.LastUsed))
	}
	fmt.Println("  0) a new random hostname")
	answer, err := prompt(reader, "Choice", "0")
	if err != nil {
		return "", err
	}
	if answer == "0" || strings.EqualFold(answer, "random") {
		return "", nil
	}
	var index int
	if _, err := fmt.Sscanf(answer, "%d", &index); err == nil && index >= 1 && index <= len(records) {
		return records[index-1].Hostname, nil
	}
	return answer, nil
}

func runInteractive(root *cobra.Command, loggedIn bool) error {
	reader := newReader()

	if config.Config.AuthToken == "" && !token.IsTokenSaved() {
		fmt.Println("You are not logged in yet.")
		if _, err := prompt(reader, "Log in now? (y/n)", "y"); err != nil {
			return err
		}
		deviceCodeSpec, err := token.RegisterDevice()
		if err != nil {
			return err
		}
		communication.LoginStart(*deviceCodeSpec)
		tokens, err := token.PollForToken(deviceCodeSpec.DeviceCode, deviceCodeSpec.Interval, make(chan bool))
		if err != nil {
			return err
		}
		if err := token.SaveToken(tokens); err != nil {
			return err
		}
		communication.LoginSuccess(tokens.IDToken)
	}

	mode, err := chooseMode(reader)
	if err != nil {
		return err
	}
	target, err := chooseTarget(reader, mode)
	if err != nil {
		return err
	}
	hostname, err := chooseHostname(reader)
	if err != nil {
		return err
	}

	remoteEndpointSpecs.SiteID = hostname
	remoteEndpointSpecs.ExposeMode = mode
	if err := state.SaveLastTunnel(state.LastTunnel{Mode: mode, Target: target, Hostname: hostname, UsedAt: time.Now()}); err != nil {
		communication.Debug("unable to remember the interactive choices: " + err.Error())
	}

	return runTunnelCommand(root, mode, target)
}

func commandNameFor(mode string) (string, error) {
	switch mode {
	case "http":
		return "http", nil
	case "path":
		return "path", nil
	case "webdav":
		return "webdav", nil
	}
	return "", fmt.Errorf("unknown mode %q", mode)
}

func runTunnelCommand(root *cobra.Command, mode string, target string) error {
	name, err := commandNameFor(mode)
	if err != nil {
		return err
	}
	root.SetArgs([]string{name, target})
	return root.Execute()
}

func replayLastTunnel(root *cobra.Command) error {
	last := state.LastTunnelSettings()
	if last == nil {
		return errors.New("there is no previous tunnel to replay, start one first")
	}
	remoteEndpointSpecs.SiteID = last.Hostname
	remoteEndpointSpecs.ExposeMode = last.Mode
	return runTunnelCommand(root, last.Mode, last.Target)
}

var lastFlag bool
