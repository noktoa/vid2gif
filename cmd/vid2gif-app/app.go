//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"vid2gif/internal/appcore"
	"vid2gif/internal/runner"
	"vid2gif/internal/settings"
)

// version is set by release builds: -ldflags "-X main.version=v0.1.0".
var version = "dev"

// App is bound to the page as window.go.main.App. Every exported method is
// callable from JS; the logic lives in internal/appcore (events listed there).
type App struct {
	ctx  context.Context
	core *appcore.Core
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// shutdown stops any run and gives it a moment to delete its temp folder.
func (a *App) shutdown(context.Context) { a.core.Shutdown(5 * time.Second) }

func (a *App) emit(name string, data any) { runtime.EventsEmit(a.ctx, name, data) }

func (a *App) Boot() appcore.BootState {
	a.core.Log.Printf("vid2gif %s on %s", version, windowsVersion())
	return a.core.Boot()
}

func (a *App) Install()                       { a.core.Install() }
func (a *App) Add(text string) []appcore.Slip { return a.core.Add(text) }
func (a *App) Make(jobs []runner.Job)         { a.core.Make(jobs) }
func (a *App) Stop()                          { a.core.Stop() }

func (a *App) Validate(start, end string, durationSec float64) appcore.Times {
	return a.core.Validate(start, end, durationSec)
}

func (a *App) SaveSettings(s settings.Settings) error { return a.core.SaveSettings(s) }

// PickFolder returns the chosen folder, or "" when cancelled.
func (a *App) PickFolder() string {
	opts := runtime.OpenDialogOptions{Title: "Save clips to"}
	if d := a.core.Current().OutDir; isDir(d) { // Wails refuses a missing default
		opts.DefaultDirectory = d
	}
	dir, err := runtime.OpenDirectoryDialog(a.ctx, opts)
	if err != nil {
		a.core.Log.Printf("folder dialog: %v", err)
	}
	return dir
}

// OpenFolder shows a folder, or a file selected in its folder, in Explorer.
// Nothing is ever passed to Explorer as a program to run.
func (a *App) OpenFolder(path string) {
	fi, err := os.Stat(path)
	switch {
	case err != nil:
		a.core.Log.Printf("open folder: %v", err)
	case fi.IsDir():
		explorer(`"` + path + `"`)
	default:
		explorer(`/select,"` + path + `"`)
	}
}

// OpenLogs opens the debug log folder (also the slip's "Show log").
func (a *App) OpenLogs() {
	d := a.core.Log.Dir()
	_ = os.MkdirAll(d, 0o755) // logs appear only after the first line
	explorer(`"` + d + `"`)
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// explorer passes args verbatim: Go's quoting breaks /select, with spaces.
// Windows paths cannot contain '"'.
func explorer(args string) {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "explorer.exe " + args}
	if err := cmd.Start(); err == nil {
		go cmd.Wait() // explorer exits 1 even on success
	}
}
