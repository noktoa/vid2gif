//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"vid2gif/internal/appcore"
	"vid2gif/internal/runner"
	"vid2gif/internal/settings"
)

// version is set by release builds: -ldflags "-X main.version=0.1.0".
var version = "dev"

// App is bound to the page as window.go.main.App. Every exported method is
// callable from JS; the logic lives in internal/appcore (events listed there).
type App struct {
	ctx  context.Context
	core *appcore.Core
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// shutdown stops any run and waits for it to delete its temp folder: longer
// than the tools' 5 s WaitDelay, so a slow-dying tool can't cut it short.
func (a *App) shutdown(context.Context) { a.core.Shutdown(8 * time.Second) }

func (a *App) emit(name string, data any) { runtime.EventsEmit(a.ctx, name, data) }

// header is the first lines of a troubleshooting log.
func header() []string { return []string{"vid2gif " + version + " on " + windowsVersion()} }

// Settings answers at once, so the page can apply the theme before Boot's
// tool checks finish.
func (a *App) Settings() settings.Settings { return a.core.Current() }

func (a *App) Boot() appcore.BootState { return a.core.Boot() }

func (a *App) Install()                       { a.core.Install() }
func (a *App) Add(text string) []appcore.Slip { return a.core.Add(text) }
func (a *App) Make(jobs []runner.Job)         { a.core.Make(jobs) }
func (a *App) Stop()                          { a.core.Stop() }

func (a *App) Validate(start, end string, endIsLength bool, durationSec float64) appcore.Times {
	return a.core.Validate(start, end, endIsLength, durationSec)
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
// Only the output folder, made files and the helper-files folder qualify
// (appcore.OpenablePath); nothing is ever passed to Explorer to run.
func (a *App) OpenFolder(path string) {
	p, isDir, ok := a.core.OpenablePath(path)
	switch {
	case !ok:
		a.core.Log.Printf("open folder: refused %q", path)
	case isDir:
		explorer(`"` + p + `"`)
	default:
		explorer(`/select,"` + p + `"`)
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
	exe := "explorer.exe"
	if root := os.Getenv("SystemRoot"); root != "" {
		exe = filepath.Join(root, "explorer.exe")
	}
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "explorer.exe " + args}
	if err := cmd.Start(); err == nil {
		go cmd.Wait() // explorer exits 1 even on success
	}
}
