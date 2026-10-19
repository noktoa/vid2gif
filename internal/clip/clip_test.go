package clip

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vid2gif/internal/debuglog"
	"vid2gif/internal/encode"
)

type fakeState struct {
	downloads int
	wantAudio bool
	encoded   []string
	progress  []bool // per encode: was a progress callback given
}

func fakePipeline(out io.Writer) (Pipeline, *fakeState) {
	st := &fakeState{}
	return Pipeline{
		title: func(ctx context.Context, url string) (string, error) { return "My: Video", nil },
		download: func(ctx context.Context, o Opts, tmpDir string, wantAudio bool, pr func(float64)) (string, error) {
			st.downloads++
			if pr != nil {
				pr(0.5)
			}
			st.wantAudio = wantAudio
			p := filepath.Join(tmpDir, "clip.mkv")
			return p, os.WriteFile(p, []byte("src"), 0o644)
		},
		encode: func(ctx context.Context, in, out string, eo encode.Opts, pr func(float64)) error {
			st.encoded = append(st.encoded, eo.Format)
			st.progress = append(st.progress, pr != nil)
			if pr != nil {
				pr(0.25)
			}
			return os.WriteFile(out, []byte("data-"+eo.Format), 0o644)
		},
		out: out,
	}, st
}

func baseOpts(dir string, formats ...string) Opts {
	return Opts{URL: "https://example.com/v", Start: 83, End: 88, Formats: formats, OutDir: dir, Quality: "medium"}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestRunWritesAllFormats(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	p, st := fakePipeline(&out)
	if err := p.Run(context.Background(), baseOpts(dir, "gif", "mp4")); err != nil {
		t.Fatal(err)
	}
	want := []string{"My__Video_83-88.gif", "My__Video_83-88.mp4"}
	if got := dirNames(t, dir); !slices.Equal(got, want) {
		t.Errorf("files = %v; want %v (no temp dir may remain)", got, want)
	}
	if st.downloads != 1 || !st.wantAudio {
		t.Errorf("downloads=%d wantAudio=%v", st.downloads, st.wantAudio)
	}
	if !strings.Contains(out.String(), "wrote 2, skipped 0") {
		t.Errorf("summary missing: %q", out.String())
	}
}

func TestRunOtherQualityMakesNewFile(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	p, _ := fakePipeline(&out)
	o := baseOpts(dir, "gif")
	for _, q := range []string{"low", "high"} {
		o.Quality = q
		if err := p.Run(context.Background(), o); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"My__Video_83-88_high.gif", "My__Video_83-88_low.gif"}
	if got := dirNames(t, dir); !slices.Equal(got, want) || strings.Contains(out.String(), "already exists") {
		t.Errorf("files = %v; out=%q", got, out.String())
	}
}

func TestRunGifOnlyDownloadsNoAudio(t *testing.T) {
	p, st := fakePipeline(io.Discard)
	if err := p.Run(context.Background(), baseOpts(t.TempDir(), "gif", "webp")); err != nil {
		t.Fatal(err)
	}
	if st.wantAudio {
		t.Error("gif/webp only must not request audio")
	}
}

func TestRunSkipsExistingAndWritesRest(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "demo.gif"), []byte("sentinel"), 0o644)
	var out bytes.Buffer
	p, _ := fakePipeline(&out)
	o := baseOpts(dir, "gif", "mp4")
	o.Name = "demo"
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "demo.gif")); string(b) != "sentinel" {
		t.Errorf("existing gif was modified: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "demo.mp4")); string(b) != "data-mp4" {
		t.Errorf("mp4 not written: %q", b)
	}
	if !strings.Contains(out.String(), "skipped") || !strings.Contains(out.String(), "wrote 1, skipped 1") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunAllSkippedDoesNotDownload(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "demo.gif"), []byte("sentinel"), 0o644)
	p, st := fakePipeline(io.Discard)
	o := baseOpts(dir, "gif")
	o.Name = "demo"
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if st.downloads != 0 {
		t.Error("must not download when every output already exists")
	}
	if got := dirNames(t, dir); !slices.Equal(got, []string{"demo.gif"}) {
		t.Errorf("files = %v", got)
	}
}

