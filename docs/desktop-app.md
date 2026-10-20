# Desktop app (Windows)

A window over the same engine as the command line, so files come out the same. Windows only; the Linux build is the command line.

## Get it

The [releases page](https://github.com/noktoa/vid2gif/releases) has:

- `vid2gif-<version>.msi`, the installer.
- `vid2gif-app.exe`, a single file you can run from any folder.

On current Windows 10 and 11 neither needs anything installed first (the window uses WebView2, which ships with Windows).

## Helper tools

yt-dlp, ffmpeg and deno (about 270 MB) come inside the MSI, so installing needs no network. The plain .exe downloads them on first launch, once, with a progress screen.

The app looks in its `bin` folder first, then on your PATH, so tools you already have work too. A copy in `bin` has to be the pinned version; one found only on PATH is used whatever its version. If it isn't the pinned version, the troubleshooting log says so, and an untested version may fail.

## Where things live

- Settings, logs and web view data: `%LOCALAPPDATA%\vid2gif`
- Helper tools: a `bin` folder next to the app (set `VID2GIF_BIN` to move it, as with the command line)

## Troubleshooting

Turn on Settings → Troubleshooting. The app then writes one log file per run to `%LOCALAPPDATA%\vid2gif\logs` and keeps the newest 5. The log includes the video links you used, so look through it before sending it with a bug report.

## Installing with the MSI

1. Run `vid2gif-<version>.msi`. It isn't code-signed, so SmartScreen may say it "protected your PC". Click **More info**, then **Run anyway**.
2. Choose **Install just for you** (default, no admin rights) or **Install for all users of this machine** (asks for an administrator).
3. Keep the suggested folder or pick your own. Just for you: `%LOCALAPPDATA%\Programs\vid2gif`. All users: `C:\Program Files\vid2gif`.
4. To use your own yt-dlp, ffmpeg or deno, open **Customize** and untick it. Each description warns it may not work if the version differs from the tested one.

The installer adds a Start Menu shortcut and an entry in Settings → Apps → Installed apps, and puts the ticked tools in `<install folder>\bin`. An unticked tool is found on your PATH instead.

**Silent install or upgrade:** `msiexec /i ... /qn` ticks everything. To pick tools, add `ADDLOCAL=Main,ToolYtDlp,ToolFfmpeg,ToolDeno` with only the ones you want.

**Upgrading:** installing a newer MSI upgrades in place, replaces the tools with its own, and keeps your settings.

**All-users installs** have one shared copy of the tools that only administrators can change. If a standard user is told the helper files couldn't be set up (a tool in neither `bin` nor PATH), an administrator can run **Repair** on vid2gif in Installed apps.

## Uninstalling

From Installed apps. This removes the program, the shortcut, the tools in `bin` (including ones the app downloaded, and any half-finished download), and `%LOCALAPPDATA%\vid2gif`.

- Only the app's own files go. `bin` and the install folder are removed only when nothing else is left in them.
- In `bin`, only `yt-dlp.exe`, `ffmpeg.exe` and `deno.exe` are ever touched.
- Your clips and output folder are never removed. After a crash, a `.vid2gif-tmp-*` folder may be left in the output folder; it's safe to delete.
- For an all-users install, uninstalling clears `%LOCALAPPDATA%\vid2gif` only for the user who runs it. Other users can delete their own copy.

## Building

The app alone needs only Go:

```powershell
go build -trimpath -tags desktop,production -ldflags "-H windowsgui -s -w" -o dist/vid2gif-app.exe ./cmd/vid2gif-app
```

The exe's icon comes from the committed `cmd/vid2gif-app/rsrc_windows_amd64.syso`. After changing `cmd/vid2gif-app/icon.ico`, run `go generate ./cmd/vid2gif-app`.

The page inside the app is plain JavaScript in `cmd/vid2gif-app/frontend/`. Its tests need Node.js 22 or newer:

```
node --test "cmd/vid2gif-app/frontend/*.test.js"
```

### Building the MSI

You need the [WiX Toolset](https://wixtoolset.org) 6 with its UI and Util extensions, and the pinned tools in `bin\` (the Go command downloads them, about 270 MB, skipping any already there):

```powershell
dotnet tool install --global wix --version 6.0.2
wix extension add --global WixToolset.UI.wixext/6.0.2 WixToolset.Util.wixext/6.0.2
go run ./build/installtools deps.json bin
wix build installer\vid2gif.wxs -ext WixToolset.UI.wixext -ext WixToolset.Util.wixext -arch x64 -d Version=0.1.0 -d AppExe=dist\vid2gif-app.exe -d AppIcon=cmd\vid2gif-app\icon.ico -d ToolsDir=bin -o dist\vid2gif-0.1.0.msi
```

`installer\test.ps1` builds two versions, installs (just for you, into a temp folder), upgrades, uninstalls, and checks what's left. It refuses to run if vid2gif is already installed, moves your `%LOCALAPPDATA%\vid2gif` aside while it runs and restores it afterwards, and fetches the tools into `bin\` the first time.
