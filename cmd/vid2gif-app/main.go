//go:build windows

// Command vid2gif-app is the Windows desktop app: a Wails window over the
// same engine as the CLI.
package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"vid2gif"
	"vid2gif/internal/appcore"
	"vid2gif/internal/debuglog"
	"vid2gif/internal/deps"
	"vid2gif/internal/settings"
)

// The exe's icon: rsrc_windows_amd64.syso holds icon.ico as icon group 3
// (Wails' window icon, winc.AppIconID) and 32512 (IDI_APPLICATION, its window
// class icon); Explorer shows the first group, which is 3.
//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --in winres.json --out rsrc --arch amd64

//go:embed all:frontend
var assets embed.FS

func main() {
	root := dataRoot()
	settingsPath := filepath.Join(root, "settings.json")
	log := debuglog.New(filepath.Join(root, "logs"), settings.Load(settingsPath).DebugMode)
	pins, err := deps.Parse(vid2gif.DepsJSON)
	if err != nil { // only a broken build; Boot then reports NeedsSetup
		log.Printf("deps.json: %v", err)
	}
	app := &App{}
	app.core = appcore.New(app.emit, log, settingsPath, binDir(), pins)
	app.core.Header = header
	err = wails.Run(&options.App{
		Title:     "vid2gif",
		Width:     1000,
		Height:    680,
		MinWidth:  480,
		MinHeight: 560,
		// Handler gets every request the embedded files don't answer: the
		// /out/ preview URLs. Mounted directly, never behind a ServeMux.
		AssetServer: &assetserver.Options{Assets: assets, Handler: app.core.Files},
		OnStartup:   app.startup,
		OnShutdown:  app.shutdown,
		Bind:        []any{app},
		Windows:     &windows.Options{WebviewUserDataPath: filepath.Join(root, "webview")},
	})
	if err != nil {
		log.Printf("window: %v", err)
		os.Exit(1)
	}
}
