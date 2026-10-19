package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/joho/godotenv"
	"vid2gif/internal/clip"
	"vid2gif/internal/deps"
	"vid2gif/internal/encode"
	"vid2gif/internal/naming"
	"vid2gif/internal/progress"
)

const usage = `vid2gif - turn a segment of an online video into a GIF, WebP, MP4 or MOV

Usage:
  vid2gif clip <url> --start T [flags]    the URL must come first
  vid2gif deps install                    download the pinned yt-dlp, ffmpeg and deno into bin/ or $VID2GIF_BIN (run once)
  vid2gif help                            show this text

Example:
  vid2gif deps install
  vid2gif clip "https://www.youtube.com/watch?v=aqz-KE-bpKQ" --start 1:23 --end 1:30 --format gif,mp4 --out ./gifs

Times are SS, MM:SS or HH:MM:SS (decimals allowed on the last part).
`

// newClipFlags defines the clip flags. Its Usage is the full help text
// (overview, example, then every flag with its default); the flag package
// prints it on -h and on flag parse errors, and `vid2gif help` calls it too,
// so the flag list can never drift from the code.
func newClipFlags(a *clipArgs, start, end, formats *string) *flag.FlagSet {
	o := &a.Opts
	fs := flag.NewFlagSet("clip", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(start, "start", "", "start time: SS, MM:SS or HH:MM:SS (required)")
	fs.StringVar(end, "end", "", "end time (default: the end of the video)")
	fs.StringVar(formats, "format", "gif", "comma-separated: gif,webp,mp4,mov")
	fs.StringVar(&o.OutDir, "out", "./gifs", "output directory (created if missing)")
	fs.StringVar(&o.Name, "name", "", "file name without extension (default <title>_<start>-<end>, plus _low or _high for those qualities)")
	fs.StringVar(&o.Quality, "quality", "medium", "low, medium or high")
	fs.BoolVar(&o.Overwrite, "overwrite", false, "replace existing output files (default: skip them)")
	fs.BoolVar(&o.NoPin, "no-pin", false, "use yt-dlp and ffmpeg from PATH without version checks")
	fs.BoolVar(&o.FullSource, "full-source", false, "gif/webp only: download the best video instead of the smallest one at least as wide as the quality (slower; same dimensions)")
	fs.BoolVar(&a.NoProgress, "no-progress", false, "no progress bar (it is shown only when the output is a terminal)")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage, "\nFlags for clip:\n")
		fs.PrintDefaults()
	}
	return fs
}

// clipArgs is a parsed `clip` command: the pipeline's options plus the
// CLI-only ones.
type clipArgs struct {
	clip.Opts
	NoProgress bool
}

