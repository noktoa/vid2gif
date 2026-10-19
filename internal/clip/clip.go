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
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"vid2gif/internal/debuglog"
	"vid2gif/internal/encode"
	"vid2gif/internal/naming"
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

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func (p Pipeline) run(ctx context.Context, o Opts) error {
	emit := func(e Event) {
		p.log.Printf("event: stage=%s pct=%.2f path=%s size=%d err=%v", e.Stage, e.Pct, e.Path, e.Size, e.Err)
		if o.OnEvent != nil {
			o.OnEvent(e)
		}
	}
	// tick reports progress inside a stage; not logged, it would flood the file.
	tick := func(stage, path string) func(float64) {
		return func(f float64) {
			if o.OnEvent != nil {
				o.OnEvent(Event{Stage: stage, Pct: min(max(f, 0), 1), Path: path})
			}
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
			return fail("", fmt.Errorf("fetching title: %w", err))
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
	dlPct := 0.0
	if o.DurationSec <= 0 {
		dlPct = -1
	}
	emit(Event{Stage: StageDownload, Pct: dlPct})
	clip, err := p.download(ctx, o, tmp, wantAudio, tick(StageDownload, ""))
	if err != nil {
		return fail("", err)
	}

	wrote, failed := 0, 0
	for _, f := range todo {
		if err := ctx.Err(); err != nil {
			return fail("", err)
		}
		final := naming.OutPath(o.OutDir, name, f)
		part := filepath.Join(tmp, name+"."+f)
		eo := encode.Opts{Format: f, Preset: encode.Presets[o.Quality]}
		p.log.Printf("run: %s %s", orDefault(p.ffmpeg, "ffmpeg"), strings.Join(encode.Args(clip, part, eo), " "))
		emit(Event{Stage: StageEncode, Pct: 0, Path: final})
		if err := p.encode(ctx, clip, part, eo, tick(StageEncode, final)); err != nil {
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
	if wantAudio {
		format = "bv*+ba/b"
	}
	end := "inf" // End 0 = until the end of the video
	if o.End > 0 {
		end = naming.FmtSeconds(o.End)
	}
	args := append([]string{"--ignore-config"}, JSRuntimeArgs(deno)...)
	return append(args,
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

// lineWriter logs each line written to it as "<prefix>: line", splitting on
// \n or \r (yt-dlp redraws progress with \r). Close flushes a trailing line.
type lineWriter struct {
	log    *debuglog.Log
	prefix string
	buf    []byte
}

func (w *lineWriter) Write(b []byte) (int, error) {
	w.buf = append(w.buf, b...)
	for {
		i := bytes.IndexAny(w.buf, "\r\n")
		if i < 0 {
			return len(b), nil
		}
		w.flush(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
}

func (w *lineWriter) flush(line []byte) {
	if s := strings.TrimSpace(string(line)); s != "" {
		w.log.Printf("%s: %s", w.prefix, s)
	}
}

func (w *lineWriter) Close() { w.flush(w.buf); w.buf = nil }

// teeStderr returns where a tool's stderr should go: next alone when ctx has
// no log, else next plus a line logger. The returned func flushes the logger.
func teeStderr(ctx context.Context, prefix string, next io.Writer) (io.Writer, func()) {
	l, _ := ctx.Value(logKey{}).(*debuglog.Log)
	if l == nil {
		return next, func() {}
	}
	lw := &lineWriter{log: l, prefix: prefix}
	return io.MultiWriter(next, lw), lw.Close
}

// runTool runs cmd and, on cancel, kills its whole process tree (yt-dlp
// spawns ffmpeg) so nothing outlives the temp-dir cleanup.
// ponytail: tree kill is Windows-only (taskkill /T); on Unix only the direct child dies, add a process group if orphans show up.
func runTool(cmd *exec.Cmd) error {
	cmd.Cancel = func() error {
		if runtime.GOOS == "windows" && exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run() == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = 5 * time.Second // do not hang on a pipe an orphan still holds
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
			cmd.Stderr, done = teeStderr(ctx, "yt-dlp", os.Stderr)
			err := runTool(cmd)
			done()
			if err != nil {
				return "", withHint(fmt.Errorf("yt-dlp: %w", err), hint)
			}
			title, _, _ := strings.Cut(strings.TrimSpace(stdout.String()), "\n")
			return strings.TrimSpace(title), nil
		},
		download: func(ctx context.Context, o Opts, tmpDir string, wantAudio bool, _ func(float64)) (string, error) {
			cmd := exec.CommandContext(ctx, ytdlp, ytdlpDownloadArgs(o, ffmpeg, deno, tmpDir, wantAudio)...)
			var done func()
			cmd.Stdout = os.Stdout
			cmd.Stderr, done = teeStderr(ctx, "yt-dlp", os.Stderr)
			err := runTool(cmd)
			done()
			if err != nil {
				return "", withHint(fmt.Errorf("yt-dlp: %w", err), hint)
			}
			return pickClipFile(tmpDir)
		},
		encode: func(ctx context.Context, in, out string, eo encode.Opts, _ func(float64)) error {
			cmd := exec.CommandContext(ctx, ffmpeg, encode.Args(in, out, eo)...)
			var buf bytes.Buffer // ffmpeg's own output goes into the error, as encode.Runner does
			var done func()
			cmd.Stderr, done = teeStderr(ctx, "ffmpeg", &buf)
			err := runTool(cmd)
			done()
			if err != nil {
				return fmt.Errorf("ffmpeg: %w\n%s", err, strings.TrimSpace(buf.String()))
			}
			return nil
		},
		out:    out,
		ytdlp:  ytdlp,
		ffmpeg: ffmpeg,
		deno:   deno,
	}
}
