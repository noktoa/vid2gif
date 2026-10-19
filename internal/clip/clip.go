// Package clip downloads a segment with yt-dlp and encodes it to the requested formats.
package clip

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"vid2gif/internal/debuglog"
	"vid2gif/internal/encode"
	"vid2gif/internal/naming"
	"vid2gif/internal/proc"
)

// Pipeline stages reported through Opts.OnEvent.
const (
	StageFind     = "find"
	StageDownload = "download"
	StageEncode   = "encode"
	StageWrote    = "wrote"
	StageSkipped  = "skipped"
	StageFailed   = "failed"
)

// Event is one step of a Run. Pct is 0..1 within the stage, or -1 when
// unknown. For StageEncode and StageFailed (when an output is involved), Path
// is the final output path; StageWrote carries Size.
type Event struct {
	Stage string
	Pct   float64
	Path  string
	Size  int64
	Err   error
}

// Opts is one clip request, already validated by the CLI.
type Opts struct {
	URL        string
	Start, End float64 // End 0 = until the end of the video
	Formats    []string
	OutDir     string
	Name       string // empty = derive from the video title
	Quality    string
	Overwrite  bool
	NoPin      bool
	FullSource bool // gif/webp only: download the best video, not the smallest one wide enough

	OnEvent     func(Event) // nil = ignore
	DurationSec float64     // length of the clip if known; 0 = unknown
}

// Pipeline holds the side-effecting steps as fields so run() is testable
// with fakes; New wires in the real yt-dlp and ffmpeg. progress reports 0..1.
type Pipeline struct {
	title    func(ctx context.Context, url string) (string, error)
	download func(ctx context.Context, o Opts, tmpDir string, wantAudio bool, progress func(frac float64)) (string, error)
	encode   func(ctx context.Context, in, out string, eo encode.Opts, progress func(frac float64)) error
	out      io.Writer

	// used only to write command lines to the debug log
	ytdlp, ffmpeg, deno string
	log                 *debuglog.Log
}

type logKey struct{}

// WithLog returns a pipeline that records commands, tool stderr, events and
// the final error in l. A nil l changes nothing; a disabled l writes nothing.
func (p Pipeline) WithLog(l *debuglog.Log) Pipeline {
	p.log = l
	return p
}

func (p Pipeline) Run(ctx context.Context, o Opts) error {
	ctx = context.WithValue(ctx, logKey{}, p.log) // read by the real tool runners
	err := p.run(ctx, o)
	if err != nil {
		p.log.Printf("error: %v", err)
	}
	return err
}

