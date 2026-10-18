# vid2gif

vid2gif cuts a segment out of an online video and saves it as a GIF, an animated WebP, an MP4, a MOV, or any mix of those. Give it a link and a start time (an end time is optional), and pick where the files go.

[yt-dlp](https://github.com/yt-dlp/yt-dlp) fetches just the part of the video you asked for, and [ffmpeg](https://ffmpeg.org) turns it into the output format. vid2gif runs both and keeps them at pinned versions, so your results don't change when one of them updates.

Supported platforms: Windows (amd64) and Linux (amd64, arm64).

## Quick start

You need [Go](https://go.dev/dl/) to build it. You don't need yt-dlp or ffmpeg installed; vid2gif fetches its own copies.

```
go build -o vid2gif .        # produces vid2gif.exe on Windows
./vid2gif deps install       # one-time: downloads the pinned yt-dlp and ffmpeg into ./bin
./vid2gif clip "https://www.youtube.com/watch?v=jNQXAC9IJRQ" --start 2 --end 5
```

That writes `gifs/<video title>_2-5.gif`. Keep `deps.json` in the same folder as the executable, because that's where vid2gif looks for it.

## Examples

```
# A 5-second GIF starting at 1:23 (no --end means start + 5 seconds)
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
| `--end T` | start + 5 seconds | Where the clip ends. Must be after the start. |
| `--format LIST` | `gif` | Any of `gif`, `webp`, `mp4`, `mov`, separated by commas. |
| `--out DIR` | `./gifs` | Where the files are written. The folder is created if it doesn't exist. |
| `--name BASE` | `<video title>_<start>-<end>` | File name without the extension. Each format adds its own. |
| `--quality Q` | `medium` | `low`, `medium` or `high`. See below. |
| `--overwrite` | off | Replace files that already exist. |
| `--no-pin` | off | Use whatever yt-dlp and ffmpeg are on your PATH and skip the version check. |

Times are written as `SS`, `MM:SS` or `HH:MM:SS`. Only the last part can have decimals.

### Existing files

If an output file already exists, vid2gif skips it, tells you, and carries on with the other formats. If every file you asked for already exists, it doesn't download anything. Use `--overwrite` when you do want to replace them.

### Quality

| Preset | GIF and WebP frame rate | GIF and WebP max width | MP4 and MOV compression (CRF) |
|---|---|---|---|
| `low` | 10 fps | 320 px | 28 |
| `medium` | 15 fps | 480 px | 23 |
| `high` | 24 fps | 720 px | 18 |

GIFs and WebPs are never scaled up, so a narrow source stays narrow. MP4 and MOV keep the original size and frame rate, and also keep the audio. GIF and WebP have no sound.

## Pinned dependencies

`deps.json` lists the exact yt-dlp and ffmpeg builds vid2gif works with, for each platform: a download URL, a SHA-256 checksum and a version.

- `vid2gif deps install` downloads them into `bin/`, checks the checksums, and confirms each one reports the pinned version. If the right version is already there, it does nothing.
- Every `clip` run looks in `bin/` first, then on your PATH, and compares the version with the pin before doing any work. If it doesn't match, vid2gif stops and tells you what it expected and what it found.
- `--no-pin` skips that check. It's also the way to use vid2gif on a platform that isn't supported yet, as long as you have your own yt-dlp and ffmpeg.

### When a download fails

Websites change, and a pinned yt-dlp will stop working on some of them sooner or later. When yt-dlp fails, vid2gif prints its error along with the pinned version. To move to a newer one:

1. Find the release you want on the [yt-dlp releases page](https://github.com/yt-dlp/yt-dlp/releases).
2. In `deps.json`, change the version, and the URL and SHA-256 for each platform. The checksums are listed in the release's `SHA2-256SUMS` file.
3. Run `vid2gif deps install`.

YouTube links can also fail if yt-dlp can't find a JavaScript runtime. It prints a warning when that's the case; installing [deno](https://deno.com) is the usual fix.

ffmpeg is bumped the same way, using the dated builds from [yt-dlp/FFmpeg-Builds](https://github.com/yt-dlp/FFmpeg-Builds/releases). Older dated builds are removed from that page over time, so if `deps install` reports a 404 for ffmpeg, pick a newer build.

The ffmpeg builds are GPL-licensed. vid2gif downloads them when you run `deps install`; they aren't included in this repository.

## Running the tests

```
go test ./...
```

runs the unit tests. They don't need yt-dlp, ffmpeg or a network connection.

```
go test -tags integration -run TestRealToolsEncode -v -timeout 30m ./...
```

installs the pinned tools into a temporary folder and encodes real test videos to every format. The first run downloads about 200 MB.

To run the same test on Linux from a Windows machine, with Docker:

```
docker build --platform linux/amd64 -f Dockerfile.test -t vid2gif-test .
docker run --rm vid2gif-test
```

Use `--platform linux/arm64` for arm64. It runs under emulation and is slow.

## How it works

1. It checks your times and flags first, so a typo doesn't cost you a download.
2. It finds yt-dlp and ffmpeg and checks their versions against the pins.
3. It asks yt-dlp for the video title (unless you gave `--name`), and works out which files it needs to write.
4. It downloads only the section you asked for, once, however many formats you want.
5. It encodes each format with ffmpeg and moves the result into your output folder.
6. It cleans up its temporary files, including when you press Ctrl+C.
