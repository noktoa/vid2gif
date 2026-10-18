# vid2gif — design

Date: 2026-10-17
Status: draft for review

## Purpose

A command-line tool: give it a video URL and a start time (and optionally an end time) and it writes that segment as a GIF, an animated WebP, an MP4, a MOV, or any combination. The user chooses the output directory, file name and quality. Downloading is done by **yt-dlp**, encoding by **ffmpeg**; this tool only orchestrates them.

The author uses it personally and intends to share it with others, so it ships as a single Go binary that can fetch its own pinned copies of yt-dlp, ffmpeg and deno. The tool is mainly for clipping YouTube uploads, and YouTube extraction needs a JavaScript runtime, so deno (yt-dlp's preferred runtime) is a hard requirement.

### Success criteria

- `vid2gif clip <url> --start 1:23 --end 1:30 --format gif,mp4 --out D:\gifs` produces `D:\gifs\<name>.gif` and `<name>.mp4` of that exact segment.
- On a machine with none of yt-dlp, ffmpeg or deno installed, `vid2gif deps install` followed by `vid2gif clip ...` works with no other setup.
- The tool never silently uses a different yt-dlp, ffmpeg or deno version than the one pinned.
- All three supported platforms pass the integration tests.

### Non-goals

A GUI or web UI, batch/playlist mode, a config file, gifski, trimming to a target file size, a `deps update` command, macOS and Windows-arm64 support, and a CI matrix. Each can be added later.

## Supported platforms

`windows/amd64`, `linux/amd64`, `linux/arm64`. Each is tested: Windows natively, Linux in Docker (arm64 under QEMU emulation). On any other platform `deps install` exits with "unsupported platform; use `--no-pin` with your own yt-dlp and ffmpeg on PATH". `--no-pin` runs everywhere but is not supported.

## CLI

```
vid2gif clip <url> --start T [--end T] [--format gif,webp,mp4,mov]
                   [--out DIR] [--name BASE] [--quality low|medium|high]
                   [--overwrite] [--no-pin]
vid2gif deps install
```

`vid2gif help` (also `-h`, `--help`, and `vid2gif clip -h`) prints the commands, an example, and every `clip` flag with its default. The flag list is generated from the flag definitions, so it cannot drift from the code.

The URL is taken as the first argument after `clip`; Go's `flag` package parses everything after it, so flags may follow the URL.

| Flag | Default | Notes |
|---|---|---|
| `--start` | required | `SS`, `MM:SS` or `HH:MM:SS`; the last component may have decimals (`1:23.5`) |
| `--end` | end of the video | must be after start; with no `--end`, yt-dlp gets `*START-inf` and the default name ends in `-end` (internally `End == 0`) |
| `--format` | `gif` | comma-separated subset of `gif,webp,mp4,mov` |
| `--out` | `./gifs` | created if missing |
| `--name` | `<sanitized-title>_<start>-<end>` | base name, no extension; each format adds its own |
| `--quality` | `medium` | see table below |
| `--overwrite` | off | replace existing outputs |
| `--no-pin` | off | skip version checks and use yt-dlp/ffmpeg/deno from PATH (deno optional) |

Exit codes: `0` success (including skipped outputs), `1` runtime failure (download or encode failed, tool missing or wrong version), `2` usage error (bad flag or timestamp).

### Quality presets

| Preset | fps | max width | GIF dither | WebP quality | mp4/mov CRF |
|---|---|---|---|---|---|
| low | 10 | 320 | bayer | 50 | 28 |
| medium | 15 | 480 | bayer | 70 | 23 |
| high | 24 | 720 | sierra2_4a | 90 | 18 |

Width is a cap: sources narrower than the cap are not upscaled. fps and width presets apply to GIF and WebP only. mp4/mov keep the source resolution and frame rate and use the preset only for CRF. There are deliberately no `--fps`/`--width` overrides; add them if someone needs them.

## Pipeline (`clip`)

1. **Validate** timestamps and flags. Nothing touches the network on a usage error.
2. **Resolve tools** (see Dependencies) and verify versions.
3. **Resolve the name.** If `--name` is absent, run `yt-dlp --print title --no-playlist <url>` (metadata only, no download) and build `<sanitized-title>_<start>-<end>`. Sanitizing replaces `<>:"/\|?*` and control characters, collapses whitespace to `_`, trims trailing dots and spaces, caps the title at 80 characters, and falls back to `clip` if the result is empty.
4. **Plan outputs.** For each requested format, if `<out>/<name>.<ext>` exists and `--overwrite` is not set, print `skipped <file> (already exists; use --overwrite to replace)` and drop it from the plan. If nothing is left, stop with exit `0` before downloading.
5. **Download once** into a temp dir created inside `--out` (`.vid2gif-tmp-*`, so the final move is a same-volume rename). yt-dlp is run with:
   `--no-playlist --download-sections "*START-END" --force-keyframes-at-cuts --merge-output-format mkv --ffmpeg-location <pinned ffmpeg dir> --ignore-config --no-js-runtimes --js-runtimes deno:<pinned deno> -o <tmp>/clip.%(ext)s`
   plus `-f "bv*+ba/b"` when mp4 or mov is planned, or `-f "bv*/b"` (no audio download) when only gif/webp are planned. yt-dlp's stdout and stderr stream to the terminal. `--ffmpeg-location` is mandatory; without it yt-dlp would use whatever ffmpeg is on PATH and defeat the pin.
6. **Encode each planned format** from the downloaded clip into the temp dir, then rename into `--out`. A failure in one format does not delete outputs already written; the run continues with the remaining formats and exits `1` at the end.
7. **Clean up** the temp dir (also on Ctrl+C, via a cancellable context passed to every child process).
8. **Summarize**: print each written path with its size, and a final line such as `wrote 2, skipped 1`.

### ffmpeg invocations

- **GIF:** `-an -vf "fps=F,scale='min(W,iw)':-1:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse=dither=D" -loop 0`
- **WebP:** `-an -vf "fps=F,scale='min(W,iw)':-1:flags=lanczos" -c:v libwebp_anim -quality Q -loop 0`
- **MP4/MOV:** `-c:v libx264 -crf C -preset medium -pix_fmt yuv420p -vf "scale=trunc(iw/2)*2:trunc(ih/2)*2" -c:a aac -b:a 128k -movflags +faststart`. The scale filter keeps dimensions even, which `yuv420p` requires. The container comes from the file extension.

(Commas inside `min()` are escaped in the actual argument.)

## Dependencies

### `deps.json`

One entry per tool, keyed by `GOOS-GOARCH`:

```json
{
  "yt-dlp": {
    "version": "2026.08.19",
    "platforms": {
      "windows-amd64": {
        "url": "https://github.com/yt-dlp/yt-dlp/releases/download/2026.08.19/yt-dlp.exe",
        "sha256": "66674953fe251b89f4d08c5f0e35e0728679bd67ab3d7d05c0562af101dd3e7a",
        "archive": "none"
      },
      "linux-amd64": {
        "url": "https://github.com/yt-dlp/yt-dlp/releases/download/2026.08.19/yt-dlp_linux",
        "sha256": "58162f9bfdc27458ea47bfcb311cf47028f17d8154a8bf7d689861d46399230a",
        "archive": "none"
      },
      "linux-arm64": {
        "url": "https://github.com/yt-dlp/yt-dlp/releases/download/2026.08.19/yt-dlp_linux_aarch64",
        "sha256": "b16e4dab368a816cd05d477d698a605a6ae87ccee1c8ffd38fa21d7254141fcc",
        "archive": "none"
      }
    }
  },
  "ffmpeg": {
    "version": "N-127588-g0b19c65afe",
    "platforms": {
      "windows-amd64": {
        "url": "https://github.com/yt-dlp/FFmpeg-Builds/releases/download/autobuild-2026-10-16-18-04/ffmpeg-N-127588-g0b19c65afe-win64-gpl.zip",
        "sha256": "51aced1016570cbda6712bcf36aa05809dd195229416fc61e3865a359d3f43ca",
        "archive": "zip"
      },
      "linux-amd64": {
        "url": "https://github.com/yt-dlp/FFmpeg-Builds/releases/download/autobuild-2026-10-16-18-04/ffmpeg-N-127588-g0b19c65afe-linux64-gpl.tar.xz",
        "sha256": "48282ca75e1364207c7c7e0d316527af0426ddf87d5cfbb672f46ad63d802708",
        "archive": "tar.xz"
      },
      "linux-arm64": {
        "url": "https://github.com/yt-dlp/FFmpeg-Builds/releases/download/autobuild-2026-10-16-18-04/ffmpeg-N-127588-g0b19c65afe-linuxarm64-gpl.tar.xz",
        "sha256": "81ce8bb9507dc960737ced800ef47748d36495205d90467fc69b57c10561e7c3",
        "archive": "tar.xz"
      }
    }
  }
}
```

The URLs and SHA-256 values above were read from the upstream release pages (yt-dlp `2026.08.19` `SHA2-256SUMS`; FFmpeg-Builds `autobuild-2026-10-16-18-04` `checksums.sha256`) on 2026-10-17. The plain `yt-dlp` release asset is a Python zipapp and is deliberately not used; `yt-dlp_linux*` are standalone binaries.

`deps.json` and `bin/` live next to the executable. If `deps.json` is not next to the executable (for example under `go run`), the current directory is used instead.

### `deps install`

For each tool: if `bin/<tool>` already reports the pinned version, do nothing. Otherwise download the platform asset, verify its SHA-256 while streaming it to disk (the file is deleted on mismatch), extract only the binary (`archive: zip`/`tar.xz` → the entry `<dir>/bin/<tool>` (ffmpeg) or `<tool>` at the archive root (deno); `none` → the file itself), set the executable bit on Linux, run `--version`, and fail if it does not match the pin.

### Resolution on every run

For each tool: `bin/<tool>` first, then PATH. Run `--version` and compare with the pin:

- yt-dlp: output must equal the pinned version exactly (`2026.08.19`).
- deno: the first line of `deno --version` must start with `deno <pin> ` (the real line is `deno 2.9.7 (stable, release, ...)`).
- ffmpeg: the first line of `ffmpeg -version` must contain `ffmpeg version <pin>` as a prefix match (the real string is `N-127588-g0b19c65afe-20261016`; only the revision and hash are pinned).

A mismatch exits `1` with the pinned version, the version found, and "run `vid2gif deps install`". If yt-dlp itself fails, its error is shown followed by a hint that the pinned version is `X` and can be bumped in `deps.json` or bypassed with `--no-pin`. `--no-pin` skips resolution checks entirely.

### Archive handling and the one dependency

Go's standard library has no xz decompressor and the Linux ffmpeg builds ship only as `.tar.xz`. The tool therefore uses **`github.com/ulikunitz/xz`** (pure Go, no cgo) — the only non-standard-library dependency of the shipped binary. The integration test also uses `github.com/joho/godotenv` to read an optional `.env` file; it is not part of the binary. The alternative of shelling out to `tar -xJf` was rejected because minimal Linux images often lack `xz`.

## Error handling summary

| Condition | Behaviour |
|---|---|
| Bad timestamp, `end <= start`, unknown format/quality | exit 2 before any network use |
| Unsupported platform for `deps install` | exit 1 with the `--no-pin` hint |
| Tool missing or version mismatch | exit 1 with the install/fix command |
| SHA-256 mismatch on download | delete the file, exit 1 |
| deno missing or not the pinned version (without `--no-pin`) | exit 1 with the `deps install` hint, before any download |
| yt-dlp or ffmpeg non-zero exit | exit 1; their output is already on screen |
| Existing output | skipped with a message; exit 0 |
| Ctrl+C | children cancelled, temp dir removed |

## Testing

- **Unit (table-driven):** timestamp parsing, end-default and validation, version matching (yt-dlp exact, ffmpeg prefix), quality-to-argument mapping for each format, filename sanitizing, `deps.json` lookup by platform, skip-existing planning (including the "everything skipped, no download" case).
- **Integration (`//go:build integration`):** runs `deps install` (hash and version check), generates a 3-second clip with ffmpeg `lavfi testsrc`, encodes it to gif, webp, mp4 and mov, and asserts each output exists, is non-empty, and has the right signature (`GIF89a`; `RIFF`…`WEBP`; `ftyp` at byte offset 4). A second case pre-creates `demo.gif`, runs with `--format gif,mp4`, and asserts the GIF is untouched, the MP4 is written, and the exit code is 0. This test also confirms each pinned ffmpeg contains `libx264` and `libwebp_anim`.
- **Manual:** one real yt-dlp download of a short, stable video, run by hand with the built binary (the only check of the `--download-sections` syntax against a real site).
- **Docker:** `Dockerfile.test` builds the tool and runs the integration tests for `--platform linux/amd64` and `linux/arm64`. Windows runs the same tests natively.

## Layout

```
cmd/vid2gif/main.go          subcommand dispatch, flag parsing, exit codes
internal/clip/clip.go        download / encode pipeline, yt-dlp wiring
internal/encode/encode.go    quality presets, ffmpeg argument builder and runner
internal/naming/             timestamps (timestamp.go), formats, file names, skip-existing planning (plan.go)
internal/deps/               deps.json, version matching, tool resolution (deps.go); download, SHA-256, extract, install (install.go)
test/integration/            real-tools test behind the integration build tag
build/Dockerfile.test        Linux integration tests (build context is the repo root); layered so modules and the pinned tools are cached separately from code
build/installtools/          installs every tool in deps.json (used by the Docker tools layer)
deps.json                    pins, next to the built executable
*_test.go                    unit tests, next to the code they cover
README.md                    usage, pin bumping, supported platforms, licensing note
```

`main` is the only `package main`; everything else is under `internal/`. The Go module is named `vid2gif` (renameable to a hosting path such as `github.com/<owner>/vid2gif` before publishing).

## Risks and assumptions

- **Pinned ffmpeg builds can disappear.** FFmpeg-Builds keeps a limited set of dated releases (38 existed on 2026-10-17, the oldest from November 2024). A pin pointing at an old build may eventually 404; the fix is to bump `deps.json` or mirror the asset.
- **yt-dlp goes stale.** A pinned yt-dlp will eventually fail when a site changes. Bumping the pin (version, URL, hash) is a manual edit by design.
- **Archive layout.** Both the Windows zip and the Linux tarballs hold `<dir>/bin/ffmpeg[.exe]`. The integration test confirmed this on Windows, Linux amd64 and Linux arm64.
- **Codec support in the pinned builds.** `libx264` and `libwebp_anim` are present in all three pinned builds; the integration test asserts this and encodes every format, including sources with odd dimensions and no audio.
- **YouTube needs deno.** deno `2.9.7` (MIT) is pinned in `deps.json` (zip assets, SHA-256 from the release `.sha256sum` files). yt-dlp is told to use only that copy (`--no-js-runtimes --js-runtimes deno:<path>`). On 2026-10-17 yt-dlp `2026.08.19` still returned the same formats for a normal video without deno and only warned that YouTube extraction without a runtime is deprecated, so the requirement protects against a future breakage rather than a current one. A real YouTube clip was verified end to end with the pinned tools; one well-known video (`jNQXAC9IJRQ`) returns "This video is unavailable" regardless of deno.
- **ffmpeg is GPL.** It is downloaded at install time, not redistributed with this tool; the README says so.
- **Windows arm64** builds exist upstream (`winarm64-gpl.zip`, `yt-dlp_arm64.exe`) and can be added as `deps.json` entries once someone can test them.
- **The author's installed yt-dlp (2026.03.03) is older than the pin.** The managed `bin/` copy takes precedence over PATH, so this doesn't affect the tool.