func TestRunOverwriteReplaces(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "demo.gif"), []byte("sentinel"), 0o644)
	p, _ := fakePipeline(io.Discard)
	o := baseOpts(dir, "gif")
	o.Name, o.Overwrite = "demo", true
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "demo.gif")); string(b) != "data-gif" {
		t.Errorf("gif not replaced: %q", b)
	}
}

func TestRunNameGivenSkipsTitleLookup(t *testing.T) {
	p, _ := fakePipeline(io.Discard)
	p.title = func(context.Context, string) (string, error) {
		t.Fatal("title must not be fetched when --name is set")
		return "", nil
	}
	o := baseOpts(t.TempDir(), "gif")
	o.Name = "demo"
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
}

func TestRunEncodeFailureContinuesAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	p, _ := fakePipeline(io.Discard)
	realEncode := p.encode
	p.encode = func(ctx context.Context, in, out string, eo encode.Opts, pr func(float64)) error {
		if eo.Format == "gif" {
			return errors.New("boom")
		}
		return realEncode(ctx, in, out, eo, pr)
	}
	o := baseOpts(dir, "gif", "mp4")
	o.Name = "demo"
	err := p.Run(context.Background(), o)
	if err == nil {
		t.Fatal("expected an error when one format fails")
	}
	if got := dirNames(t, dir); !slices.Equal(got, []string{"demo.mp4"}) {
		t.Errorf("files = %v; want only demo.mp4 and no temp dir", got)
	}
}

func TestRunDownloadFailureLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	p, _ := fakePipeline(io.Discard)
	p.download = func(context.Context, Opts, string, bool, func(float64)) (string, error) {
		return "", errors.New("network down")
	}
	if err := p.Run(context.Background(), baseOpts(dir, "gif")); err == nil {
		t.Fatal("expected error")
	}
	if got := dirNames(t, dir); len(got) != 0 {
		t.Errorf("leftover files: %v", got)
	}
}

func TestRunCancelledContextCleansUp(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	p, _ := fakePipeline(io.Discard)
	realDownload := p.download
	p.download = func(c context.Context, o Opts, tmp string, a bool, pr func(float64)) (string, error) {
		f, err := realDownload(c, o, tmp, a, pr)
		cancel() // Ctrl+C arrives right after the download
		return f, err
	}
	if err := p.Run(ctx, baseOpts(dir, "gif", "mp4")); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
	if got := dirNames(t, dir); len(got) != 0 {
		t.Errorf("leftover files: %v", got)
	}
}

func TestYtdlpDownloadArgs(t *testing.T) {
	o := Opts{URL: "-evil-url", Start: 83.5, End: 90}
	args := ytdlpDownloadArgs(o, "/pinned/ffmpeg", "", "/tmp/x", true)
	if n := len(args); n < 2 || args[n-2] != "--" || args[n-1] != "-evil-url" {
		t.Errorf("URL must follow `--`: %q", args)
	}
	if argAfter(args, "--download-sections") != "*83.5-90" {
		t.Errorf("section = %q", argAfter(args, "--download-sections"))
	}
	if argAfter(args, "--ffmpeg-location") != "/pinned/ffmpeg" {
		t.Error("--ffmpeg-location must point at the pinned ffmpeg")
	}
	if argAfter(args, "-f") != "bv*+ba/b" || !slices.Contains(args, "--force-keyframes-at-cuts") {
		t.Errorf("args = %q", args)
	}
	o.FullSource = true
	if got := argAfter(ytdlpDownloadArgs(o, "f", "", "/tmp/x", false), "-f"); got != "bv*/b" {
		t.Errorf("video-only full-source format = %q", got)
	}
}

func gifOpts(quality string) Opts {
	return Opts{URL: "https://x", Start: 1, End: 2, Quality: quality}
}

func TestDownloadArgsGifOnlyUsesWidthSelector(t *testing.T) {
	for q, w := range map[string]string{"low": "320", "medium": "480", "high": "720"} {
		args := ytdlpDownloadArgs(gifOpts(q), "ffmpeg", "", "tmp", false)
		if got, want := argAfter(args, "-f"), "bv*[width>="+w+"]/wv*[width<"+w+"]/bv*/b"; got != want {
			t.Errorf("%s: -f = %q; want %q", q, got, want)
		}
		if argAfter(args, "-S") != "+width" {
			t.Errorf("%s: -S = %q; want +width (smallest first)", q, argAfter(args, "-S"))
		}
		if strings.Contains(strings.Join(args, " "), "height") {
			t.Errorf("%s: must not cap by height (portrait videos would shrink): %q", q, args)
		}
	}
}

