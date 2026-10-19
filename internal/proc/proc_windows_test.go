//go:build windows

package proc

import (
	"os/exec"
	"syscall"
	"testing"
)

const createNoWindow = 0x08000000

func TestHideCreatesAttr(t *testing.T) {
	cmd := exec.Command("cmd")
	hide(cmd)
	a := cmd.SysProcAttr
	if a == nil || !a.HideWindow || a.CreationFlags&createNoWindow == 0 {
		t.Fatalf("SysProcAttr = %+v", a)
	}
}

func TestHideKeepsExistingFields(t *testing.T) {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	hide(cmd)
	a := cmd.SysProcAttr
	if a.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP == 0 || a.CreationFlags&createNoWindow == 0 || !a.HideWindow {
		t.Fatalf("SysProcAttr = %+v", a)
	}
}

// Hide hides only when this process has no console window (the windowsgui
// app, or an agent shell); in a developer's terminal it leaves cmd alone so
// the tool shares the console. Valid in both situations.
func TestHideOnlyWithoutConsole(t *testing.T) {
	cmd := exec.Command("cmd")
	Hide(cmd)
	hidden := cmd.SysProcAttr != nil && cmd.SysProcAttr.CreationFlags&createNoWindow != 0
	if want := !hasConsoleWindow(); hidden != want {
		t.Errorf("hidden = %v with console window = %v; want %v", hidden, !want, want)
	}
	if !hidden && cmd.SysProcAttr != nil {
		t.Errorf("console present but SysProcAttr changed: %+v", cmd.SysProcAttr)
	}
}
