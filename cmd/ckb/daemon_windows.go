//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// setDaemonSysProcAttr sets platform-specific process attributes for daemon mode.
func setDaemonSysProcAttr(cmd *exec.Cmd) {
	// CREATE_NEW_PROCESS_GROUP detaches the process from the console
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}
