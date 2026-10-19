//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
	"vid2gif/internal/deps"
)

// dataRoot is %LOCALAPPDATA%\vid2gif, the one per-user folder holding
// settings.json, logs and webview (so uninstall removes one folder).
func dataRoot() string {
	d, err := os.UserCacheDir() // %LOCALAPPDATA% on Windows
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "vid2gif")
}

// exeDir is the install folder: the tools live in its bin folder.
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Dir(exe)
}

func binDir() string { return deps.BinDir(exeDir(), os.Getenv) }

func windowsVersion() string {
	v := windows.RtlGetVersion()
	return fmt.Sprintf("Windows %d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
}
