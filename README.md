# vid2gif

vid2gif cuts a segment out of an online video and saves it as a GIF, an animated WebP, an MP4, a MOV, or any mix of those. Give it a link and a start time (an end time is optional), and pick where the files go.

[yt-dlp](https://github.com/yt-dlp/yt-dlp) fetches just the part of the video you asked for, and [ffmpeg](https://ffmpeg.org) turns it into the output format. YouTube also needs a JavaScript runtime, which is [deno](https://deno.com). vid2gif runs all three and keeps them at pinned versions, so your results don't change when one of them updates.

Supported platforms: Windows (amd64) and Linux (amd64, arm64).

## Quick start

You need [Go](https://go.dev/dl/) 1.26.2 or newer to build it. You don't need yt-dlp, ffmpeg or deno installed; vid2gif fetches its own copies.

```powershell
go build -o vid2gif.exe ./cmd/vid2gif   # produces vid2gif.exe on Windows
./vid2gif deps install            # one-time: downloads the pinned yt-dlp, ffmpeg and deno into bin/ (or $VID2GIF_BIN)
./vid2gif clip "https://www.youtube.com/watch?v=aqz-KE-bpKQ" --start 2 --end 5
```

That writes `gifs/<video title>_2-5.gif`. Keep `deps.json` in the same folder as the executable, because that's where vid2gif looks for it (if it isn't there, the current folder is used, which is what makes `go run ./cmd/vid2gif` work from the repo root).

## Examples

```
# A GIF from 1:23 to the end of the video (no --end means until the end)
vid2gif clip "https://example.com/watch?v=abc" --start 1:23

# A GIF and an MP4 of the same moment, in a folder of your choice, with your own file name
vid2gif clip "https://example.com/watch?v=abc" --start 1:23 --end 1:30 --format gif,mp4 --out D:\clips --name highlight

# A small, rough WebP
vid2gif clip "https://example.com/watch?v=abc" --start 90 --end 95 --format webp --quality low

# Fractional seconds work on the last part of a time
vid2gif clip "https://example.com/watch?v=abc" --start 1:23.5 --end 1:25
```

The URL always comes first, then the flags.

## Options

Run `vid2gif help` to see all of this in the terminal.

| Flag | Default | What it does |
|---|---|---|
| `--start T` | required | Where the clip begins. |
| `--end T` | the end of the video | Where the clip ends. Must be after the start. |
| `--format LIST` | `gif` | Any of `gif`, `webp`, `mp4`, `mov`, separated by commas. |
| `--out DIR` | `./gifs` | Where the files are written. The folder is created if it doesn't exist. |
| `--name BASE` | `<video title>_<start>-<end>` | File name without the extension. Each format adds its own. With no `--end` the end part is the word `end`, as in `title_83-end`. |
| `--quality Q` | `medium` | `low`, `medium` or `high`. See below. |
| `--overwrite` | off | Replace files that already exist. |
| `--no-pin` | off | Skip the version check. The tools folder is still searched first, then your PATH. deno is optional here. |

Times are written as `SS`, `MM:SS` or `HH:MM:SS`. Only the last part can have decimals.

Without `--end` the clip runs to the end of the video. On a long video, give an end time unless you really want everything after the start, because a GIF of that much footage gets very large.

### Existing files

If an output file already exists, vid2gif skips it, tells you, and carries on with the other formats. If every file you asked for already exists, it doesn't download any video (it still asks yt-dlp for the title unless you gave `--name`). Use `--overwrite` when you do want to replace them.

### Quality

| Preset | GIF and WebP frame rate | GIF and WebP max width | MP4 and MOV compression (CRF) |
|---|---|---|---|
| `low` | 10 fps | 320 px | 28 |
| `medium` | 15 fps | 480 px | 23 |
| `high` | 24 fps | 720 px | 18 |

GIFs and WebPs are resampled to the preset's frame rate (a source with a lower rate gets frames repeated), but the clip still plays at the same speed. Frames are dropped, not stretched. GIFs and WebPs are never scaled up, so a narrow source stays narrow. MP4 and MOV keep the original frame rate and size (rounded down to even numbers of pixels), and also keep the audio. GIF and WebP have no sound.

## Pinned dependencies

`deps.json` lists the exact yt-dlp, ffmpeg and deno builds vid2gif works with, for each platform: a download URL, a SHA-256 checksum and a version.

- `vid2gif deps install` downloads them into `bin/`, checks the checksums, and confirms each one reports the pinned version. If the right version is already there, it does nothing.
- Every `clip` run looks in the tools folder (`bin/` by default) first, then on your PATH, and compares the version with the pin before doing any work. If it doesn't match, vid2gif stops and tells you what it expected and what it found.
- `--no-pin` skips that check. It's also the way to use vid2gif on a platform that isn't supported yet, as long as you have your own yt-dlp and ffmpeg (and deno, for YouTube).

### Where the tools live

By default the tools go in `bin/`, next to `deps.json`. Set the `VID2GIF_BIN` environment variable to use another folder; both `deps install` and `clip` honour it. A relative path starts from the folder that holds `deps.json`, so `VID2GIF_BIN=bin` is the same as the default.

You can also put it in a `.env` file in that folder. `.env.example` shows the format, so `cp .env.example .env` is all it takes. A variable set in your real environment takes priority over `.env`. The integration test reads the same variable and the same file.

### When a download fails

Websites change, and a pinned yt-dlp will stop working on some of them sooner or later. When yt-dlp fails, vid2gif prints its error along with the pinned version. To move to a newer one:

1. Find the release you want on the [yt-dlp releases page](https://github.com/yt-dlp/yt-dlp/releases).
2. In `deps.json`, change the version, and the URL and SHA-256 for each platform. The checksums are listed in the release's `SHA2-256SUMS` file.
3. Run `vid2gif deps install`.

YouTube extraction needs a JavaScript runtime, so every `clip` run requires the pinned deno and stops with a `deps install` hint if it's missing or the wrong version. yt-dlp only uses the deno that vid2gif found (the pinned one, unless you pass `--no-pin`), never one it picks up by itself. See yt-dlp's [EJS page](https://github.com/yt-dlp/yt-dlp/wiki/EJS) for why. deno is bumped like the others: pick a release on the [deno releases page](https://github.com/denoland/deno/releases), copy its version, URL and the checksum from the `.sha256sum` file next to each zip into `deps.json`.

ffmpeg is bumped the same way, using the dated builds from [yt-dlp/FFmpeg-Builds](https://github.com/yt-dlp/FFmpeg-Builds/releases). Older dated builds are removed from that page over time, so if `deps install` reports a 404 for ffmpeg, pick a newer build.

The ffmpeg builds are GPL-licensed. vid2gif downloads them, and deno (MIT), when you run `deps install`; neither is included in this repository.

## Running the tests

```
go test ./...
```

runs the unit tests. They don't need yt-dlp, ffmpeg, deno or a network connection.

```
go test -tags integration -v -timeout 30m ./test/integration/
```

installs the pinned tools into a temporary folder and encodes real test videos to every format. The first run downloads about 250 MB.

To run the same test on Linux from a Windows machine, with Docker:

```
docker build --platform linux/amd64 -f build/Dockerfile.test -t vid2gif-test .
docker run --rm vid2gif-test
```

Use `--platform linux/arm64` for arm64. It runs under emulation and is slow.

The integration test installs the tools into a temporary folder unless `VID2GIF_BIN` is set (see [Where the tools live](#where-the-tools-live)); set it to reuse an existing folder such as `bin/` so nothing is downloaded again.

On GitHub, `.github/workflows/ci.yml` checks gofmt, runs `go vet` and the unit tests on every push and pull request. `integration.yml` runs the integration test on Windows, Linux amd64 and Linux arm64 every week and whenever `deps.json` or the code that uses it changes. The pinned tools are about 250 MB, so they're cached: the Linux jobs reuse Docker layers (`build/Dockerfile.test` puts the tools in their own layer, which only rebuilds when `deps.json`, `go.mod`, `go.sum` or the installer changes), and the Windows job caches the tools folder. The weekly run skips those caches so it still catches a pinned download that has been removed upstream. Neither workflow contacts YouTube; the integration test only downloads the pinned tools from GitHub releases.

## Architecture

How the code is split up, and what talks to what:

```mermaid
flowchart LR
    cli["cmd/vid2gif<br/>flags, exit codes"]
    naming["internal/naming<br/>times, formats, file names"]
    deps["internal/deps<br/>deps.json, version checks, installer"]
    clip["internal/clip<br/>download and encode pipeline"]
    encode["internal/encode<br/>quality presets, ffmpeg arguments"]
    bin[("bin/<br/>yt-dlp, ffmpeg, deno")]
    github[("GitHub releases")]
    site[("video site")]

    cli --> naming
    cli --> deps
    cli --> clip
    clip --> naming
    clip --> encode
    deps --> bin
    github -.->|"deps install"| deps
    clip -->|"runs"| bin
    bin -.->|"yt-dlp fetches the section"| site
```

What happens during one `clip` run:

```mermaid
sequenceDiagram
    actor You
    participant CLI as cmd/vid2gif
    participant Deps as internal/deps
    participant Clip as internal/clip
    participant YT as yt-dlp with deno
    participant FF as ffmpeg

    You->>CLI: clip URL --start 1:23
    CLI->>CLI: validate times and flags
    CLI->>Deps: find yt-dlp, ffmpeg and deno, check pins
    CLI->>Clip: run
    Clip->>YT: get the title (unless --name is set)
    Clip->>Clip: choose file names, skip existing files
    Clip->>YT: download only the requested section
    YT-->>Clip: the clip file in a temporary folder
    loop each format
        Clip->>FF: encode
        FF-->>Clip: output file in the temporary folder
        Clip->>Clip: move it into --out
    end
    Clip->>Clip: remove the temporary folder
```

The temporary folder is created inside `--out`, so the final move is a rename. `deps install` is separate: it downloads the three tools from GitHub releases, checks their SHA-256 hashes, and puts them in `bin/`.

## How it works

1. It checks your times and flags first, so a typo doesn't cost you a download.
2. It finds yt-dlp, ffmpeg and deno and checks their versions against the pins.
3. It asks yt-dlp for the video title (unless you gave `--name`), and works out which files it needs to write.
4. It downloads only the section you asked for, once, however many formats you want.
5. It encodes each format with ffmpeg and moves the result into your output folder.
6. It cleans up its temporary files, including when you press Ctrl+C. They live in a `.vid2gif-tmp-*` folder inside your output folder, so if the process is killed hard you can delete that folder.