// cancelErr reports a failure after the context was cancelled as the
// cancellation itself: the tool was killed, so its exit status (and any
// "update yt-dlp" hint) says nothing useful.
func cancelErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// taskkillPath prefers the system copy so a taskkill.exe planted on PATH is not run.
func taskkillPath() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		return filepath.Join(root, "System32", "taskkill.exe")
	}
	return "taskkill"
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func (p Pipeline) run(ctx context.Context, o Opts) error {
	// Tool output goroutines report progress while this one emits, so OnEvent
	// calls go through one mutex, and none happen once run has returned.
	var mu sync.Mutex
	closed := false
	defer func() { mu.Lock(); closed = true; mu.Unlock() }()
	send := func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		if o.OnEvent != nil && !closed {
			o.OnEvent(e)
		}
	}
	emit := func(e Event) {
		p.log.Printf("event: stage=%s pct=%.2f path=%s size=%d err=%v", e.Stage, e.Pct, e.Path, e.Size, e.Err)
		send(e)
	}
	// tick reports progress inside a stage; only its first report is logged,
	// the rest would flood the file. A negative f is "unknown" and stays -1;
	// known values never go down.
	tick := func(stage, path string) func(float64) {
		last, first := -1.0, true
		return func(f float64) {
			if f >= 0 {
				last = max(last, min(f, 1))
			}
			e := Event{Stage: stage, Pct: last, Path: path}
			if first {
				first = false
				emit(e)
				return
			}
			send(e)
		}
	}
	fail := func(path string, err error) error {
		emit(Event{Stage: StageFailed, Pct: -1, Path: path, Err: err})
		return err
	}

	emit(Event{Stage: StageFind, Pct: -1})
	name := o.Name
	if name == "" {
		p.log.Printf("run: %s %s", orDefault(p.ytdlp, "yt-dlp"), strings.Join(ytdlpTitleArgs(o.URL, p.deno), " "))
		title, err := p.title(ctx, o.URL)
		if err != nil {
			return fail("", cancelErr(ctx, fmt.Errorf("fetching title: %w", err)))
		}
		name = naming.BuildName(title, o.Start, o.End, o.Quality)
	}
	todo, skipped := naming.PlanOutputs(o.OutDir, name, o.Formats, o.Overwrite)
	for _, f := range skipped {
		path := naming.OutPath(o.OutDir, name, f)
		fmt.Fprintf(p.out, "skipped %s (already exists; use --overwrite to replace)\n", path)
		emit(Event{Stage: StageSkipped, Pct: 1, Path: path})
	}
	if len(todo) == 0 {
		fmt.Fprintf(p.out, "wrote 0, skipped %d\n", len(skipped))
		return nil
	}
	if err := os.MkdirAll(o.OutDir, 0o755); err != nil {
		return fail("", err)
	}
	tmp, err := os.MkdirTemp(o.OutDir, ".vid2gif-tmp-")
	if err != nil {
		return fail("", err)
	}
	// Runs on every return, cancel included. The tool runners wait for their
	// child to exit before returning, so nothing still holds files in tmp.
	defer os.RemoveAll(tmp)

	wantAudio := slices.ContainsFunc(todo, func(f string) bool { return f == "mp4" || f == "mov" })
	p.log.Printf("run: %s %s", orDefault(p.ytdlp, "yt-dlp"), strings.Join(ytdlpDownloadArgs(o, p.ffmpeg, p.deno, tmp, wantAudio), " "))
	// No StageDownload event yet: the stage starts at the first progress report.
	clip, err := p.download(ctx, o, tmp, wantAudio, tick(StageDownload, ""))
	if err != nil {
		return fail("", cancelErr(ctx, err))
	}

	wrote, failed := 0, 0
	for _, f := range todo {
		if err := ctx.Err(); err != nil {
			return fail("", err)
		}
		final := naming.OutPath(o.OutDir, name, f)
		part := filepath.Join(tmp, name+"."+f)
		eo := encode.Opts{Format: f, Preset: encode.Presets[o.Quality], DurationSec: o.DurationSec}
		// Only mp4/mov encodes can be measured; gif and webp stay indeterminate.
		var progress func(float64)
		startPct := -1.0
		if f == "mp4" || f == "mov" {
			progress = tick(StageEncode, final)
			eo.OnProgress = progress
			if o.DurationSec > 0 {
				startPct = 0
			}
		}
		p.log.Printf("run: %s %s", orDefault(p.ffmpeg, "ffmpeg"), strings.Join(encode.Args(clip, part, eo), " "))
		emit(Event{Stage: StageEncode, Pct: startPct, Path: final})
		if err := p.encode(ctx, clip, part, eo, progress); err != nil {
			err = cancelErr(ctx, err)
			fmt.Fprintf(p.out, "failed %s: %v\n", final, err)
			emit(Event{Stage: StageFailed, Pct: -1, Path: final, Err: err})
			if ctx.Err() != nil { // cancelled, not a bad format: stop
				return err
			}
			failed++
			continue
		}
		if err := os.Rename(part, final); err != nil {
			fmt.Fprintf(p.out, "failed %s: %v\n", final, err)
			emit(Event{Stage: StageFailed, Pct: -1, Path: final, Err: err})
			failed++
			continue
		}
		size := int64(0)
		if st, err := os.Stat(final); err == nil {
			size = st.Size()
		}
		fmt.Fprintf(p.out, "wrote %s (%d KB)\n", final, size/1024)
		emit(Event{Stage: StageWrote, Pct: 1, Path: final, Size: size})
		wrote++
	}
	fmt.Fprintf(p.out, "wrote %d, skipped %d\n", wrote, len(skipped))
	if failed > 0 {
		return fmt.Errorf("%d of %d outputs failed", failed, len(todo))
	}
	return nil
}

// ytdlpTitleArgs asks yt-dlp for the video title only (no download).
// --ignore-config keeps a user's own yt-dlp config from changing a pinned run.
func ytdlpTitleArgs(url, deno string) []string {
	return append(append([]string{"--ignore-config"}, JSRuntimeArgs(deno)...), "--print", "title", "--no-playlist", "--", url)
}

