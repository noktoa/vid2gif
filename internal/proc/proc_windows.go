//go:build windows

// Package proc keeps tool child processes from flashing console windows.
package proc

import (
	"os/exec"
	"syscall"
)

var getConsoleWindow = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")

// hasConsoleWindow reports whether this process has a visible console window
// (a CLI in a terminal), as opposed to the windowsgui app, which has none.
func hasConsoleWindow() bool {
	hwnd, _, _ := getConsoleWindow.Call()
	return hwnd != 0
}

// Hide makes cmd start without a console window when this process has none
// of its own (the desktop app), so no window flashes. A console CLI leaves cmd
// alone: the tool shares the user's console, so its output and Ctrl+C reach
// it as they would without vid2gif.
func Hide(cmd *exec.Cmd) {
	if !hasConsoleWindow() {
		hide(cmd)
	}
}

// hide sets CREATE_NO_WINDOW, keeping any SysProcAttr fields already set.
func hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
