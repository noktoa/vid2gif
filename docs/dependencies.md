# Pinned dependencies

`deps.json` lists the exact yt-dlp, ffmpeg and deno builds vid2gif works with, for each platform: a download URL, a SHA-256 checksum and a version.

- `vid2gif deps install` downloads them into `bin/`, checks the checksums, and confirms each reports the pinned version. If the right version is already there, it does nothing.
- Every `clip` run looks in the tools folder (`bin/` by default) first, then on your PATH, and compares versions with the pin before doing any work. On a mismatch it stops and says what it expected and what it found.
- `--no-pin` skips the check. It's also how to use vid2gif on an unsupported platform, if you have your own yt-dlp and ffmpeg (and deno, for YouTube).

## Where the tools live

By default in `bin/`, next to `deps.json`. Set `VID2GIF_BIN` to use another folder; both `deps install` and `clip` honour it. A relative path starts from the folder that holds `deps.json`, so `VID2GIF_BIN=bin` is the default.

You can also set it in a `.env` file in that folder (`cp .env.example .env`). A variable in your real environment wins over `.env`. The integration test reads the same variable and file.

`deps.json` is looked up next to the executable, then in the current folder (which is why `go run ./cmd/vid2gif` works from the repo root).

## When a download fails

Websites change, and a pinned yt-dlp will eventually stop working on some of them. When yt-dlp fails, vid2gif prints its error with the pinned version. To move to a newer one:

1. Pick a release on the [yt-dlp releases page](https://github.com/yt-dlp/yt-dlp/releases).
2. In `deps.json`, change the version and each platform's URL and SHA-256 (from the release's `SHA2-256SUMS` file).
3. Run `vid2gif deps install`.

**deno** is bumped the same way: pick a release on the [deno releases page](https://github.com/denoland/deno/releases), and copy its version, URL and the checksum from the `.sha256sum` file next to each zip.

**ffmpeg** uses the dated builds from [yt-dlp/FFmpeg-Builds](https://github.com/yt-dlp/FFmpeg-Builds/releases). Old builds are removed from that page over time, so if `deps install` reports a 404 for ffmpeg, pick a newer one.

### Why deno is required

YouTube extraction needs a JavaScript runtime, so every `clip` run requires the pinned deno and stops with a `deps install` hint if it's missing or the wrong version. yt-dlp only uses the deno vid2gif found (the pinned one, unless you pass `--no-pin`), never one it picks up by itself. See yt-dlp's [EJS page](https://github.com/yt-dlp/yt-dlp/wiki/EJS) for why.

## Licenses

vid2gif is under the GNU GPL version 3 (`LICENSE`). The ffmpeg builds are GPL too, deno is MIT and yt-dlp is the Unlicense. `deps install` downloads them and the MSI includes them; none is in this repository. `THIRD-PARTY-NOTICES.txt` says where their source is, and the MSI installs it next to the app.
