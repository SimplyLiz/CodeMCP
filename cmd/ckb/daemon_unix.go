//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// setDaemonSysProcAttr sets platform-specific process attributes for daemon mode.
func setDaemonSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
