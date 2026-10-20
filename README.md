# vid2gif

Cut a segment out of an online video and save it as a GIF, an animated WebP, an MP4, a MOV, or any mix. Give it a link and a start time (an end time is optional).

[yt-dlp](https://github.com/yt-dlp/yt-dlp) fetches just the part you asked for and [ffmpeg](https://ffmpeg.org) encodes it. YouTube also needs [deno](https://deno.com). vid2gif runs all three at pinned versions, so results don't change when one of them updates.

Platforms: Windows (amd64) and Linux (amd64, arm64). The desktop app is Windows only.

## Desktop app (Windows)

Paste a link, pick the moment, get the clip. It uses the same engine as the command line.

Download `vid2gif-<version>.msi` (or the single-file `vid2gif-app.exe`) from the [releases page](https://github.com/noktoa/vid2gif/releases). Neither needs anything installed first. The MSI bundles yt-dlp, ffmpeg and deno; the plain .exe downloads them on first launch.

Install steps, where files live, uninstalling and logs: [the wiki](https://github.com/noktoa/vid2gif/wiki).

## Command line

You need [Go](https://go.dev/dl/) 1.26.2 or newer to build it. You don't need yt-dlp, ffmpeg or deno installed.

```powershell
go build -o vid2gif.exe ./cmd/vid2gif
./vid2gif deps install   # one-time: downloads the pinned tools into bin/
./vid2gif clip "https://www.youtube.com/watch?v=aqz-KE-bpKQ" --start 2 --end 5
```

That writes `gifs/<video title>_2-5.gif`. Keep `deps.json` next to the executable.

More examples:

```
# From 1:23 to the end of the video
vid2gif clip "https://example.com/watch?v=abc" --start 1:23

# A GIF and an MP4, in a folder of your choice, with your own file name
vid2gif clip "https://example.com/watch?v=abc" --start 1:23 --end 1:30 --format gif,mp4 --out D:\clips --name highlight

# A small, rough WebP
vid2gif clip "https://example.com/watch?v=abc" --start 90 --end 95 --format webp --quality low
```

The URL comes first, then the flags. Times are `SS`, `MM:SS` or `HH:MM:SS`; only the last part can have decimals.

Every flag, the quality presets, and how existing files are handled: [docs/cli.md](docs/cli.md). `vid2gif help` shows the flags in the terminal.

## More docs

- [docs/cli.md](docs/cli.md): flags, quality presets, progress bar
- [Wiki](https://github.com/noktoa/vid2gif/wiki): desktop app install, uninstall, troubleshooting
- [docs/building.md](docs/building.md): building the desktop app and MSI
- [docs/dependencies.md](docs/dependencies.md): pinned tools, where they live, upgrading them, licenses
- [docs/development.md](docs/development.md): tests, CI, architecture

## License

GNU GPL version 3, see `LICENSE`. Third-party tool licenses are in [docs/dependencies.md](docs/dependencies.md#licenses).
