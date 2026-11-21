//go:build !desktop && windows
// +build !desktop,windows

package cmd

import (
	"os"
	"os/exec"
	"syscall"
)

const createNewProcessGroup = 0x00000200

func detachProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}

func terminateProcess(process *os.Process) error {
	return process.Kill()
}

func processRunning(process *os.Process) bool {
	return process.Signal(syscall.Signal(0)) == nil
}
