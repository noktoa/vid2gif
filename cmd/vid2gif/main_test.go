package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"vid2gif/internal/clip"
)

func TestParseClipArgsDefaults(t *testing.T) {
	o, err := parseClipArgs([]string{"https://x/y", "--start", "1:23"})
	if err != nil {
		t.Fatal(err)
	}
	if o.URL != "https://x/y" || o.Start != 83 || o.End != 0 || !slices.Equal(o.Formats, []string{"gif"}) ||
		o.OutDir != "./gifs" || o.Quality != "medium" || o.Name != "" || o.Overwrite || o.NoPin || o.FullSource {
		t.Errorf("unexpected defaults: %+v", o)
	}
}

func TestParseClipArgsAllFlagsAfterURL(t *testing.T) {
	o, err := parseClipArgs([]string{"https://x/y", "--start", "10", "--end", "1:00", "--format", "gif,mp4",
		"--out", "D", "--name", "demo", "--quality", "high", "--overwrite", "--no-pin", "--full-source"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Start != 10 || o.End != 60 || !slices.Equal(o.Formats, []string{"gif", "mp4"}) || o.OutDir != "D" ||
		o.Name != "demo" || o.Quality != "high" || !o.Overwrite || !o.NoPin || !o.FullSource {
		t.Errorf("unexpected opts: %+v", o)
	}
}

func TestParseClipArgsFullSource(t *testing.T) {
	o, err := parseClipArgs([]string{"https://x/y", "--start", "1", "--full-source"})
	if err != nil || !o.FullSource {
		t.Errorf("FullSource = %v, err = %v; want true", o.FullSource, err)
	}
}

func TestParseClipArgsNoProgress(t *testing.T) {
	a, err := parseClipArgs([]string{"https://x/y", "--start", "1"})
	if err != nil || a.NoProgress {
		t.Errorf("default NoProgress = %v, err = %v; want false", a.NoProgress, err)
	}
	a, err = parseClipArgs([]string{"https://x/y", "--start", "1", "--no-progress"})
	if err != nil || !a.NoProgress {
		t.Errorf("NoProgress = %v, err = %v; want true", a.NoProgress, err)
	}
}

// fakeClip stands in for clip.Pipeline: it records how it was set up and
// writes what a real run would.
type fakeClip struct {
	out, toolOut io.Writer
	opts         clip.Opts
	run          func(f *fakeClip, ctx context.Context, o clip.Opts) error
}

func (f *fakeClip) Run(ctx context.Context, o clip.Opts) error {
	f.opts = o
	return f.run(f, ctx, o)
}

func newFake(run func(f *fakeClip, ctx context.Context, o clip.Opts) error) (*fakeClip, func(out, toolOut io.Writer) clipRunner) {
	f := &fakeClip{run: run}
	return f, func(out, toolOut io.Writer) clipRunner {
		f.out, f.toolOut = out, toolOut
		return f
	}
}

// plainRun writes the lines a real two-format run prints, then fails.
func plainRun(f *fakeClip, _ context.Context, o clip.Opts) error {
	if o.OnEvent != nil {
		o.OnEvent(clip.Event{Stage: clip.StageFind, Pct: -1})
		o.OnEvent(clip.Event{Stage: clip.StageEncode, Pct: -1, Path: "g/x.gif"})
	}
	if f.toolOut != nil {
		f.toolOut.Write([]byte("ERROR: yt-dlp says why\n"))
	}
	io.WriteString(f.out, "wrote g/x.gif (3 KB)\nfailed g/x.mp4: ffmpeg: exit status 1\nwrote 1, skipped 0\n")
	return errors.New("1 of 2 outputs failed")
}

// fakeTerminal makes stdout count as a terminal (or not) and stops the
// ticker from drawing on its own, so tests draw with bar.tick().
func fakeTerminal(t *testing.T, is bool) {
	t.Helper()
	oldTerm, oldTick := isTerm, tickEvery
	isTerm = func(io.Writer) bool { return is }
	tickEvery = time.Hour
	t.Cleanup(func() { isTerm, tickEvery = oldTerm, oldTick })
}

// tickBar draws the bar the way the ticker would; out is the bar while it is on.
func tickBar(t *testing.T, out io.Writer) {
	t.Helper()
	b, ok := out.(*bar)
	if !ok {
		t.Fatalf("pipeline out is %T, want the bar", out)
	}
	b.tick()
}

// Off a terminal, or with --no-progress even on one, the run is exactly the
// plain one: the pipeline writes straight to stdout, yt-dlp keeps the
// terminal (no capture writer), no events, no clip length, nothing added.
func TestPlainOutputUnchanged(t *testing.T) {
	for _, c := range []struct {
		terminal bool
		args     []string
	}{
		{false, []string{"https://x", "--start", "1", "--end", "9", "--format", "gif,mp4"}},
		{false, []string{"https://x", "--start", "1", "--end", "9", "--format", "gif,mp4", "--no-progress"}},
		{true, []string{"https://x", "--start", "1", "--end", "9", "--format", "gif,mp4", "--no-progress"}},
	} {
		fakeTerminal(t, c.terminal)
		a, err := parseClipArgs(c.args)
		if err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		f, newRunner := newFake(plainRun)
		if code := makeClip(context.Background(), a, &out, &errb, newRunner); code != 1 {
			t.Errorf("code = %d, want 1", code)
		}
		if f.out != &out || f.toolOut != nil || f.opts.OnEvent != nil || f.opts.DurationSec != 0 {
			t.Errorf("%v: plain run was changed: out=%T toolOut=%v opts=%+v", c, f.out, f.toolOut, f.opts)
		}
		if got := out.String(); got != "wrote g/x.gif (3 KB)\nfailed g/x.mp4: ffmpeg: exit status 1\nwrote 1, skipped 0\n" {
			t.Errorf("stdout = %q", got)
		}
		if got := errb.String(); got != "error: 1 of 2 outputs failed\n" {
			t.Errorf("stderr = %q", got)
		}
	}
}

func TestWantBar(t *testing.T) {
	plain, _ := parseClipArgs([]string{"https://x", "--start", "1"})
	off, _ := parseClipArgs([]string{"https://x", "--start", "1", "--no-progress"})
	for _, c := range []struct {
		terminal bool
		a        clipArgs
		want     bool
	}{
		{true, plain, true},
		{true, off, false}, // --no-progress wins on a terminal
		{false, plain, false},
		{false, off, false},
	} {
		fakeTerminal(t, c.terminal)
		if got := wantBar(c.a, os.Stdout); got != c.want {
			t.Errorf("terminal=%v NoProgress=%v: wantBar = %v, want %v", c.terminal, c.a.NoProgress, got, c.want)
		}
	}
}

// With the bar: same lines on stdout (with the bar erased around them),
// yt-dlp's output held back and shown on stderr only because the run failed,
// and no bar left on screen.
func TestBarRunKeepsLinesAndShowsToolOutputOnFailure(t *testing.T) {
	fakeTerminal(t, true)
	a, _ := parseClipArgs([]string{"https://x", "--start", "1", "--end", "9", "--format", "gif,mp4"})
	var out, errb bytes.Buffer
	f, newRunner := newFake(func(f *fakeClip, ctx context.Context, o clip.Opts) error {
		o.OnEvent(clip.Event{Stage: clip.StageFind, Pct: -1})
		tickBar(t, f.out)
		return plainRun(f, ctx, o)
	})
	if code := makeClip(context.Background(), a, &out, &errb, newRunner); code != 1 {
		t.Errorf("code = %d, want 1", code)
	}
	if f.opts.DurationSec != 8 || f.toolOut == nil {
		t.Errorf("DurationSec = %v, toolOut = %v; want 8 and a capture", f.opts.DurationSec, f.toolOut)
	}
	got := out.String()
	if !strings.HasPrefix(got, "\rFinding the video") {
		t.Errorf("bar not drawn first: %q", got)
	}
	// what stays on screen: per line, the last \r-drawn text; the last line
	// (where the bar was) must be blank
	if s := screen(got); s != "wrote g/x.gif (3 KB)\nfailed g/x.mp4: ffmpeg: exit status 1\nwrote 1, skipped 0\n" {
		t.Errorf("screen = %q (raw %q)", s, got)
	}
	if errb.String() != "ERROR: yt-dlp says why\nerror: 1 of 2 outputs failed\n" {
		t.Errorf("stderr = %q", errb.String())
	}
}

// screen models a terminal for \r-only output: each line shows its last
// \r-separated part, trailing spaces trimmed.
func screen(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		parts := strings.Split(l, "\r")
		lines[i] = strings.TrimRight(parts[len(parts)-1], " ")
	}
	return strings.Join(lines, "\n")
}

// A cancel (Ctrl+C) is not a failure to explain: the held-back tool output
// stays hidden, and the bar is gone.
func TestBarRunCancelHidesToolOutput(t *testing.T) {
	fakeTerminal(t, true)
	a, _ := parseClipArgs([]string{"https://x", "--start", "1"})
	ctx, cancel := context.WithCancel(context.Background())
	var out, errb bytes.Buffer
	f, newRunner := newFake(func(f *fakeClip, ctx context.Context, o clip.Opts) error {
		o.OnEvent(clip.Event{Stage: clip.StageDownload, Pct: -1})
		tickBar(t, f.out)
		f.toolOut.Write([]byte("KeyboardInterrupt\n"))
		cancel()
		return ctx.Err()
	})
	makeClip(ctx, a, &out, &errb, newRunner)
	if f.opts.DurationSec != 0 {
		t.Errorf("DurationSec = %v without --end, want 0 (unknown)", f.opts.DurationSec)
	}
	if errb.String() != "error: context canceled\n" {
		t.Errorf("stderr = %q", errb.String())
	}
	if got := out.String(); !regexp.MustCompile(`^\rGetting the clip(…|\.\.\.)[^%\r]*\r +\r$`).MatchString(got) {
		t.Errorf("stdout = %q; want one indeterminate frame then an erase", got)
	}
}

func TestParseClipArgsUsageErrors(t *testing.T) {
	cases := map[string][]string{
		"no args":             {},
		"url not first":       {"--start", "1", "https://x"},
		"missing start":       {"https://x"},
		"bad start":           {"https://x", "--start", "abc"},
		"bad end":             {"https://x", "--start", "1", "--end", "x"},
		"end before start":    {"https://x", "--start", "10", "--end", "5"},
		"end equals start":    {"https://x", "--start", "10", "--end", "10"},
		"bad format":          {"https://x", "--start", "1", "--format", "avi"},
		"bad quality":         {"https://x", "--start", "1", "--quality", "ultra"},
		"unknown flag":        {"https://x", "--start", "1", "--bogus"},
		"extra positional":    {"https://x", "--start", "1", "extra"},
		"name with slash":     {"https://x", "--start", "1", "--name", "../evil"},
		"name with backslash": {"https://x", "--start", "1", "--name", `..\evil`},
	}
	for label, args := range cases {
		_, err := parseClipArgs(args)
		if err == nil || errors.Is(err, flag.ErrHelp) {
			t.Errorf("%s: err = %v; want a usage error", label, err)
		}
	}
}

func TestParseClipArgsHelp(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"https://x", "-h"}} {
		if _, err := parseClipArgs(args); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%v: err = %v; want flag.ErrHelp", args, err)
		}
	}
}