// An unknown quality has no width to aim for, so it must not narrow the source.
func TestDownloadArgsUnknownQualityKeepsFullSelector(t *testing.T) {
	for _, q := range []string{"", "bogus"} {
		args := ytdlpDownloadArgs(gifOpts(q), "ffmpeg", "", "tmp", false)
		if argAfter(args, "-f") != "bv*/b" || slices.Contains(args, "-S") {
			t.Errorf("quality %q: args = %q", q, args)
		}
	}
}

func TestDownloadArgsMp4KeepsFullSelector(t *testing.T) {
	args := ytdlpDownloadArgs(gifOpts("low"), "ffmpeg", "", "tmp", true)
	if argAfter(args, "-f") != "bv*+ba/b" || slices.Contains(args, "-S") {
		t.Errorf("args = %q", args)
	}
}

func TestDownloadArgsFullSourceKeepsFullSelector(t *testing.T) {
	o := gifOpts("low")
	o.FullSource = true
	args := ytdlpDownloadArgs(o, "ffmpeg", "", "tmp", false)
	if argAfter(args, "-f") != "bv*/b" || slices.Contains(args, "-S") {
		t.Errorf("args = %q", args)
	}
}

// A gif plus an mp4 sets wantAudio (see TestRunWritesAllFormats), so the gif is
// cut from the same full-quality download the mp4 needs.
func TestDownloadArgsMixedGifMp4KeepsFullSelector(t *testing.T) {
	p, st := fakePipeline(io.Discard)
	o := baseOpts(t.TempDir(), "gif", "mp4")
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	args := ytdlpDownloadArgs(o, "ffmpeg", "", "tmp", st.wantAudio)
	if argAfter(args, "-f") != "bv*+ba/b" || slices.Contains(args, "-S") {
		t.Errorf("wantAudio=%v args = %q", st.wantAudio, args)
	}
}

func TestYtdlpTitleArgs(t *testing.T) {
	args := ytdlpTitleArgs("-evil-url", "")
	if n := len(args); n < 2 || args[n-2] != "--" || args[n-1] != "-evil-url" {
		t.Errorf("URL must follow `--`: %q", args)
	}
}

func TestPickClipFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "clip.mkv.part"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "clip.mp4"), nil, 0o644)
	got, err := pickClipFile(dir)
	if err != nil || filepath.Base(got) != "clip.mp4" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := pickClipFile(t.TempDir()); err == nil {
		t.Error("expected error when yt-dlp produced nothing")
	}
}

func TestYtdlpIgnoresUserConfig(t *testing.T) {
	for label, args := range map[string][]string{
		"title":    ytdlpTitleArgs("https://x", ""),
		"download": ytdlpDownloadArgs(Opts{URL: "https://x", Start: 1, End: 2}, "ffmpeg", "", "tmp", false),
	} {
		if !slices.Contains(args, "--ignore-config") {
			t.Errorf("%s args must include --ignore-config so a user's yt-dlp config cannot change a pinned run: %q", label, args)
		}
	}
}