// JSRuntimeArgs restricts yt-dlp to the pinned deno, which YouTube extraction
// needs. --no-js-runtimes must come first because it clears earlier entries.
// With no deno (--no-pin and none installed) yt-dlp keeps its own defaults.
func JSRuntimeArgs(deno string) []string {
	if deno == "" {
		return nil
	}
	return []string{"--no-js-runtimes", "--js-runtimes", "deno:" + deno}
}

// ytdlpDownloadArgs downloads just [Start, End], or [Start, end of video] when End is 0. --ffmpeg-location is
// mandatory so yt-dlp uses the pinned ffmpeg, and the URL follows `--` so a
// value starting with "-" is never read as an option.
func ytdlpDownloadArgs(o Opts, ffmpegPath, deno, tmpDir string, wantAudio bool) []string {
	format := "bv*/b"
	var sort []string
	if wantAudio {
		format = "bv*+ba/b"
	} else if w := encode.PresetWidth(o.Quality); w > 0 && !o.FullSource {
		// gif/webp are scaled down to w, so fetch the smallest video at least
		// w wide. With -S +width, bv*[width>=w] is the narrowest such format
		// and wv*[width<w] (when nothing is wide enough) is the widest of the
		// narrower ones. A filter drops formats whose width is unknown, so a
		// source that reports no width at all matches neither; the trailing
		// bv*/b then take the best format as before (all widths tie, so the
		// default quality order decides), instead of the narrowest. No height
		// cap: a portrait video's width is its short side.
		format = fmt.Sprintf("bv*[width>=%d]/wv*[width<%[1]d]/bv*/b", w)
		sort = []string{"-S", "+width"}
	}
	end := "inf" // End 0 = until the end of the video
	if o.End > 0 {
		end = naming.FmtSeconds(o.End)
	}
	args := append([]string{"--ignore-config"}, JSRuntimeArgs(deno)...)
	return append(append(args, sort...),
		"--no-playlist",
		"-f", format,
		"--download-sections", fmt.Sprintf("*%s-%s", naming.FmtSeconds(o.Start), end),
		"--force-keyframes-at-cuts",
		"--merge-output-format", "mkv",
		"--ffmpeg-location", ffmpegPath,
		"-o", filepath.Join(tmpDir, "clip.%(ext)s"),
		"--", o.URL)
}

// pickClipFile finds yt-dlp's output (clip.<ext>), ignoring partial files.
func pickClipFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, "clip.") && !strings.HasSuffix(n, ".part") && !strings.HasSuffix(n, ".ytdl") {
			return filepath.Join(dir, n), nil
		}
	}
	return "", fmt.Errorf("yt-dlp produced no clip file in %s", dir)
}

func withHint(err error, hint string) error {
	if hint == "" {
		return err
	}
	return fmt.Errorf("%w\n%s", err, hint)
}

