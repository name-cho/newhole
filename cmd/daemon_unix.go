//go:build !desktop && !windows
// +build !desktop,!windows

package cmd

import (
	"os"
	"os/exec"
	"syscall"
)

func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func terminateProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

func processRunning(process *os.Process) bool {
	return process.Signal(syscall.Signal(0)) == nil
}
