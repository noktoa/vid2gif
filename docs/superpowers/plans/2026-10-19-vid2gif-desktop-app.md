# vid2gif Desktop App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Windows desktop app that lets a non-technical person drop in video links or a playlist, set Start/Stop per video, and make GIF/WebP/MP4/MOV clips with progress and an in-app preview, sharing one engine with the CLI.

**Architecture:** New logic lives in small, platform-neutral `internal/` packages (settings, lookup, runner, filesrv) plus additions to `clip`, `encode` and `deps`, all unit-tested on Linux CI. A thin Wails shell (`cmd/vid2gif-app`, Windows-only) binds them to a plain HTML/CSS/JS page that ports the approved layout-B mock. The CLI keeps its behaviour.

**Tech Stack:** Go 1.26.2, Wails v2.17.0 (WebView2), plain HTML/CSS/JS (no framework, no bundler), `node --test` for pure JS helpers.

**Spec:** `docs/superpowers/specs/2026-10-18-vid2gif-desktop-design.md` (builds on `2026-10-17-vid2gif-design.md`). Visual reference: `docs/superpowers/specs/2026-10-18-vid2gif-desktop-mockup.html` (layout B is the second window). Product and direction: `PRODUCT.md`, `.impeccable/surfaces/frontend-index-html.md`. Executors read all of these.

## Global Constraints