// parseClipArgs expects `<url> [flags]`.
func parseClipArgs(args []string) (clipArgs, error) {
	var a clipArgs
	if len(args) == 0 {
		return a, errors.New("missing <url>")
	}
	o := &a.Opts
	var start, end, formats string
	fs := newClipFlags(&a, &start, &end, &formats)
	rest := args
	if !strings.HasPrefix(args[0], "-") {
		o.URL, rest = args[0], args[1:]
	}
	if err := fs.Parse(rest); err != nil {
		return a, err
	}
	if o.URL == "" {
		return a, errors.New("the URL must come first: vid2gif clip <url> --start T")
	}
	if fs.NArg() > 0 {
		return a, errors.New("unexpected argument " + fs.Arg(0))
	}
	if start == "" {
		return a, errors.New("--start is required")
	}
	var err error
	if o.Start, err = naming.ParseTimestamp(start); err != nil {
		return a, errors.New("--start: " + err.Error())
	}
	if end != "" { // no --end: o.End stays 0, meaning until the end of the video
		if o.End, err = naming.ParseTimestamp(end); err != nil {
			return a, errors.New("--end: " + err.Error())
		}
		if o.End <= o.Start {
			return a, errors.New("--end must be after --start")
		}
	}
	if o.Formats, err = naming.ParseFormats(formats); err != nil {
		return a, errors.New("--format: " + err.Error())
	}
	if _, ok := encode.Presets[o.Quality]; !ok {
		return a, fmt.Errorf("unknown --quality %q (choose low, medium or high)", o.Quality)
	}
	if strings.ContainsAny(o.Name, `/\`) {
		return a, errors.New("--name must not contain path separators; use --out for the directory")
	}
	return a, nil
}

// baseDir is where deps.json and bin/ live: next to the executable, or the
// current directory when the executable is elsewhere (e.g. `go run`).
func baseDir() string {
	if exe, err := os.Executable(); err == nil {
		d := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(d, "deps.json")); err == nil {
			return d
		}
	}
	wd, _ := os.Getwd()
	return wd
}

// loadEnv reads an optional .env file next to deps.json. Variables already
// set in the real environment win, and a missing file is fine.
func loadEnv(base string) {
	_ = godotenv.Load(filepath.Join(base, ".env"))
}

func runDepsInstall(base string, stdout, stderr io.Writer) int {
	d, err := deps.Load(filepath.Join(base, "deps.json"))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if err := deps.InstallAll(d, deps.BinDir(base, os.Getenv), stdout); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

func runClipCmd(args []string, stdout, stderr io.Writer) int {
	o, err := parseClipArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		return 0 // the flag package already printed the flag list
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		fmt.Fprintln(stderr, "run `vid2gif clip -h` for usage and an example")
		return 2
	}
	base := baseDir()
	d := deps.Pins{}
	if !o.NoPin {
		if d, err = deps.Load(filepath.Join(base, "deps.json")); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
	}
	r := deps.NewResolver(deps.BinDir(base, os.Getenv))
	// YouTube extraction needs deno, so it is required unless --no-pin, where
	// a missing deno just leaves yt-dlp on its own defaults.
	var paths [3]string
	for i, name := range []string{"yt-dlp", "ffmpeg", "deno"} {
		p, err := r.Resolve(name, d[name], o.NoPin)
		if err != nil && (!o.NoPin || name != "deno") {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		paths[i] = p // empty for a missing deno under --no-pin
	}
	ytdlp, ffmpeg, deno := paths[0], paths[1], paths[2]
	hint := ""
	if !o.NoPin {
		hint = fmt.Sprintf("yt-dlp is pinned at %s. If this is a site-extraction error, bump the pin in deps.json or retry with --no-pin.", d["yt-dlp"].Version)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return makeClip(ctx, o, stdout, stderr, func(out, toolOut io.Writer) clipRunner {
		return clip.New(ytdlp, ffmpeg, deno, hint, out).WithToolOutput(toolOut)
	})
}

type clipRunner interface {
	Run(ctx context.Context, o clip.Opts) error
}

// isTerm decides whether stdout is a terminal; tests replace it.
var isTerm = isTerminal

// wantBar: the progress bar is on by default, but only on a terminal.
func wantBar(a clipArgs, stdout io.Writer) bool {
	return !a.NoProgress && isTerm(stdout)
}

// makeClip runs one clip; newRunner builds the pipeline writing its lines to
// out and yt-dlp's own output to toolOut (nil = the terminal). Without the bar
// the run is exactly the plain one. With it, the bar shows the app's progress
// (same tracker, labels and honesty rules), and yt-dlp's output is held back
// and printed only if the run fails (not on Ctrl+C).
func makeClip(ctx context.Context, a clipArgs, stdout, stderr io.Writer, newRunner func(out, toolOut io.Writer) clipRunner) int {
	if !wantBar(a, stdout) {
		if err := newRunner(stdout, nil).Run(ctx, a.Opts); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
		return 0
	}
	o := a.Opts
	if o.End > 0 {
		// ponytail: without --end the length is unknown and the bar is indeterminate; the title lookup could also fetch the duration.
		o.DurationSec = o.End - o.Start
	}
	tr := progress.New(o.Quality, o.Formats, o.FullSource, o.DurationSec)
	b := newBar(stdout, blocksOK(runtime.GOOS, os.Getenv))
	o.OnEvent = func(e clip.Event) { // serialized by clip, never after Run returns
		if stage, stagePct, overall, ok := tr.Apply(e); ok {
			b.update(progress.Label(stage, stagePct), overall, stagePct >= 0)
		}
	}
	tool := &tail{max: 64 << 10}
	b.start(tickEvery)
	err := newRunner(b, tool).Run(ctx, o)
	b.stop()
	if err != nil {
		if ctx.Err() == nil {
			stderr.Write(tool.bytes())
		}
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

func realMain(args []string, stdout, stderr io.Writer) int {
	loadEnv(baseDir())
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "clip":
		return runClipCmd(args[1:], stdout, stderr)
	case "deps":
		if len(args) == 2 && args[1] == "install" {
			return runDepsInstall(baseDir(), stdout, stderr)
		}
		fmt.Fprint(stderr, usage)
		return 2
	case "help", "-h", "--help":
		fs := newClipFlags(&clipArgs{}, new(string), new(string), new(string))
		fs.SetOutput(stdout)
		fs.Usage()
		return 0
	}
	fmt.Fprintf(stderr, "error: unknown command %q\n\n%s", args[0], usage)
	return 2
}

func main() { os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr)) }
