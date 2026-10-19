//go:build windows

// Package proc keeps tool child processes from flashing console windows.
package proc

import (
	"os/exec"
	"syscall"
)

// Hide makes cmd start without a console window (CREATE_NO_WINDOW), keeping
// any SysProcAttr fields already set.
func Hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