- Windows amd64 only for the app. The CLI stays Windows amd64 + Linux amd64/arm64; `go vet ./...` and `go test ./...` must keep passing on Linux CI, so every file that imports Wails carries `//go:build windows`.
- Pinned yt-dlp, ffmpeg and deno exactly as `deps.json`; the app never uses other versions and never passes `--no-pin`.
- Times are `SS`, `MM:SS` or `HH:MM:SS`, decimals on the last part only (`naming.ParseTimestamp`). Start required. Stop optional = to the end of the video. Stop must be after Start.
- Formats `gif, webp, mp4, mov` (at least one stays on, default `gif`). Quality labels Small file / Balanced / Sharp = `low` / `medium` / `high`, default Balanced. Default folder `Videos\Clips` in the user's profile.
- A link that is `watch?v=…&list=…` is one video (CLI `--no-playlist`). Only a pure playlist link expands.
- Clips run one at a time. Stop cancels the whole run; the current clip returns to ready and its `.vid2gif-tmp-*` folder is removed; unstarted clips stay ready; finished clips stay done.
- Existing-file rule: skip unless overwrite. UI shows "Already in your folder" and writes nothing (no overwrite option in the app).
- Never show raw tool output, tool names, flags or "CRF" in the UI. Plain-language errors only.
- Window: default 1000 x 680, minimum 480 x 560, must work properly at 600 px wide. Breakpoints: wide ≥ 900, medium 640–900, narrow < 640. No horizontal scroll at any width. Click targets ≥ 32 px high.
- Snappy: window visible in about a second; UI never blocks while clips run; a pasted link shows a slip at once; progress events to the page are throttled to at most 10 per second per clip; the title comes from the lookup so yt-dlp is never started only to fetch a title (set `clip.Opts.Name`).
- Fonts bundled, no network at runtime for UI assets: Bowlby One (logo, stamps), Atkinson Hyperlegible (everything else), tabular figures for times and sizes via `font-variant-numeric: tabular-nums` (only if Atkinson's digits turn out not to be tabular, add Barlow Semi Condensed for those). Thumbnails are the only remote images.
- Fixed colours in every theme: ink `#1E1B24`; yellow `#FFD60A` (Make button, progress); magenta `#E6007E` (only the ring on the one clip under the eye); done green `#00805A` text / `#00A66E` fill; error `#C23B22`. Themes: grey (default) ground `#E4E8ED` slip `#F8FAFC` line `#CBD1D9` titlebar `#DCE1E8`; mint `#CDEBE4` `#F7FCFB` `#9BC7BD` `#B8DDD5`; lilac `#E4DEF6` `#FBF9FF` `#BDB3DB` `#D4CCEB`. Text ≥ 4.5:1 in every theme. No warm cream grounds.
- Style rules: no gradient text, no eyebrow labels above headings, no cards inside cards, no modal for the preview, no emoji as icons.
- Helper tools live in `<install folder>\bin` (next to the exe; `VID2GIF_BIN` overrides): one shared copy for an all-users install, one per user for a per-user install. Settings, logs and web-view data are per user under `%LOCALAPPDATA%\vid2gif`; set Wails' `WebviewUserDataPath` to `%LOCALAPPDATA%\vid2gif\webview`. `deps.json` embedded in the app. A missing or corrupt settings file falls back to defaults.
- Installer: MSI (WiX v4+), per-user default or all-users, user-chosen folder, tools NOT bundled (downloaded on first launch), unsigned, plain exe still published. Uninstall is a clean removal of the program and all app data (see Task 11b) and never touches the user's clips or output folder.
- Progress is honest (spec "Progress accuracy"): one bar plus a stage label; percentages only on measured stages (getting the clip, MP4/MOV encode); never 100% while working, never backwards, no time estimates; unknown clip length = indeterminate bar.
- Smaller source by default: GIF/WebP-only runs download the smallest format at least as wide as the quality preset's width (320/480/720), best available if none is. `Settings.FullSource` / CLI `--full-source` / any MP4 or MOV in the run = best available. Output dimensions never change.
- Commit the design inputs before Task 1: `PRODUCT.md`, both 2026-10-18 specs, this plan, `.impeccable/config.json`, `.impeccable/surfaces/`.

## Review Focus

Inputs and conditions the spec implies but does not spell out. Each has a test in the task named.

1. Pasted text that is not a clean URL (blank, several links separated by newlines/spaces/commas, trailing whitespace, duplicates, `javascript:` or file paths): only distinct http(s) URLs are accepted. (Task 2)
2. Output folder that does not exist, is not writable, or was deleted between runs: the slip says "Couldn't save to that folder. Pick another one." and the run continues. (Task 6)
3. Stop time past the end of the video, Start past the end, or a live/unknown-length video (duration 0): a plain message, or indeterminate progress instead of a bogus percent. (Tasks 6, 4a)
4. No internet during lookup or during the first-launch download: a plain message and a Try again path, no crash, no half-installed tool. (Tasks 5, 6)
5. Closing the window or pressing Stop mid-clip: child processes end and no `.vid2gif-tmp-*` folder is left behind. (Tasks 3, 8)

---

### Task 1: Settings

**Files:**
- Create: `internal/settings/settings.go`, `internal/settings/settings_test.go`

**Interfaces:**
- Produces:
  - `type Settings struct { Theme string; OutDir string; Formats []string; Quality string; FullSource bool; DebugMode bool }` with JSON tags `theme,outDir,formats,quality,fullSource,debugMode` (`Default().DebugMode` is false; `Default().FullSource` is false = the faster download).
  - `var Themes = []string{"grey", "mint", "lilac"}`
  - `func Default() Settings` (theme `grey`, `OutDir` = `<user home>\Videos\Clips` via `os.UserHomeDir`, formats `["gif"]`, quality `medium`)
  - `func Load(path string) Settings` never fails; `func (s Settings) Save(path string) error` (creates the folder).

- [ ] **Step 1: Write failing tests** in `settings_test.go`: `TestLoadMissingFileGivesDefault`; `TestSaveLoadRoundTrip`; `TestLoadCorruptFileGivesDefault` (file contains `{not json`); `TestLoadUnknownThemeFallsBackToGrey`; `TestLoadEmptyFormatsFallsBackToGif`; `TestLoadUnknownQualityFallsBackToMedium`; `TestSaveCreatesParentFolder`; `TestDefaultIsFasterDownload`; `TestDefaultDebugModeOff`.
- [ ] **Step 2:** Run `go test ./internal/settings/` — expect FAIL (package missing).
- [ ] **Step 3:** Implement the signatures above; `Load` validates each field independently (a bad theme must not discard a good folder). Write with a temp file + rename.
- [ ] **Step 4:** Run `go test ./internal/settings/` — PASS.
- [ ] **Step 5:** Commit design inputs (see Global Constraints) then `git add internal/settings && git commit -m "feat: add app settings"`.

### Task 1b: Debug log

**Files:**
- Create: `internal/debuglog/debuglog.go`, `internal/debuglog/debuglog_test.go`

**Interfaces:**
- Produces:
  - `type Log struct{…}` wrapping `log/slog` (a text handler on the run's file); `func New(dir string, enabled bool) *Log`; `func (l *Log) SetEnabled(on bool)`; `func (l *Log) Printf(format string, args ...any)` (timestamped line; no-op when disabled; safe for concurrent use); `func (l *Log) Dir() string`.
  - `func Redact(s string) string` — removes the query string of any `http(s)` URL host-agnostically except keeping `v=` and `list=` for youtube.com/youtu.be page links, and removes `ip=…`, `expire=…`, `sig=…` style parameters wherever they appear.
  - A nil `*Log` is valid and does nothing, so callers never need a nil check.
- Rotation: a new file per app run named `vid2gif-<yyyymmdd-hhmmss>.log`, created lazily on the first write after enabling; at startup delete all but the newest 4 existing log files so at most 5 remain. No size cap.

- [ ] **Step 1: Write failing tests:** `TestDisabledCreatesNoFile` (no directory, no file); `TestEnabledWritesTimestampedLine`; `TestSetEnabledMidRun` (off → on → writes appear only after); `TestNilLogIsSafe`; `TestRedactSignedMediaURL` (a `googlevideo.com/videoplayback?expire=…&ip=…&sig=…` URL loses its query); `TestRedactKeepsYouTubeVideoID`; `TestRotationKeepsFiveNewest`; `TestConcurrentPrintf` under `-race`.
- [ ] **Step 2:** `go test ./internal/debuglog/` — FAIL.
- [ ] **Step 3:** Implement with a mutex and lazy file creation on the first write after enabling.
- [ ] **Step 4:** `go test -race ./internal/debuglog/` — PASS.
- [ ] **Step 5:** Commit `feat: debug log`.

### Task 2: Lookup (links, playlists, metadata)

**Files:**
- Create: `internal/lookup/lookup.go`, `internal/lookup/lookup_test.go`, `internal/lookup/testdata/single.jsonl`, `internal/lookup/testdata/playlist.jsonl`

**Interfaces:**
- Consumes: `clip` package's pinned-tool argument style (`--ignore-config`, `--no-js-runtimes --js-runtimes deno:<path>`); reuse by exporting `clip.JSRuntimeArgs(deno string) []string` (rename of the existing private `jsRuntimeArgs`, update its callers and tests).
- Produces:
  - `type Video struct { ID, URL, Title, ThumbnailURL string; DurationSec float64; Unavailable bool; Reason string }`
  - `func ParseInput(text string) []string`
  - `func Parse(jsonl []byte) (source string, vids []Video, err error)` — one parser for yt-dlp's JSON lines: a single video gives one line, a playlist gives one per entry. `source` is the playlist title when lines carry `playlist_title`, else `"Pasted links"`.
  - `type Looker struct { YtDlp, Deno string; Log *debuglog.Log }` (logs the command line and yt-dlp's stderr when enabled); `func (l Looker) Lookup(ctx context.Context, url string) (source string, vids []Video, err error)` — `source` as in `Parse`.

- [ ] **Step 1: Write failing tests.** `TestParseInput` table (Review Focus 1): blank → none; `"a b\nc,d"` splits on whitespace and commas; trims; drops non-http(s) (`javascript:alert(1)`, `C:\x.mp4`); dedupes keeping first order. `TestParseSingleVideoLine` against `testdata/single.jsonl` (fields `id,title,duration,thumbnail,webpage_url`; a live video with no `duration` → 0; source `Pasted links`). `TestParsePlaylistLines` against `testdata/playlist.jsonl` (one JSON object per line, `playlist_title` on each, one entry titled `[Private video]` → `Unavailable` with `Reason` "private"; one `[Deleted video]` → "removed"; source is the playlist title). Fixtures: capture real output once with `yt-dlp --flat-playlist --dump-json --no-playlist` on a public video and on a small public playlist, trim the playlist to 3 lines.
- [ ] **Step 2:** Run `go test ./internal/lookup/` — FAIL.
- [ ] **Step 2b: Verify the one-command assumption with the real tool** before relying on it: `yt-dlp --flat-playlist --dump-json --no-playlist -- <url>` must give (a) one line with duration and thumbnail for a plain video URL, (b) exactly one line for `watch?v=…&list=…`, (c) one line per entry for a `playlist?list=…` URL. If any of these fails, fall back to two commands (a playlist-URL check plus the old per-case flags) and say so in the commit message.
- [ ] **Step 3:** Implement `ParseInput` and `Parse`. `Lookup` runs yt-dlp once with `--ignore-config`, the JS runtime args and `--flat-playlist --dump-json --no-playlist`, URL after `--`, honouring `ctx`; a yt-dlp failure returns an error whose text keeps yt-dlp's stderr for `runner.PlainError` to classify.
- [ ] **Step 4:** Run `go test ./...` — PASS (the `clip` rename must not break anything).
- [ ] **Step 5:** Commit `feat: lookup links and playlists`.

### Task 2b: Quality in file names

**Files:**
- Modify: `internal/naming/plan.go`, `internal/naming/plan_test.go`, `internal/clip/clip.go` (its `BuildName` call), `README.md`, `docs/superpowers/specs/2026-10-17-vid2gif-design.md`

**Interfaces:**
- Produces: `func naming.BuildName(title string, start, end float64, quality string) string` — `<title>_<start>-<end>` for `medium` (unchanged), `…_low` for `low`, `…_high` for `high`. An explicit `--name` never goes through it. Later tasks (runner, Task 6) call it with the job's quality from settings.

- [ ] **Step 1: Write failing tests:** `TestBuildNameMediumUnchanged` (all existing expectations still pass with `"medium"`); `TestBuildNameLowAndHighHaveSuffix`; `TestBuildNameEndLabelWithQuality` (`title_83-end_high`); `TestPlanOutputsOtherQualityIsNotSkipped` (create `x_1-2.gif`, then plan the `_high` name for gif: it is in `todo`, not `skipped`); `TestPlanOutputsSameQualityIsSkipped`.
- [ ] **Step 2:** `go test ./internal/naming/` — FAIL (signature).
- [ ] **Step 3:** Implement and update the one existing caller in `clip.Run` (`o.Quality`) and its tests; document the naming rule in the README and the CLI spec's naming section.
- [ ] **Step 4:** `go test ./...` — PASS. Manual: make a clip at `--quality low` and again at `--quality high`; two files exist, nothing skipped.
- [ ] **Step 5:** Commit `feat: quality in default file names`.

### Task 3: Clip events and cleanup

**Files:**
- Modify: `internal/clip/clip.go`, `internal/clip/clip_test.go`

**Interfaces:**
- Produces (in package `clip`):
  - `type Event struct { Stage string; Pct float64; Path string; Size int64; Err error }`
  - Stage constants `StageFind="find"`, `StageDownload="download"`, `StageEncode="encode"`, `StageWrote="wrote"`, `StageSkipped="skipped"`, `StageFailed="failed"`. `Pct` is 0..1 within the stage, or `-1` when unknown. For `StageEncode` and `StageFailed`, `Path` is the final output path.
  - `func (p Pipeline) WithLog(l *debuglog.Log) Pipeline` — when the log is enabled, records each yt-dlp and ffmpeg command line (through `debuglog.Redact`), logs each line of the tools' stderr as `yt-dlp: …` / `ffmpeg: …` with `l.Printf` (the pipeline already scans those lines for progress), and records every `Event` and the final error. A nil log changes nothing, and CLI output is unaffected.
  - New `Opts` fields: `OnEvent func(Event)` (nil = ignore) and `DurationSec float64` (length of the clip if known; 0 = unknown).
  - Internal: `Pipeline.download` and `Pipeline.encode` get an extra `progress func(frac float64)` argument so Task 4 can feed them; fakes in tests ignore it.
- CLI output text is unchanged.

- [ ] **Step 1: Write failing tests** with the existing `fakePipeline`: `TestRunEmitsEventsInOrder` (find → download → encode(gif) → wrote(gif) → encode(mp4) → wrote(mp4) with sizes); `TestRunEmitsSkippedWhenAllExist` (one `skipped` event per format, no download event); `TestRunEmitsFailedAndContinues` (gif encode fails, mp4 succeeds: events `failed` then `wrote`, error returned); `TestRunCancelRemovesTempDir` (cancel the context inside the fake download; assert no `.vid2gif-tmp-*` remains in `OutDir` — Review Focus 5); `TestRunNilOnEventStillWorks`; `TestWithLogRecordsCommandsAndEvents` (enabled log in a temp dir receives the redacted argv and every event); `TestWithLogDisabledWritesNothing`.
- [ ] **Step 2:** Run `go test ./internal/clip/` — FAIL.
- [ ] **Step 3:** Add the types and call `o.OnEvent` where `Run` already prints. Keep every existing `fmt.Fprintf(p.out, …)` line.
- [ ] **Step 4:** Run `go test ./...` — PASS (all pre-existing clip tests untouched).
- [ ] **Step 5:** Commit `feat: clip pipeline events`.

### Task 4a: Real progress parsing

**Why this shape:** measured on 2026-10-19 (spec "Progress accuracy"): yt-dlp prints no percentages for a sectioned download; the ffmpeg it starts writes `time=HH:MM:SS.xx` stats lines to stderr, separated by `\r`. GIF and WebP encodes give no usable progress; MP4/MOV give `out_time_us`.

**Files:**
- Modify: `internal/encode/encode.go`, `internal/encode/encode_test.go`, `internal/clip/clip.go`, `internal/clip/clip_test.go`
- Create: `internal/clip/testdata/ffmpeg-section-stderr.txt` (copy of `docs/superpowers/plans/fixtures/ffmpeg-section-stderr.txt`, real captured lines)

**Interfaces:**
- Consumes: Task 3's `progress func(frac float64)` parameters and `StageFind`/`StageDownload`.
- Produces:
  - `func clip.ParseFFmpegTime(line string) (seconds float64, ok bool)` — reads `time=HH:MM:SS.xx`; `time=N/A` and lines without it give `ok=false`.
  - `func clip.SplitProgress(data []byte, atEOF bool) (advance int, token []byte, err error)` — a `bufio.SplitFunc` that splits on `\r` or `\n`.
  - `encode.Opts` gains `OnProgress func(frac float64)` and `DurationSec float64`; `func encode.ParseProgress(line string, durationSec float64) (frac float64, ok bool)` reads `out_time_us=<n>`.
- Behaviour: `Pipeline` emits `StageFind` (Pct -1) when the download begins and switches to `StageDownload` on the first valid `time=` line, with `Pct = seconds / Opts.DurationSec` clamped to 0..1, or -1 when `DurationSec` is 0. `OnProgress` is only given to MP4/MOV encodes, and the emitted fraction is clamped to be non-decreasing; GIF/WebP encodes emit `Pct -1`.

- [ ] **Step 1: Write failing tests:** `TestParseFFmpegTimeFixture` (scan the fixture with `SplitProgress`; the first `ok` line is 0.05 s, the last `ok` line is 8.63 s, `time=N/A` is skipped, values never decrease); `TestSplitProgressSplitsOnCR`; `TestDownloadFractionUsesDuration` (5 s of 10 → 0.5; Review Focus 3: `DurationSec` 0 → -1); `TestParseProgressOutTime` (`out_time_us=2500000`, duration 5 → 0.5; duration 0 → not ok; above 1 clamps); `TestEncodeFractionNeverDecreases` (feed `out_time_us` values 55 s, 54 s, 56 s of a 60 s clip: emitted fractions are non-decreasing; measured on 2026-10-19, ffmpeg steps back about 1 s once per MP4 encode); `TestArgsUnchangedWithoutProgress`; `TestRunnerAddsProgressFlags` (`-progress pipe:1 -nostats` before the output path only when `OnProgress != nil`); `TestGifAndWebpEncodeGetNoProgressCallback`.
- [ ] **Step 2:** Run `go test ./internal/clip/ ./internal/encode/` — FAIL.
- [ ] **Step 3:** Implement. In `clip.New`'s download, run yt-dlp with stdout and stderr tee'd: still written to the terminal writer exactly as before for the CLI, and also scanned with `SplitProgress` + `ParseFFmpegTime`.
- [ ] **Step 4:** `go test ./...` PASS. Manual with the real tools on a 10 s clip: events are in order find → download (rising) → encode → wrote, and the CLI terminal output is identical to before.
- [ ] **Step 5:** Commit `feat: download and encode progress`.

### Task 4b: Smaller source for GIF and WebP

**Files:**
- Modify: `internal/clip/clip.go`, `internal/clip/clip_test.go`, `internal/encode/encode.go`, `cmd/vid2gif/main.go`, `cmd/vid2gif/main_test.go`, `README.md`, `docs/superpowers/specs/2026-10-17-vid2gif-design.md`

**Interfaces:**
- Produces: `func encode.PresetWidth(quality string) int` (320 / 480 / 720); `clip.Opts.FullSource bool`; CLI flag `--full-source` (default off). `ytdlpDownloadArgs` picks the format as follows:
  - Any `mp4`/`mov` requested, or `FullSource`: today's selector (`bv*+ba/b` or `bv*/b`).
  - Otherwise (gif/webp only): the smallest video format whose width is at least `PresetWidth(o.Quality)`, falling back to the best available when none is that wide. Choose the yt-dlp selector/sort that achieves exactly this; the builder verifies it against the example video with `yt-dlp -F` / `--print format_id`. Do not use a height cap: it would shrink portrait videos below the preset width.

- [ ] **Step 1: Write failing tests:** `TestDownloadArgsGifOnlyUsesWidthSelector` (low, medium and high contain 320, 480, 720); `TestDownloadArgsMp4KeepsFullSelector`; `TestDownloadArgsFullSourceKeepsFullSelector`; `TestDownloadArgsMixedGifMp4KeepsFullSelector`; `TestParseClipArgsFullSource` (`--full-source` sets the field; default false); `TestPresetWidth`.
- [ ] **Step 2:** `go test ./...` — FAIL.
- [ ] **Step 3:** Implement; document `--full-source` in the flag help (so `vid2gif help` shows it), in the README options table, and replace the `-f "bv*/b"` text in the CLI spec.
- [ ] **Step 4:** `go test ./...` PASS. **Measure:** with the real tools, make the same 10 s GIF at each quality with and without `--full-source`; record the download seconds for both, and confirm the two GIFs have identical dimensions, in the commit message. If the speed-up is under 20% on this machine, say so in the commit message and ask the user whether to keep it as the default.
- [ ] **Step 5:** Measure the stage costs used by Task 6 once, after this change, with the same procedure: find (fixed seconds), getting the clip per clip-second, GIF per clip-second, WebP per clip-second, MP4/MOV per clip-second. Keep the numbers in the commit message until Task 6 creates `internal/runner/costs.go`, then hardcode them there as one `map[string]float64` with the date, machine and sample size in a comment. Commit `feat: smaller source for GIF and WebP (cli --full-source)`.

### Task 5: Tool install with progress, embedded pins

**Files:**
- Modify: `internal/deps/deps.go`, `internal/deps/install.go`, `internal/deps/install_test.go`
- Create: `embed.go` (repo root, `package vid2gif`), `internal/deps/parse_test.go`

**Interfaces:**
- Produces:
  - `embed.go`: `//go:embed deps.json` → `var DepsJSON []byte`.
  - `func deps.Parse(b []byte) (Pins, error)`; `Load` calls it.
  - `func InstallAllProgress(pins Pins, binDir string, log io.Writer, onProgress func(tool string, done, total int64)) error`; `InstallAll` calls it with `nil`.
  - `downloadVerified` gains a final `onProgress func(done, total int64)` argument (`total` from `Content-Length`, `-1` if unknown); update its three existing test calls to pass `nil`.
  - `func deps.BinDir(base string, getenv func(string) string) string` — `VID2GIF_BIN` if set (an absolute value as is, a relative one from `base`), else `base\bin`. The CLI's private `binDir` in `cmd/vid2gif/main.go` is replaced by a call to it.

- [ ] **Step 1: Write failing tests:** `TestParseDepsJSON` (the embedded bytes parse and contain `yt-dlp`, `ffmpeg`, `deno`); `TestDownloadVerifiedReportsProgress` (served body, assert last callback `done == len(body)`); `TestInstallAllProgressNamesTool`; `TestBinDir` (default `base\bin`; absolute env; relative env resolved from `base`); `TestInstallOfflineLeavesNoPartialFile` (Review Focus 4: unreachable URL → error, `dest` and `.new` files absent).
- [ ] **Step 2:** `go test ./internal/deps/` — FAIL.
- [ ] **Step 3:** Implement; wrap `resp.Body` in a counting reader.
- [ ] **Step 4:** `go test ./... && go vet ./...` — PASS; `go build ./...` still builds `build/installtools`.
- [ ] **Step 5:** Commit `feat: install progress and embedded pins`.

### Task 6: Runner (queue, validation, plain errors)

**Files:**
- Create: `internal/runner/runner.go`, `internal/runner/validate.go`, `internal/runner/errors.go`, `internal/runner/runner_test.go`, `internal/runner/validate_test.go`, `internal/runner/errors_test.go`

**Interfaces:**
- Consumes: `clip.Opts`, `clip.Event` (Task 3), `settings.Settings` (Task 1), `naming.BuildName`.
- Produces:
  - `type Job struct { ID, URL, Title string; Start, End, DurationSec float64 }`
  - `type File struct { Format, Path string; Size int64 }`
  - `type Event struct { JobID, Kind, Stage, Message string; Pct, StagePct float64; Files []File }` — `Kind` is one of `"making"` (`Stage` is `find`, `download` or the format name; `Pct` is the overall 0..1 or -1 when the clip length is unknown; `StagePct` is that stage's own 0..1 or -1 when unmeasured), `"done"`, `"partial"` (some formats made, `Message` names the failed one), `"failed"`, `"already"`, `"ready"` (cancelled, back to waiting).
  - `type ClipRunner interface { Run(ctx context.Context, o clip.Opts) error }`
  - `func Run(ctx context.Context, cr ClipRunner, jobs []Job, s settings.Settings, emit func(Event))` — sequential; sets `Opts.Name = naming.BuildName(job.Title, job.Start, job.End, s.Quality)`, `Formats`, `Quality`, `OutDir` and `FullSource` from settings (`Overwrite` is always false); maps clip events to runner events (overall percent = download × 0.5 + encode share × 0.5, throttled to 10 events/s per job); a failed job never stops the next.
  - `func Validate(startText, endText string, durationSec float64) (start, end float64, msg string)` — empty `msg` = valid.
  - `func PlainError(err error, debugOn bool) string` — the generic message ends with the debug-mode hint described in the spec (off: "Turn on troubleshooting in Settings and try again to save details."; on: "Details were saved to the log.").
  - Wire names: `Job`, `File` and `Event` carry lowerCamelCase `json` tags (`id`, `url`, `title`, `start`, `end`, `durationSec`; `format`, `path`, `size`; `jobId`, `kind`, `stage`, `message`, `pct`, `stagePct`, `files`); Task 8's `Slip` does the same. The page uses these names.

- [ ] **Step 1: Write failing tests.** `TestValidate` table: `"" , ""` → "Add a start time to include this one."; `"1:12","1:19"` ok; `"1:12",""` ok (end 0); `"abc",""` → "That time doesn't look right. Try 1:23 or 0:01:23."; `"1:19","1:12"` → "Stop must be after Start."; `"5:00","",dur 120` → "That's past the end of the video."; `"1:00","3:00",dur 120` → "Stop is past the end of the video."; dur 0 (live) accepts any times (Review Focus 3). `TestPlainError` table: yt-dlp stderr containing "Private video" / "Video unavailable" → "Couldn't get this video. It may be private or removed."; "no such host" / "dial tcp" / "timeout" → "Couldn't reach the internet. Check your connection and try again."; "permission denied" / "access is denied" / "no such file or directory" on the output folder → "Couldn't save to that folder. Pick another one." (Review Focus 2); yt-dlp extraction/sign-in/bot-check/403 text (`Unable to extract`, `Sign in to confirm`, `HTTP Error 403`, `nsig extraction failed`) → "This site isn't working with the app right now. An update may fix it." (checked after the private/removed phrases, which win); anything else → "Something went wrong with this one." plus the hint for `debugOn` false and true, and the result never contains "yt-dlp", "ffmpeg", "exit status". `TestRunSequentialAndContinuesAfterFailure` (fake runner fails job 2, job 3 still runs); `TestRunPartial` (events: gif written, mp4 failed → `partial`); `TestRunAlready` (only skipped events → `already`); `TestRunOtherQualityMakesNewFile` (same job at `low` then `high` into one folder: two files, both `done`, nothing `already`); `TestRunStopMarksCurrentReadyAndStops` (cancel during job 1: `ready` for job 1, jobs 2–3 get no events); `TestRunBadFolder` (OutDir is a file path → `failed` with the folder message, next job still runs); `TestRunThrottlesMaking` (1000 fast fake events produce ≤ ~10 per second of fake time, always including the first and last); `TestOverallPctNeverDecreases`; `TestOverallPctStaysBelow1UntilDone` (the last `making` event is < 1, then `done` follows); `TestOverallPctUnknownLengthIsMinus1`; `TestCostsCoverEveryStage` (every stage name the pipeline can emit has a cost); `TestFullSourcePassedThrough` (settings → `clip.Opts.FullSource`); `TestRunLogsOriginalErrorWhenDebugOn` (a fake `debuglog.Log` receives the raw error and job id; with a nil log nothing breaks).

  Overall percent = (completed stage costs + current stage fraction × its cost) ÷ total cost, from the cost table in `internal/runner/costs.go` (find: fixed seconds; others: seconds per clip-second, measured once in Task 4b step 5), held at the stage start for unmeasured stages and clamped to be non-decreasing.
- [ ] **Step 2:** `go test ./internal/runner/` — FAIL.
- [ ] **Step 3:** Implement. Classify errors by substring on `err.Error()` (it contains yt-dlp's stderr via `clip`).
- [ ] **Step 4:** `go test ./...` — PASS.
- [ ] **Step 5:** Commit `feat: clip queue runner`.

### Task 7: Preview file server

**Files:**
- Create: `internal/filesrv/filesrv.go`, `internal/filesrv/filesrv_test.go`

**Interfaces:**
- Produces: `type Server struct{…}`; `func New() *Server`; `func (s *Server) Allow(dir string)` (add a folder the page may read from); `func (s *Server) URL(path string) (string, bool)` (`/out/<url-escaped absolute path>`, false when the file is not under an allowed folder); `func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request)` — serves a file only when its cleaned absolute path is inside an allowed folder and its extension is `.gif`, `.webp`, `.mp4` or `.mov`, using `http.ServeFile` (Range requests and content types come from the standard library); everything else is 404.

- [ ] **Step 1: Write failing tests:** `TestServesFileUnderAllowedFolder`; `TestRejectsPathOutsideAllowedFolders`; `TestRejectsTraversal` (`/out/..%2f..%2fsecret.gif`); `TestRejectsNonMediaExtension` (a `.txt` and a `.exe` inside the allowed folder); `TestRangeRequestGets206`; `TestVanishedFileIs404`; `TestContentTypes` (gif, webp, mp4).
- [ ] **Step 2:** `go test ./internal/filesrv/` — FAIL.
- [ ] **Step 3:** Implement with a mutex-guarded slice of allowed folders and `filepath.Rel` containment checks.
- [ ] **Step 4:** `go test ./...` — PASS.
- [ ] **Step 5:** Commit `feat: preview file server`.

### Task 8: Wails shell

**Files:**
- Create: `cmd/vid2gif-app/main.go`, `cmd/vid2gif-app/app.go`, `cmd/vid2gif-app/paths.go` (all `//go:build windows`), `cmd/vid2gif-app/frontend/.gitkeep` until Task 9
- Modify: `go.mod` (add `github.com/wailsapp/wails/v2 v2.17.0`), `go.sum`

**Interfaces:**
- Consumes: Tasks 1, 2, 5, 6, 7 and `vid2gif.DepsJSON`.
- Produces — Go methods bound to the page (`window.go.main.App.<Name>`):
  - `Boot() BootState` → `{ NeedsSetup bool; Settings settings.Settings; Themes []string }`
  - `Install()` — runs `deps.InstallAllProgress` into `deps.BinDir(<exe folder>, os.Getenv)`; emits event `setup` `{tool, done, total}`, then `setup:done` or `setup:error` `{message, needsAdmin}` (plain message; Review Focus 4; `needsAdmin` is true when the failure is a permission error on the `bin` folder, and the page then shows the Repair message instead of Try again).
  - `Add(text string) []Slip` — returns placeholder slips instantly (`{ID, URL}`); each lookup then emits `slips:replace` `{id, source, slips: []Slip}` where `Slip = {ID, URL, Title, ThumbnailURL, DurationSec, Unavailable, Reason}` (a playlist replaces one placeholder with many).
  - `Validate(start, end string, durationSec float64) string` → `runner.Validate` message.
  - `Make(jobs []runner.Job)` — starts `runner.Run` in a goroutine with a cancellable context; emits `slip:status` (the `runner.Event`, with `Files[].Path` converted to preview URLs via `filesrv`) and `run:finished`.
  - `OpenLogs()` opens `Log.Dir()` in Explorer; a `Show log` request for one slip is just `OpenLogs()`.
  - Failure and recovery events: a failed lookup emits `slips:replace` with the placeholder replaced by a slip that has `Failed: true` and a plain `Message` (from `runner.PlainError`); `Make` first resolves the tools and, if any is missing or the wrong version, emits `setup:needed` instead of starting, and the page shows the first-launch screen.
  - `Stop()`, `SaveSettings(s settings.Settings) error`, `PickFolder() string` (native folder dialog), `OpenFolder(path string)` (Explorer).
- Debug log wiring: `app.go` creates one `debuglog.New(<data root>\logs, settings.DebugMode)` at startup and passes it to `lookup.Looker.Log`, `clip.New(...).WithLog(log)` and the runner; `SaveSettings` calls `log.SetEnabled(s.DebugMode)` so the switch applies at once; when enabled, `Boot()` writes one line each for the app version, Windows version, the three tool versions and the resolved folders.
- `main.go`: window title `vid2gif`, 1000 x 680, min 480 x 560, `AssetServer` with embedded `frontend` plus `Handler` = the `filesrv.Server` (the app calls `Allow(OutDir)` whenever a run starts), `OnShutdown` cancels the run context and waits briefly for temp-folder cleanup (Review Focus 5). `paths.go`: the per-user data root `%LOCALAPPDATA%\vid2gif` holding `settings.json`, `logs` and `webview` (pass it as `windows.Options.WebviewUserDataPath`, so Wails does not create a data folder elsewhere), and the tools folder from `deps.BinDir`. `main.go` also handles `--install-tools` before creating any window: it runs `deps.InstallAllProgress` for the exe's own folder, writes progress lines to `%TEMP%\vid2gif-install.log`, and exits 0 on success and 1 on failure (this is what the MSI runs). `Boot()` reports `NeedsSetup` when any of yt-dlp, ffmpeg or deno fails `Resolver.Resolve(name, pin, false)` (a three-item loop in `app.go`).

- [ ] **Step 1:** Add the dependency and a build-tag test: `go build -tags desktop,production -o NUL ./cmd/vid2gif-app` fails until the files exist (record the failing output).
- [ ] **Step 2:** Implement the files above. Build command: `go build -tags desktop,production -ldflags "-H windowsgui" -o dist/vid2gif-app.exe ./cmd/vid2gif-app`. Resolve tools through `deps.NewResolver(binDir)` from the app's own bin folder; never PATH fallback for the pin (use `Resolve(name, pin, false)` after checking the bin file exists).
- [ ] **Step 3:** `go build` as above succeeds; `go vet ./...` and `go test ./...` pass on the Linux CI image (via `GOOS=linux go vet ./...` locally to confirm the build tag keeps the app out).
- [ ] **Step 4: Manual smoke** with a throwaway `frontend/index.html` that calls `Boot()` and prints the result: the window opens, `NeedsSetup` is true on a clean profile. (Run it on the user's desktop, not inside a sandboxed tool shell: WebView2 helper processes crashed there in the earlier spike.)
- [ ] **Step 5:** Commit `feat: wails shell`.

### Task 9: Frontend foundation (layout, themes, responsive)

**Files:**
- Create under `cmd/vid2gif-app/frontend/`: `index.html`, `styles.css`, `themes.css`, `fonts/` (woff2 + `fonts/README.md` with source and OFL licence per font), `package.json` (`{"private":true,"type":"module"}`), `contrast.js`, `contrast.test.js`, `dev-stub.js`

**Interfaces:**
- Produces: the static page structure and CSS classes the behaviour task relies on: `.counter`, `.sheet`, `.slip` (+ `.eye`, `.open`), `.stub`, `.main`, `.time` (+ `.need`), `.state`, `.stamp` (+ `.done`, `.bad`, `.wait`), `.progress`, `.inline-preview`, `.make`, `.chip`, `.seg`, `.setup-screen`, `.settings-menu`. Themes are applied by `data-theme="grey|mint|lilac"` on `<html>` with CSS custom properties `--ground --slip --line --tb`. `dev-stub.js` defines `window.go.main.App` and `window.runtime` fakes with sample data so the page can be opened in any browser.

- [ ] **Step 1: Write the failing test** `contrast.test.js`: for every theme token set in `themes.css` (parsed in the test), ink on ground, ink on slip, `#4A4655` (secondary text) on slip, `#00805A` on slip, `#C23B22` on slip must be ≥ 4.5:1, and ink on `#FFD60A` ≥ 7:1. Run `node --test cmd/vid2gif-app/frontend/` — FAIL (no themes).
- [ ] **Step 2:** Bundle the two fonts as local woff2 (`npm pack @fontsource/bowlby-one @fontsource/atkinson-hyperlegible`, copy the latin woff2 files, record licences). Check that Atkinson's digits are tabular with `tabular-nums` (render `1111` over `8888` and compare widths); only if they are not, also bundle `@fontsource/barlow-semi-condensed` for times and sizes. Port the mock's CSS and layout B markup; drop the fake OS title bar; define the three themes.
- [ ] **Step 3:** Progress bar: a stage label above it, a pulse on the fill for unmeasured stages and an indeterminate state, replaced by a static striped fill under `prefers-reduced-motion`. Make it responsive per the spec's three breakpoints: counter becomes a top block under 640 px with the Make button pinned to the bottom edge; slips stack to one column; inline preview takes full width; `.sheet` scrolls, the page never scrolls horizontally.
- [ ] **Step 4:** `node --test cmd/vid2gif-app/frontend/` — PASS.
- [ ] **Step 5: Manual check:** open `index.html` with `dev-stub.js` in a browser at widths 1000, 760, 600 and 480, in all three themes: no horizontal scroll, controls ≥ 32 px, nothing clipped, perforated stub intact. Fix in one batch. Commit `feat: app frontend layout and themes`.

### Task 10: Frontend behaviour

**Files:**
- Create under `cmd/vid2gif-app/frontend/`: `app.js`, `state.js`, `state.test.js`
- Modify: `index.html`

**Interfaces:**
- Consumes: the Go methods and events listed in Task 8 (if either side's shapes change, change both together and update that task's list).
- Produces: `state.js` pure functions, ESM exports: `addPlaceholders(slips, newSlips)`, `replaceSlip(slips, id, newSlips)`, `readyJobs(slips)` (slips with a valid Start and no unavailable flag), `applyStatus(slips, runnerEvent)`, `eyeId(slips, previewId)` (the previewed slip, else the one being made, else `null`), `makeLabel(n)` (`"Make 3 clips"`, `"Make 1 clip"`, disabled-looking `"Make clips"` at 0), `formatDuration(sec)`, `stageLabel(event)`.

- [ ] **Step 1: Write failing tests** `state.test.js`: `readyJobs` skips slips without Start and unavailable ones; `replaceSlip` expands one placeholder into many in place; `applyStatus` for each kind (`making`, `done`, `partial`, `failed`, `already`, `ready`); `eyeId` returns exactly one id or `null`, never two; `makeLabel(1)` is `"Make 1 clip"`; `formatDuration(272)` is `"4:32"`, `0` is `"—"`; `stageLabel` gives `"Finding the video…"` (stage `find`), `"Getting the clip 63%"` (stage `download`, stagePct 0.63), `"Making GIF…"` (stage `gif`, stagePct -1), `"Making MP4 40%"`, and never shows a percentage when `stagePct` is -1.
- [ ] **Step 2:** `node --test cmd/vid2gif-app/frontend/` — FAIL.
- [ ] **Step 3:** Implement `state.js`, then `app.js` wiring: drop and paste (a window-level paste handler and the Add field both call `Add`); drag-drop of link text (if WebView2 does not deliver dropped text, keep paste as the path and note it in the commit); per-slip time inputs validated on blur via `Validate`; format chips (at least one stays on), quality segments, Change folder → `PickFolder`; Make/Stop button; slip states and stamps exactly as the spec lists them (Next up, progress bar, DONE + formats, needs-start hint, NOT FOUND + Remove, "Already in your folder", partial note); inline preview with format tabs (GIF, WebP and MP4 players; MOV-only shows Open folder only), facts (size from `files[].size`; length from the job's Stop minus Start, or the player's `duration` when Stop is blank; width from the loaded image's `naturalWidth` or the video's `videoWidth`), Open folder, Close; the gear → `.settings-menu` with colour swatches and a "Download size" choice ("Faster (recommended)" / "Full size (slower, slightly sharper)") and a read-only "Helper files" path with an Open button (`OpenFolder`) and a "Troubleshooting" switch with its note, and an Open logs button, calling `SaveSettings`/`OpenLogs`; failed slips show their plain sentence, Remove, and (when troubleshooting is on) a Show log link; `setup:needed` shows the first-launch screen; the progress bar driven by `pct` and labelled with `stageLabel`; first-launch screen driven by `Boot().NeedsSetup`, `Install`, `setup` events, with a Try again button on `setup:error`; the magenta ring on `eyeId` only.
- [ ] **Step 4:** `node --test cmd/vid2gif-app/frontend/` — PASS.
- [ ] **Step 5: Manual run in the real app** on the user's desktop: first-launch screen and download; paste one link, a `watch?v=…&list=…` link, and a playlist link; make clips in two formats; stop one mid-way; force a NOT FOUND with a private link; switch themes and restart to confirm it persisted. Walk the spec's acceptance list and tick each item. Commit `feat: app behaviour`.

### Task 11: Release, CI, README

**Files:**
- Modify: `release.ps1`, `.github/workflows/ci.yml`, `README.md`

- [ ] **Step 1:** `release.ps1`: also build `dist\vid2gif-app.exe` with `go build -trimpath -tags desktop,production -ldflags "-H windowsgui -s -w" ./cmd/vid2gif-app`, include it in the checksums and the `gh release create` asset list. The app does not need `deps.json` beside it (embedded).
- [ ] **Step 2:** `ci.yml`: add a `windows` job (`runs-on: windows-latest`) that runs the app build command above, and a step in `unit` running `node --test cmd/vid2gif-app/frontend/`. Keep `gofmt`, `vet`, `test`.
- [ ] **Step 3:** README: a "Desktop app" section (what it is, download, first-launch note about the one-time ~250 MB download, where settings and tools live, Windows only, how to build it) and update the Architecture diagram with the new packages.
- [ ] **Step 4:** Push-less verification: run `release.ps1` up to its first prompt answers `n`, confirm `dist\vid2gif-app.exe` exists and starts; `go vet ./... && go test ./...` pass. Commit `chore: release and CI for the app`.

### Task 11b: MSI installer

**Files:**
- Create: `installer/vid2gif.wxs` (WiX v4+ source), `installer/test.ps1`
- Modify: `release.ps1`, `.github/workflows/ci.yml`, `README.md`

**Interfaces:**
- Consumes: `dist\vid2gif-app.exe` from Task 11.
- Produces: `dist\vid2gif-<x.y.z>.msi`, built by one command: `wix build installer\vid2gif.wxs -ext WixToolset.UI.wixext -ext WixToolset.Util.wixext -arch x64 -d Version=<a.b.c> -d AppExe=dist\vid2gif-app.exe -o dist\vid2gif-<a.b.c>.msi`, run from `release.ps1` (version from the release tag with the `v` removed; reject anything that is not `a.b.c`), from CI, and from `test.ps1`. Fixed `UpgradeCode` GUID, new `ProductCode` per version (WiX default behaviour for a major-upgrade package).

- [ ] **Step 1: Write the failing test** `installer/test.ps1`: builds an MSI as version 0.0.1 and another as 0.0.2, then installs per-user and without admin (`msiexec /i <msi> /qn MSIINSTALLPERUSER=1 ALLUSERS="" INSTALLFOLDER="<temp>\a b\vid2gif"` with a space in the path): asserts `vid2gif-app.exe` and the Start Menu shortcut exist, the app is listed in `HKCU\...\Uninstall`, and `<install>\bin\yt-dlp.exe`, `ffmpeg.exe` and `deno.exe` exist (the install-time `--install-tools` step ran against the real network; this is the same 250 MB the weekly integration test already downloads); installs 0.0.2 over 0.0.1 and asserts one uninstall entry, the new exe, and that `%TEMP%\vid2gif-install.log` says the tools were already installed; `msiexec /x` must remove the exe, the shortcut, the whole `bin` folder, and a pre-created fake `%LOCALAPPDATA%\vid2gif\settings.json` (use a throwaway user profile or the real folder with a unique marker file), and the install folder because it ends up empty. A second run installs into a non-empty existing folder and asserts that after uninstall the folder and its unrelated file are still there. Run before the .wxs exists — FAIL.
- [ ] **Step 2:** Write `vid2gif.wxs` with the Advanced UI (`WixToolset.UI.wixext`), `Scope="perUserOrMachine"`, default folder per-user `%LOCALAPPDATA%\Programs\vid2gif`, the exe, a Start Menu shortcut, and an icon if one exists. No tools inside the package. A deferred custom action (`Impersonate="no"`, `Return="ignore"`, after `InstallFiles`, on install, upgrade and Repair, not on uninstall) runs `[INSTALLFOLDER]vid2gif-app.exe --install-tools`. Clean removal with `util:RemoveFolderEx` on uninstall for `[INSTALLFOLDER]bin` and `[LocalAppDataFolder]vid2gif` (`WixToolset.Util.wixext`); never on `INSTALLFOLDER` itself, so a folder the user already had is not wiped. In an all-users install, uninstall can only clear the uninstalling user's `%LOCALAPPDATA%\vid2gif`; say so in the README.
- [ ] **Step 3:** `installer/test.ps1` — PASS. Install WiX in CI/local with `dotnet tool install --global wix` and `wix extension add WixToolset.UI.wixext`.
- [ ] **Step 4:** `release.ps1` builds the MSI after the exe, adds it to the checksums and the `gh release create` assets. `ci.yml`'s windows job installs WiX, builds the MSI with version `0.0.1`, and runs `installer/test.ps1`. README: how to install (just me vs all users, folder choice), the SmartScreen warning, where helper files and settings live, and how to remove them after uninstalling.
- [ ] **Step 5: Manual** on the user's desktop: (a) per-user install into a folder with a space, launch from the Start Menu, confirm the tools are already in `<install>\bin` and no setup screen appears; (b) all-users install into `Program Files` as an administrator, then run the app as a standard user: it starts with no setup screen and no UAC prompt; delete `bin\yt-dlp.exe` as an administrator and confirm the standard user sees the "ask an administrator to run Repair" message, and that Repair restores it; (c) uninstall from Windows Settings and confirm the program, `bin`, the shortcut and `%LOCALAPPDATA%\vid2gif` are gone while a GIF you made earlier is still in your output folder. Commit `feat: msi installer`.

### Task 12: Finish

- [ ] **Step 1:** Run the Impeccable finish steps required by `.impeccable/surfaces/frontend-index-html.md` (FINISH line): detector once over the frontend files, the finish review with desktop (1000), user-width (600) and narrow (480) captures across all three themes, one fix batch, then the documenter to write `DESIGN.md` and `.impeccable/design.json`.
- [ ] **Step 2:** Re-check the spec's "Feel (snappy)" list on the real app (window visible in about a second; no stutter during a run; slip appears at once on paste) and record the measured numbers in the PR or release notes.
- [ ] **Step 3:** Commit `docs: DESIGN.md for the desktop app`.
