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
	Hide(cmd)
	a := cmd.SysProcAttr
	if a == nil || !a.HideWindow || a.CreationFlags&createNoWindow == 0 {
		t.Fatalf("SysProcAttr = %+v", a)
	}
}

func TestHideKeepsExistingFields(t *testing.T) {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	Hide(cmd)
	a := cmd.SysProcAttr
	if a.CreationFlags&syscall.CREATE_NEW_PROCESS_GROUP == 0 || a.CreationFlags&createNoWindow == 0 || !a.HideWindow {
		t.Fatalf("SysProcAttr = %+v", a)
	}
}