// SplitProgress is a bufio.SplitFunc that splits on \r or \n: ffmpeg redraws
// its stats line with \r, so a plain line scanner would see one endless line.
func SplitProgress(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

var ffmpegTime = regexp.MustCompile(`\btime=(\d+):(\d\d):(\d\d(?:\.\d+)?)`)

// ParseFFmpegTime reads the time=HH:MM:SS.xx of an ffmpeg stats line.
// time=N/A and lines without it are not ok.
func ParseFFmpegTime(line string) (seconds float64, ok bool) {
	m := ffmpegTime.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	h, _ := strconv.Atoi(m[1])
	mn, _ := strconv.Atoi(m[2])
	sec, _ := strconv.ParseFloat(m[3], 64)
	return float64(h*3600+mn*60) + sec, true
}

// downloadProgress turns yt-dlp's stderr lines into download fractions for
// progress: seconds of the clip over durationSec, or -1 when that is unknown.
func downloadProgress(durationSec float64, progress func(frac float64)) func(line string) {
	return func(line string) {
		sec, ok := ParseFFmpegTime(line)
		if !ok {
			return
		}
		if durationSec <= 0 {
			progress(-1)
			return
		}
		progress(min(sec/durationSec, 1))
	}
}

// lineWriter logs each line written to it as "<prefix>: line" and/or hands it
// to each, splitting like SplitProgress. Close flushes a trailing line.
type lineWriter struct {
	log    *debuglog.Log
	prefix string
	each   func(line string) // nil = log only
	buf    []byte
}

func (w *lineWriter) Write(b []byte) (int, error) {
	w.buf = append(w.buf, b...)
	for {
		adv, tok, _ := SplitProgress(w.buf, false)
		if adv == 0 {
			return len(b), nil
		}
		w.flush(tok)
		w.buf = w.buf[adv:]
	}
}

func (w *lineWriter) flush(line []byte) {
	if s := strings.TrimSpace(string(line)); s != "" {
		w.log.Printf("%s: %s", w.prefix, s)
		if w.each != nil {
			w.each(s)
		}
	}
}

func (w *lineWriter) Close() { w.flush(w.buf); w.buf = nil }

// teeStderr returns where a tool's stderr should go: next alone when ctx has
// no log and each is nil, else next plus a line logger / each(line) callback.
// next always gets the raw bytes. The returned func flushes the last line.
func teeStderr(ctx context.Context, prefix string, next io.Writer, each func(line string)) (io.Writer, func()) {
	l, _ := ctx.Value(logKey{}).(*debuglog.Log)
	if l == nil && each == nil {
		return next, func() {}
	}
	lw := &lineWriter{log: l, prefix: prefix, each: each}
	return io.MultiWriter(next, lw), lw.Close
}

// runTool runs cmd and, on cancel, kills its whole process tree (yt-dlp
// spawns ffmpeg) so nothing outlives the temp-dir cleanup.
// ponytail: tree kill is Windows-only (taskkill /T); on Unix only the direct child dies, add a process group if orphans show up.
func runTool(cmd *exec.Cmd) error {
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" {
			kill := exec.Command(taskkillPath(), "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
			proc.Hide(kill)
			if kill.Run() == nil {
				return nil
			}
		}
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = 5 * time.Second // do not hang on a pipe an orphan still holds
	proc.Hide(cmd)
	return cmd.Run()
}

// New wires in the real tools. hint is appended to yt-dlp failures
// (e.g. how to bump the pin).
func New(ytdlp, ffmpeg, deno, hint string, out io.Writer) Pipeline {
	return Pipeline{
		title: func(ctx context.Context, url string) (string, error) {
			cmd := exec.CommandContext(ctx, ytdlp, ytdlpTitleArgs(url, deno)...)
			var stdout bytes.Buffer
			var done func()
			cmd.Stdout = &stdout
			cmd.Stderr, done = teeStderr(ctx, "yt-dlp", os.Stderr, nil)
			err := runTool(cmd)
			done()
			if err != nil {
				return "", cancelErr(ctx, withHint(fmt.Errorf("yt-dlp: %w", err), hint))
			}
			title, _, _ := strings.Cut(strings.TrimSpace(stdout.String()), "\n")
			return strings.TrimSpace(title), nil
		},
		download: func(ctx context.Context, o Opts, tmpDir string, wantAudio bool, progress func(float64)) (string, error) {
			cmd := exec.CommandContext(ctx, ytdlp, ytdlpDownloadArgs(o, ffmpeg, deno, tmpDir, wantAudio)...)
			var done func()
			var each func(string)
			if progress != nil {
				each = downloadProgress(o.DurationSec, progress)
			}
			cmd.Stdout = os.Stdout
			cmd.Stderr, done = teeStderr(ctx, "yt-dlp", os.Stderr, each)
			err := runTool(cmd)
			done()
			if err != nil {
				return "", cancelErr(ctx, withHint(fmt.Errorf("yt-dlp: %w", err), hint))
			}
			return pickClipFile(tmpDir)
		},
		encode: func(ctx context.Context, in, out string, eo encode.Opts, progress func(float64)) error {
			if progress != nil {
				eo.OnProgress = progress
			}
			cmd := exec.CommandContext(ctx, ffmpeg, encode.Args(in, out, eo)...)
			if progress != nil {
				cmd.Stdout = encode.NewProgressWriter(eo.DurationSec, progress) // -progress pipe:1
			}
			var buf bytes.Buffer // ffmpeg's own output goes into the error, as encode.Runner does
			var done func()
			cmd.Stderr, done = teeStderr(ctx, "ffmpeg", &buf, nil)
			err := runTool(cmd)
			done()
			if err != nil {
				return cancelErr(ctx, fmt.Errorf("ffmpeg: %w\n%s", err, strings.TrimSpace(buf.String())))
			}
			return nil
		},
		out:    out,
		ytdlp:  ytdlp,
		ffmpeg: ffmpeg,
		deno:   deno,
	}
}