func TestHelpListsCommandsFlagsAndExample(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		var out, errb bytes.Buffer
		if code := realMain(args, &out, &errb); code != 0 {
			t.Fatalf("realMain(%v) = %d", args, code)
		}
		for _, want := range []string{
			"vid2gif clip <url>", "vid2gif deps install", "Example:", "--start 1:23",
			"\n  --start ", "\n  --end ", "\n  --format ", "\n  --out ", "\n  --name ", "\n  --quality ",
			"\n  --overwrite\n", "\n  --no-pin\n", "\n  --full-source\n", "\n  --no-progress\n",
		} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("realMain(%v) help is missing %q:\n%s", args, want, out.String())
			}
		}
	}
}

func TestVersion(t *testing.T) {
	defer func(v string) { version = v }(version)
	version = "1.2.3"
	for _, args := range [][]string{{"version"}, {"--version"}} {
		var out, errb bytes.Buffer
		if code := realMain(args, &out, &errb); code != 0 || out.String() != "vid2gif 1.2.3\n" {
			t.Errorf("realMain(%v) = %d, %q", args, code, out.String())
		}
	}
}

func TestRealMainExitCodes(t *testing.T) {
	cases := []struct {
		args []string
		want int
	}{
		{nil, 2},
		{[]string{"bogus"}, 2},
		{[]string{"deps"}, 2},
		{[]string{"deps", "uninstall"}, 2},
		{[]string{"clip"}, 2},
		{[]string{"clip", "https://x", "--start", "abc"}, 2},
		{[]string{"clip", "https://x", "--start", "5", "--end", "1"}, 2},
		{[]string{"clip", "-h"}, 0},
		{[]string{"help"}, 0},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		if got := realMain(c.args, &out, &errb); got != c.want {
			t.Errorf("realMain(%v) = %d; want %d (stderr: %s)", c.args, got, c.want, errb.String())
		}
	}
}

func TestLoadEnv(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("# comment\nVID2GIF_BIN=from-dotenv\n"), 0o644)

	t.Setenv("VID2GIF_BIN", "") // registers cleanup, then really unset it
	os.Unsetenv("VID2GIF_BIN")
	loadEnv(dir)
	if got := os.Getenv("VID2GIF_BIN"); got != "from-dotenv" {
		t.Errorf("VID2GIF_BIN = %q; want the .env value", got)
	}

	t.Setenv("VID2GIF_BIN", "from-real-env")
	loadEnv(dir)
	if got := os.Getenv("VID2GIF_BIN"); got != "from-real-env" {
		t.Errorf("VID2GIF_BIN = %q; a real environment variable must win over .env", got)
	}

	loadEnv(t.TempDir()) // no .env file: must be silent
}
