# Building the desktop app and MSI

Using the app (install, uninstall, troubleshooting) is documented in the [wiki](https://github.com/noktoa/vid2gif/wiki). This page is for building it.

## The app

Needs only Go:

```powershell
go build -trimpath -tags desktop,production -ldflags "-H windowsgui -s -w" -o dist/vid2gif-app.exe ./cmd/vid2gif-app
```

The exe's icon comes from the committed `cmd/vid2gif-app/rsrc_windows_amd64.syso`. After changing `cmd/vid2gif-app/icon.ico`, run `go generate ./cmd/vid2gif-app`.

The page inside the app is plain JavaScript in `cmd/vid2gif-app/frontend/`. Its tests need Node.js 22 or newer:

```
node --test "cmd/vid2gif-app/frontend/*.test.js"
```

## The MSI

You need the [WiX Toolset](https://wixtoolset.org) 6 with its UI and Util extensions, and the pinned tools in `bin\` (the Go command downloads them, about 270 MB, skipping any already there):

```powershell
dotnet tool install --global wix --version 6.0.2
wix extension add --global WixToolset.UI.wixext/6.0.2 WixToolset.Util.wixext/6.0.2
go run ./build/installtools deps.json bin
wix build installer\vid2gif.wxs -ext WixToolset.UI.wixext -ext WixToolset.Util.wixext -arch x64 -d Version=0.1.0 -d AppExe=dist\vid2gif-app.exe -d AppIcon=cmd\vid2gif-app\icon.ico -d ToolsDir=bin -o dist\vid2gif-0.1.0.msi
```

`installer\test.ps1` builds two versions, installs (just for you, into a temp folder), upgrades, uninstalls, and checks what's left. It refuses to run if vid2gif is already installed, moves your `%LOCALAPPDATA%\vid2gif` aside while it runs and restores it afterwards, and fetches the tools into `bin\` the first time.

## Keep the wiki in step

The wiki pages (Install, Uninstall, Troubleshooting) describe installer behaviour. If you change what the MSI installs or removes, update them too; the wiki is a separate repo, `noktoa/vid2gif.wiki`.