func argAfter(args []string, key string) string {
	for i, a := range args {
		if a == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// YouTube extraction needs a JavaScript runtime. The pinned deno is the only
// runtime yt-dlp may use, so a deno elsewhere on the machine can't change results.
func TestYtdlpUsesOnlyPinnedDeno(t *testing.T) {
	o := Opts{URL: "https://x", Start: 1, End: 2}
	for label, args := range map[string][]string{
		"title":    ytdlpTitleArgs("https://x", "/pinned/deno"),
		"download": ytdlpDownloadArgs(o, "ffmpeg", "/pinned/deno", "tmp", false),
	} {
		if argAfter(args, "--js-runtimes") != "deno:/pinned/deno" || !slices.Contains(args, "--no-js-runtimes") {
			t.Errorf("%s args must restrict yt-dlp to the pinned deno: %q", label, args)
		}
		if n := len(args); args[n-2] != "--" {
			t.Errorf("%s: URL must still follow `--`: %q", label, args)
		}
		if slices.Index(args, "--no-js-runtimes") > slices.Index(args, "--js-runtimes") {
			t.Errorf("%s: --no-js-runtimes must come first or it clears the pinned deno: %q", label, args)
		}
	}
	// --no-pin without any deno: leave yt-dlp's own defaults alone.
	if args := ytdlpTitleArgs("https://x", ""); slices.Contains(args, "--js-runtimes") || slices.Contains(args, "--no-js-runtimes") {
		t.Errorf("no deno given, but runtime flags were passed: %q", args)
	}
}

func TestYtdlpSectionRunsToEndWhenNoEnd(t *testing.T) {
	args := ytdlpDownloadArgs(Opts{URL: "https://x", Start: 83.5}, "ffmpeg", "", "tmp", false)
	if got := argAfter(args, "--download-sections"); got != "*83.5-inf" {
		t.Errorf("section = %q; want *83.5-inf", got)
	}
}

// TestMain doubles as a stand-in for yt-dlp/ffmpeg: with VID2GIF_HELPER set,
// the test binary just sleeps so cancellation can be tested on a real process.
func TestMain(m *testing.M) {
	switch os.Getenv("VID2GIF_HELPER") {
	case "sleep":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "fail":
		os.Exit(1)
	case "chatter":
		os.Stdout.WriteString("TOOL-STDOUT\n")
		os.Stderr.WriteString("ERROR: TOOL-STDERR\n")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func collect(o *Opts) *[]Event {
	var evs []Event
	o.OnEvent = func(e Event) { evs = append(evs, e) }
	return &evs
}

// stages collapses consecutive events of one stage (progress ticks).
func stages(evs []Event) []string {
	var s []string
	for _, e := range evs {
		if len(s) == 0 || s[len(s)-1] != e.Stage {
			s = append(s, e.Stage)
		}
	}
	return s
}

func TestRunEmitsEventsInOrder(t *testing.T) {
	dir := t.TempDir()
	p, _ := fakePipeline(io.Discard)
	o := baseOpts(dir, "gif", "mp4")
	evs := collect(&o)
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	want := []string{StageFind, StageDownload, StageEncode, StageWrote, StageEncode, StageWrote}
	if got := stages(*evs); !slices.Equal(got, want) {
		t.Fatalf("stages = %v; want %v", got, want)
	}
	for _, e := range *evs {
		switch e.Stage {
		case StageEncode:
			if filepath.Dir(e.Path) != dir {
				t.Errorf("encode Path = %q; want final path in %s", e.Path, dir)
			}
		case StageWrote:
			if strings.HasSuffix(e.Path, ".gif") && e.Size != int64(len("data-gif")) {
				t.Errorf("gif size = %d", e.Size)
			}
			if strings.HasSuffix(e.Path, ".mp4") && e.Size != int64(len("data-mp4")) {
				t.Errorf("mp4 size = %d", e.Size)
			}
		}
	}
}

func TestRunEmitsSkippedWhenAllExist(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "demo.gif"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "demo.mp4"), []byte("x"), 0o644)
	p, _ := fakePipeline(io.Discard)
	o := baseOpts(dir, "gif", "mp4")
	o.Name = "demo"
	evs := collect(&o)
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range *evs {
		switch e.Stage {
		case StageSkipped:
			n++
		case StageDownload, StageEncode:
			t.Errorf("unexpected %s event", e.Stage)
		}
	}
	if n != 2 {
		t.Errorf("skipped events = %d; want 2", n)
	}
}

func TestRunEmitsFailedAndContinues(t *testing.T) {
	dir := t.TempDir()
	p, _ := fakePipeline(io.Discard)
	realEncode := p.encode
	p.encode = func(ctx context.Context, in, out string, eo encode.Opts, pr func(float64)) error {
		if eo.Format == "gif" {
			return errors.New("boom")
		}
		return realEncode(ctx, in, out, eo, pr)
	}
	o := baseOpts(dir, "gif", "mp4")
	o.Name = "demo"
	evs := collect(&o)
	if err := p.Run(context.Background(), o); err == nil {
		t.Fatal("expected error")
	}
	got := stages(*evs)
	if tail := got[len(got)-4:]; !slices.Equal(tail, []string{StageEncode, StageFailed, StageEncode, StageWrote}) {
		t.Fatalf("stages = %v", got)
	}
	for _, e := range *evs {
		if e.Stage == StageFailed && (e.Err == nil || e.Path != filepath.Join(dir, "demo.gif")) {
			t.Errorf("failed event = %+v", e)
		}
	}
}

// Review Focus 5: a cancel mid-download or mid-encode leaves nothing in OutDir.
func TestRunCancelRemovesTempDir(t *testing.T) {
	for _, where := range []string{"download", "encode"} {
		t.Run(where, func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			p, _ := fakePipeline(io.Discard)
			if where == "download" {
				p.download = func(c context.Context, o Opts, tmp string, a bool, _ func(float64)) (string, error) {
					os.WriteFile(filepath.Join(tmp, "clip.mkv.part"), []byte("half"), 0o644)
					cancel()
					return "", c.Err()
				}
			} else {
				p.encode = func(c context.Context, in, out string, eo encode.Opts, _ func(float64)) error {
					os.WriteFile(out, []byte("half"), 0o644)
					cancel()
					return c.Err()
				}
			}
			o := baseOpts(dir, "gif", "mp4")
			evs := collect(&o)
			if err := p.Run(ctx, o); !errors.Is(err, context.Canceled) {
				t.Errorf("err = %v; want context.Canceled", err)
			}
			if got := dirNames(t, dir); len(got) != 0 {
				t.Errorf("leftovers in OutDir: %v", got)
			}
			if last := (*evs)[len(*evs)-1]; last.Stage != StageFailed {
				t.Errorf("last event = %+v; want failed", last)
			}
		})
	}
}

// A real child process must end when the context is cancelled.
func TestRealToolsStopOnCancel(t *testing.T) {
	t.Setenv("VID2GIF_HELPER", "sleep")
	p := New(os.Args[0], os.Args[0], "", "", io.Discard)
	for name, run := range map[string]func(context.Context) error{
		"download": func(ctx context.Context) error {
			_, err := p.download(ctx, Opts{URL: "https://x"}, t.TempDir(), false, nil)
			return err
		},
		"encode": func(ctx context.Context) error {
			return p.encode(ctx, "in", filepath.Join(t.TempDir(), "out.gif"), encode.Opts{Format: "gif"}, nil)
		},
	} {
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(300*time.Millisecond, cancel)
		start := time.Now()
		err := run(ctx)
		cancel()
		// Under the 5s WaitDelay: fails if Cancel did nothing and Wait merely gave up.
		if !errors.Is(err, context.Canceled) || time.Since(start) > 4*time.Second {
			t.Errorf("%s: err=%v after %v; want context.Canceled promptly", name, err, time.Since(start))
		}
	}
}

func TestRunNilOnEventStillWorks(t *testing.T) {
	p, _ := fakePipeline(io.Discard)
	if err := p.Run(context.Background(), baseOpts(t.TempDir(), "gif")); err != nil {
		t.Fatal(err)
	}
}

func readLog(t *testing.T, dir string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "*.log"))
	if len(files) != 1 {
		t.Fatalf("log files = %v", files)
	}
	b, _ := os.ReadFile(files[0])
	return string(b)
}

