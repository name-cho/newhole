//go:build !desktop
// +build !desktop

package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/name-cho/newhole/internal/pkg/cache"
	"github.com/name-cho/newhole/internal/pkg/communication"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	daemonFlagName     = "daemon"
	daemonEnvName      = "NEWHOLE_DAEMONIZED"
	daemonLogFileBase  = "daemon"
	daemonProcessFile  = "daemon.pid"
	daemonLogDirName   = "logs"
	daemonStdoutSuffix = ".out.log"
	daemonStderrSuffix = ".err.log"
)

func daemonRequested(cmd *cobra.Command, args []string) bool {
	if os.Getenv(daemonEnvName) == "1" {
		return false
	}
	flag := cmd.Flags().Lookup(daemonFlagName)
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup(daemonFlagName)
	}
	if flag == nil {
		return false
	}
	value, err := strconv.ParseBool(flag.Value.String())
	return err == nil && value
}

func respawnAsDaemon(cmd *cobra.Command, args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("unable to locate the Newhole binary: %w", err)
	}

	logDirectory := cache.GetLocalStorageDir(daemonLogDirName)
	stdoutPath := filepath.Join(logDirectory, daemonLogFileBase+daemonStdoutSuffix)
	stderrPath := filepath.Join(logDirectory, daemonLogFileBase+daemonStderrSuffix)

	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("unable to open %s: %w", stdoutPath, err)
	}
	defer stdoutFile.Close()
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("unable to open %s: %w", stderrPath, err)
	}
	defer stderrFile.Close()

	childArgs := daemonArguments(cmd, args)
	child := exec.Command(executable, childArgs...)
	child.Stdout = stdoutFile
	child.Stderr = stderrFile
	child.Stdin = nil
	child.Env = append(os.Environ(), daemonEnvName+"=1")
	detachProcess(child)
	if err := child.Start(); err != nil {
		return fmt.Errorf("unable to start the background process: %w", err)
	}

	pidFile := cache.GetLocalStorageFile(daemonProcessFile, "")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", child.Process.Pid)), 0o600); err != nil {
		communication.Warn("unable to store the background process id: " + err.Error())
	}

	communication.Info(fmt.Sprintf("Newhole runs in the background with pid %d", child.Process.Pid))
	communication.Info("Output: " + stdoutPath)
	communication.Info("Errors: " + stderrPath)
	communication.Info("Stop it with `newhole stop`")
	return nil
}

func daemonArguments(cmd *cobra.Command, args []string) []string {
	commandPath := []string{}
	for current := cmd; current != nil; current = current.Parent() {
		commandPath = append([]string{current.Name()}, commandPath...)
	}
	commandPath = commandPath[1:]

	arguments := append([]string{}, commandPath...)
	arguments = append(arguments, args...)
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		if flag.Name == daemonFlagName {
			return
		}
		if flag.Value.Type() == "bool" {
			if flag.Value.String() == "true" {
				arguments = append(arguments, "--"+flag.Name)
			}
			return
		}
		arguments = append(arguments, "--"+flag.Name, flag.Value.String())
	})
	cmd.InheritedFlags().Visit(func(flag *pflag.Flag) {
		if flag.Value.Type() == "bool" {
			if flag.Value.String() == "true" {
				arguments = append(arguments, "--"+flag.Name)
			}
			return
		}
		arguments = append(arguments, "--"+flag.Name, flag.Value.String())
	})
	return arguments
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the tunnel running in the background",
	RunE: func(cmd *cobra.Command, args []string) error {
		pidFile := cache.GetLocalStorageFile(daemonProcessFile, "")
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return errors.New("no background tunnel is running")
		}
		reader := bufio.NewReader(strings.NewReader(string(data)))
		line, _ := reader.ReadString('\n')
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || pid <= 0 {
			return errors.New("the stored background process id is not valid")
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			_ = os.Remove(pidFile)
			return errors.New("no background tunnel is running")
		}
		if !processRunning(process) {
			_ = os.Remove(pidFile)
			communication.Info("No background tunnel is running")
			return nil
		}
		if err := terminateProcess(process); err != nil {
			_ = os.Remove(pidFile)
			return fmt.Errorf("unable to stop pid %d: %w", pid, err)
		}
		_ = os.Remove(pidFile)
		communication.Info(fmt.Sprintf("Stopped the background tunnel (pid %d)", pid))
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether a background tunnel is running",
	RunE: func(cmd *cobra.Command, args []string) error {
		pidFile := cache.GetLocalStorageFile(daemonProcessFile, "")
		data, err := os.ReadFile(pidFile)
		if err != nil {
			communication.Info("No background tunnel is running")
			return nil
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || pid <= 0 {
			communication.Warn("The stored background process id is not valid")
			return nil
		}
		process, err := os.FindProcess(pid)
		if err != nil || !processRunning(process) {
			_ = os.Remove(pidFile)
			communication.Info(fmt.Sprintf("The background tunnel with pid %d is not running", pid))
			return nil
		}
		communication.Info(fmt.Sprintf("The background tunnel runs with pid %d", pid))
		communication.Info("Logs: " + cache.GetLocalStorageDir(daemonLogDirName))
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().Bool(daemonFlagName, false, "run the tunnel in the background")
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
}