func TestWithLogRecordsCommandsAndEvents(t *testing.T) {
	logDir := t.TempDir()
	p, _ := fakePipeline(io.Discard)
	p = p.WithLog(debuglog.New(logDir, true))
	o := baseOpts(t.TempDir(), "gif")
	o.URL = "https://example.com/v?token=SECRET&x=1"
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	got := readLog(t, logDir)
	for _, want := range []string{"yt-dlp", "ffmpeg", "--download-sections", "stage=find", "stage=download", "stage=encode", "stage=wrote"} {
		if !strings.Contains(got, want) {
			t.Errorf("log missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "SECRET") {
		t.Errorf("log leaked the query:\n%s", got)
	}
}

func TestWithLogRecordsFinalError(t *testing.T) {
	logDir := t.TempDir()
	p, _ := fakePipeline(io.Discard)
	p.download = func(context.Context, Opts, string, bool, func(float64)) (string, error) {
		return "", errors.New("network down")
	}
	p = p.WithLog(debuglog.New(logDir, true))
	p.Run(context.Background(), baseOpts(t.TempDir(), "gif"))
	if got := readLog(t, logDir); !strings.Contains(got, "network down") {
		t.Errorf("final error not logged:\n%s", got)
	}
}

func TestWithLogDisabledWritesNothing(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	p, _ := fakePipeline(io.Discard)
	p = p.WithLog(debuglog.New(logDir, false))
	if err := p.Run(context.Background(), baseOpts(t.TempDir(), "gif")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logDir); !os.IsNotExist(err) {
		t.Errorf("disabled log created %s (err=%v)", logDir, err)
	}
}

func TestLineWriterLogsEachLine(t *testing.T) {
	logDir := t.TempDir()
	w := &lineWriter{log: debuglog.New(logDir, true), prefix: "yt-dlp"}
	w.Write([]byte("[download]  10%\r[download]  20%\nsecond "))
	w.Write([]byte("line\n"))
	w.Write([]byte("tail"))
	w.Close()
	got := readLog(t, logDir)
	for _, want := range []string{"yt-dlp: [download]  10%", "yt-dlp: [download]  20%", "yt-dlp: second line", "yt-dlp: tail"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

// A killed tool exits non-zero; Run must still report the cancel, with no
// "update yt-dlp" hint, and must not start the next format.
func TestRunCancelWithToolExitErrorReportsCanceled(t *testing.T) {
	for _, where := range []string{"download", "encode"} {
		t.Run(where, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			var out bytes.Buffer
			p, _ := fakePipeline(&out)
			encodes := 0
			toolErr := errors.New("yt-dlp: exit status 1\nBUMP-THE-PIN")
			if where == "download" {
				p.download = func(context.Context, Opts, string, bool, func(float64)) (string, error) {
					cancel()
					return "", toolErr
				}
			} else {
				p.encode = func(context.Context, string, string, encode.Opts, func(float64)) error {
					encodes++
					cancel()
					return toolErr
				}
			}
			o := baseOpts(t.TempDir(), "gif", "mp4")
			evs := collect(&o)
			err := p.Run(ctx, o)
			if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "BUMP") || strings.Contains(out.String(), "BUMP") {
				t.Errorf("err = %v; out = %q", err, out.String())
			}
			if encodes > 1 {
				t.Errorf("encodes = %d; remaining formats must not run", encodes)
			}
			if last := (*evs)[len(*evs)-1]; last.Stage != StageFailed || !errors.Is(last.Err, context.Canceled) {
				t.Errorf("last event = %+v; want failed with context.Canceled", last)
			}
		})
	}
}

// syncBuf is a bytes.Buffer safe for the two tool-output copy goroutines.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

// WithToolOutput takes yt-dlp's own stdout and stderr (title and download)
// off the terminal; the CLI's own lines still go to out.
func TestWithToolOutputCapturesYtdlp(t *testing.T) {
	t.Setenv("VID2GIF_HELPER", "chatter")
	for _, name := range []string{"", "given"} { // "" runs the title lookup
		var tool syncBuf
		var out bytes.Buffer
		p := New(os.Args[0], os.Args[0], "", "", &out).WithToolOutput(&tool)
		o := baseOpts(t.TempDir(), "gif")
		o.Name = name
		if err := p.Run(context.Background(), o); err == nil {
			t.Fatalf("name %q: want the tool's failure", name)
		}
		got := tool.b.String()
		if !strings.Contains(got, "TOOL-STDERR") || (name != "" && !strings.Contains(got, "TOOL-STDOUT")) {
			t.Errorf("name %q: captured %q", name, got)
		}
		if strings.Contains(out.String(), "TOOL-") {
			t.Errorf("name %q: tool output leaked into out: %q", name, out.String())
		}
	}
}

// WithToolOutput(nil), the CLI's plain path, leaves yt-dlp writing to this
// process's own stdout and stderr (the terminal), not to a capture.
func TestWithToolOutputNilPassesThrough(t *testing.T) {
	t.Setenv("VID2GIF_HELPER", "chatter")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	var out bytes.Buffer
	p := New(os.Args[0], os.Args[0], "", "", &out).WithToolOutput(nil)
	o := baseOpts(t.TempDir(), "gif")
	o.Name = "given"
	runErr := p.Run(context.Background(), o)
	os.Stdout, os.Stderr = oldOut, oldErr
	w.Close()
	got, _ := io.ReadAll(r)
	if runErr == nil || !strings.Contains(string(got), "TOOL-STDOUT") || !strings.Contains(string(got), "TOOL-STDERR") {
		t.Errorf("err = %v; process stdout/stderr got %q, want the tool's output", runErr, got)
	}
}

func TestToolOutputDefaultsToTerminal(t *testing.T) {
	if o, e := toolOutput(context.Background()); o != os.Stdout || e != os.Stderr {
		t.Errorf("default tool output = %v, %v; want os.Stdout, os.Stderr", o, e)
	}
}

// Without a cancel, a failing tool keeps its error text and the hint.
func TestRealToolFailureKeepsHint(t *testing.T) {
	t.Setenv("VID2GIF_HELPER", "fail")
	p := New(os.Args[0], os.Args[0], "", "BUMP-THE-PIN", io.Discard)
	_, err := p.download(context.Background(), Opts{URL: "https://x"}, t.TempDir(), false, nil)
	if err == nil || errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "exit status 1") || !strings.Contains(err.Error(), "BUMP-THE-PIN") {
		t.Errorf("err = %v; want yt-dlp exit error with hint", err)
	}
}

func TestSplitProgressSplitsOnCR(t *testing.T) {
	sc := bufio.NewScanner(strings.NewReader("a\rb\nc\r\nd"))
	sc.Split(SplitProgress)
	var got []string
	for sc.Scan() {
		if sc.Text() != "" {
			got = append(got, sc.Text())
		}
	}
	if !slices.Equal(got, []string{"a", "b", "c", "d"}) {
		t.Errorf("got %q", got)
	}
}

func TestParseFFmpegTimeFixture(t *testing.T) {
	f, err := os.Open("testdata/ffmpeg-section-stderr.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Split(SplitProgress)
	var got []float64
	skippedNA := false
	for sc.Scan() {
		sec, ok := ParseFFmpegTime(sc.Text())
		if !ok {
			skippedNA = skippedNA || strings.Contains(sc.Text(), "time=N/A")
			continue
		}
		got = append(got, sec)
	}
	if len(got) == 0 || got[0] != 0.05 {
		t.Fatalf("first = %v", got)
	}
	// 8.63 is the last stats line; the closing summary line (Lsize=) says 10.00.
	if n := len(got); n < 2 || got[n-2] != 8.63 || got[n-1] != 10 {
		t.Errorf("tail = %v", got)
	}
	if !skippedNA || !sort.Float64sAreSorted(got) {
		t.Errorf("time=N/A skipped=%v, sorted=%v", skippedNA, sort.Float64sAreSorted(got))
	}
}

func TestParseFFmpegTimeRejects(t *testing.T) {
	for _, l := range []string{"", "time=N/A bitrate=N/A", "[download] 10%", "elapsed=0:00:01.02", "time=00:00"} {
		if _, ok := ParseFFmpegTime(l); ok {
			t.Errorf("%q must not be ok", l)
		}
	}
	if s, ok := ParseFFmpegTime("size= 5KiB time=01:02:03.50 bitrate="); !ok || s != 3723.5 {
		t.Errorf("got %v, %v", s, ok)
	}
}

func TestDownloadFractionUsesDuration(t *testing.T) {
	var got []float64
	feed := downloadProgress(10, func(f float64) { got = append(got, f) })
	feed("frame=3 time=00:00:05.00 bitrate=1")
	feed("time=N/A")
	feed("[download] nothing")
	feed("time=00:00:12.00") // past the end clamps
	if !slices.Equal(got, []float64{0.5, 1}) {
		t.Errorf("got %v", got)
	}
	got = nil
	downloadProgress(0, func(f float64) { got = append(got, f) })("time=00:00:05.00")
	if !slices.Equal(got, []float64{-1}) {
		t.Errorf("unknown duration: got %v; want [-1]", got)
	}
}

func TestRunProgressEvents(t *testing.T) {
	p, st := fakePipeline(io.Discard)
	o := baseOpts(t.TempDir(), "gif", "mp4")
	o.DurationSec = 5
	evs := collect(&o)
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if first := (*evs)[0]; first.Stage != StageFind || first.Pct != -1 {
		t.Errorf("first = %+v; want find with Pct -1", first)
	}
	if second := (*evs)[1]; second.Stage != StageDownload || second.Pct != 0.5 {
		t.Errorf("second = %+v; want download at the first progress report, not before", second)
	}
	for _, e := range *evs {
		if e.Stage == StageEncode && strings.HasSuffix(e.Path, ".gif") && e.Pct != -1 {
			t.Errorf("gif encode Pct = %v; want -1", e.Pct)
		}
	}
	// gif is planned first, then mp4
	if !slices.Equal(st.progress, []bool{false, true}) {
		t.Errorf("progress callbacks per encode = %v; want only mp4/mov", st.progress)
	}
}

func TestRunUnknownDurationIsIndeterminate(t *testing.T) {
	p, _ := fakePipeline(io.Discard)
	p.download = func(_ context.Context, _ Opts, tmp string, _ bool, pr func(float64)) (string, error) {
		pr(-1)
		path := filepath.Join(tmp, "clip.mkv")
		return path, os.WriteFile(path, []byte("src"), 0o644)
	}
	p.encode = func(_ context.Context, _, out string, _ encode.Opts, _ func(float64)) error {
		// ffmpeg reports nothing without a duration
		return os.WriteFile(out, []byte("x"), 0o644)
	}
	o := baseOpts(t.TempDir(), "mp4") // DurationSec 0
	evs := collect(&o)
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	for _, e := range *evs {
		if (e.Stage == StageDownload || e.Stage == StageEncode) && e.Pct != -1 {
			t.Errorf("%+v; want Pct -1 while the duration is unknown", e)
		}
	}
}

func TestRunProgressNeverGoesBackwards(t *testing.T) {
	p, _ := fakePipeline(io.Discard)
	p.download = func(_ context.Context, _ Opts, tmp string, _ bool, pr func(float64)) (string, error) {
		for _, f := range []float64{0.4, 0.3, 0.6} {
			pr(f)
		}
		path := filepath.Join(tmp, "clip.mkv")
		return path, os.WriteFile(path, []byte("src"), 0o644)
	}
	o := baseOpts(t.TempDir(), "gif")
	o.DurationSec = 5
	evs := collect(&o)
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	last := -1.0
	for _, e := range *evs {
		if e.Stage == StageDownload {
			if e.Pct < last {
				t.Errorf("download went back: %v after %v", e.Pct, last)
			}
			last = e.Pct
		}
	}
}

// Progress arrives on the tool's output goroutine while Run emits its own
// events; OnEvent must never run concurrently or after Run returns.
func TestRunSerializesEventsAndStopsAfterReturn(t *testing.T) {
	p, _ := fakePipeline(io.Discard)
	release := make(chan struct{})
	var wg sync.WaitGroup
	p.download = func(_ context.Context, _ Opts, tmp string, _ bool, pr func(float64)) (string, error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				pr(float64(i) / 200)
			}
			<-release
			pr(1) // a straggler after Run returned
		}()
		path := filepath.Join(tmp, "clip.mkv")
		return path, os.WriteFile(path, []byte("src"), 0o644)
	}
	o := baseOpts(t.TempDir(), "gif")
	var inside atomic.Int32
	var after, done atomic.Bool
	o.OnEvent = func(Event) {
		if inside.Add(1) != 1 {
			t.Error("OnEvent called concurrently")
		}
		if done.Load() {
			after.Store(true)
		}
		inside.Add(-1)
	}
	if err := p.Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	done.Store(true)
	close(release)
	wg.Wait()
	if after.Load() {
		t.Error("OnEvent called after Run returned")
	}
}

func TestTeeStderrScansLinesAndKeepsOutput(t *testing.T) {
	var next bytes.Buffer
	var lines []string
	w, done := teeStderr(context.Background(), "yt-dlp", &next, func(l string) { lines = append(lines, l) })
	io.WriteString(w, "frame=1 time=00:00:01.00\rframe=2 time=00:00:02")
	io.WriteString(w, ".00\rtail")
	done()
	if next.String() != "frame=1 time=00:00:01.00\rframe=2 time=00:00:02.00\rtail" {
		t.Errorf("terminal output changed: %q", next.String())
	}
	if !slices.Equal(lines, []string{"frame=1 time=00:00:01.00", "frame=2 time=00:00:02.00", "tail"}) {
		t.Errorf("lines = %q", lines)
	}
}
