package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vid2gif/internal/clip"
	"vid2gif/internal/debuglog"
	"vid2gif/internal/naming"
	"vid2gif/internal/settings"
)

// fakeClock makes now() advance by step on every call.
func fakeClock(t *testing.T, step time.Duration) {
	t.Helper()
	cur := time.Unix(0, 0)
	old := now
	now = func() time.Time { cur = cur.Add(step); return cur }
	t.Cleanup(func() { now = old })
}

// fakeRunner calls fn per Run, recording the opts.
type fakeRunner struct {
	calls []clip.Opts
	fn    func(ctx context.Context, o clip.Opts) error
}

func (f *fakeRunner) Run(ctx context.Context, o clip.Opts) error {
	f.calls = append(f.calls, o)
	return f.fn(ctx, o)
}

// writeFiles mimics clip.Run: skip existing outputs, write the rest.
func writeFiles(_ context.Context, o clip.Opts) error {
	o.OnEvent(clip.Event{Stage: clip.StageFind, Pct: -1})
	todo, skipped := naming.PlanOutputs(o.OutDir, o.Name, o.Formats, o.Overwrite)
	for _, f := range skipped {
		o.OnEvent(clip.Event{Stage: clip.StageSkipped, Pct: 1, Path: naming.OutPath(o.OutDir, o.Name, f)})
	}
	if len(todo) == 0 {
		return nil
	}
	o.OnEvent(clip.Event{Stage: clip.StageDownload, Pct: 0.5})
	for _, f := range todo {
		p := naming.OutPath(o.OutDir, o.Name, f)
		o.OnEvent(clip.Event{Stage: clip.StageEncode, Pct: -1, Path: p})
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			return err
		}
		o.OnEvent(clip.Event{Stage: clip.StageWrote, Pct: 1, Path: p, Size: 1})
	}
	return nil
}

func testSettings(t *testing.T, formats ...string) settings.Settings {
	return settings.Settings{OutDir: t.TempDir(), Formats: formats, Quality: "medium"}
}

func jobs(n int) []Job {
	var js []Job
	for i := 1; i <= n; i++ {
		js = append(js, Job{ID: fmt.Sprint("j", i), URL: fmt.Sprint("https://x/", i), Title: fmt.Sprint("Video ", i), Start: 1, End: 11})
	}
	return js
}

func collect(ctx context.Context, cr ClipRunner, js []Job, s settings.Settings, l *debuglog.Log) []Event {
	var evs []Event
	Run(ctx, cr, js, s, l, func(e Event) { evs = append(evs, e) })
	return evs
}

// final returns each job's last non-making event.
func final(evs []Event) map[string]Event {
	m := map[string]Event{}
	for _, e := range evs {
		if e.Kind != "making" {
			m[e.JobID] = e
		}
	}
	return m
}

func making(evs []Event, id string) []Event {
	var out []Event
	for _, e := range evs {
		if e.JobID == id && e.Kind == "making" {
			out = append(out, e)
		}
	}
	return out
}

func TestRunSequentialAndContinuesAfterFailure(t *testing.T) {
	fakeClock(t, time.Second)
	fr := &fakeRunner{fn: func(ctx context.Context, o clip.Opts) error {
		if strings.HasSuffix(o.URL, "/2") {
			return errors.New("ffmpeg: exit status 1")
		}
		return writeFiles(ctx, o)
	}}
	evs := collect(context.Background(), fr, jobs(3), testSettings(t, "gif"), nil)
	if len(fr.calls) != 3 || fr.calls[0].URL != "https://x/1" || fr.calls[2].URL != "https://x/3" {
		t.Fatalf("calls = %+v", fr.calls)
	}
	f := final(evs)
	if f["j1"].Kind != "done" || f["j2"].Kind != "failed" || f["j3"].Kind != "done" {
		t.Fatalf("finals = %+v", f)
	}
	if !strings.HasPrefix(f["j2"].Message, "Something went wrong with this one.") {
		t.Errorf("j2 message = %q", f["j2"].Message)
	}
	// events of a job never interleave with the next one's
	last := ""
	seen := map[string]bool{}
	for _, e := range evs {
		if e.JobID != last {
			if seen[e.JobID] {
				t.Fatalf("job %s events interleaved", e.JobID)
			}
			seen[e.JobID], last = true, e.JobID
		}
	}
}

func TestRunOptsFromSettings(t *testing.T) {
	fakeClock(t, time.Second)
	fr := &fakeRunner{fn: writeFiles}
	s := testSettings(t, "GIF", "bogus", "mp4", "gif")
	s.Quality = "high"
	collect(context.Background(), fr, jobs(1), s, nil)
	o := fr.calls[0]
	if strings.Join(o.Formats, ",") != "gif,mp4" || o.Quality != "high" || o.OutDir != s.OutDir || o.Overwrite {
		t.Errorf("opts = %+v", o)
	}
	if want := naming.BuildName("Video 1", 1, 11, "high"); o.Name != want {
		t.Errorf("Name = %q, want %q", o.Name, want)
	}
	if o.DurationSec != 10 {
		t.Errorf("DurationSec = %v, want 10", o.DurationSec)
	}

	fr = &fakeRunner{fn: writeFiles}
	collect(context.Background(), fr, jobs(1), testSettings(t, "nope"), nil)
	if strings.Join(fr.calls[0].Formats, ",") != "gif" {
		t.Errorf("empty formats = %v, want gif", fr.calls[0].Formats)
	}
}

func TestFullSourcePassedThrough(t *testing.T) {
	fakeClock(t, time.Second)
	for _, full := range []bool{false, true} {
		fr := &fakeRunner{fn: writeFiles}
		s := testSettings(t, "gif")
		s.FullSource = full
		collect(context.Background(), fr, jobs(1), s, nil)
		if fr.calls[0].FullSource != full {
			t.Errorf("FullSource = %v, want %v", fr.calls[0].FullSource, full)
		}
	}
}

func TestRunPartial(t *testing.T) {
	fakeClock(t, time.Second)
	fr := &fakeRunner{fn: func(ctx context.Context, o clip.Opts) error {
		gif := naming.OutPath(o.OutDir, o.Name, "gif")
		mp4 := naming.OutPath(o.OutDir, o.Name, "mp4")
		o.OnEvent(clip.Event{Stage: clip.StageFind, Pct: -1})
		o.OnEvent(clip.Event{Stage: clip.StageEncode, Pct: -1, Path: gif})
		o.OnEvent(clip.Event{Stage: clip.StageWrote, Pct: 1, Path: gif, Size: 42})
		o.OnEvent(clip.Event{Stage: clip.StageEncode, Pct: 0, Path: mp4})
		o.OnEvent(clip.Event{Stage: clip.StageFailed, Pct: -1, Path: mp4, Err: errors.New("ffmpeg: exit status 1")})
		return errors.New("1 of 2 outputs failed")
	}}
	evs := collect(context.Background(), fr, jobs(1), testSettings(t, "gif", "mp4"), nil)
	e := final(evs)["j1"]
	if e.Kind != "partial" || e.Message != "The MP4 couldn't be made." {
		t.Fatalf("final = %+v", e)
	}
	if len(e.Files) != 1 || e.Files[0].Format != "gif" || e.Files[0].Size != 42 {
		t.Errorf("files = %+v", e.Files)
	}
}

func TestRunAlready(t *testing.T) {
	fakeClock(t, time.Second)
	s := testSettings(t, "gif", "webp")
	fr := &fakeRunner{fn: writeFiles}
	collect(context.Background(), fr, jobs(1), s, nil)
	evs := collect(context.Background(), fr, jobs(1), s, nil)
	e := final(evs)["j1"]
	if e.Kind != "already" || len(e.Files) != 2 {
		t.Fatalf("final = %+v", e)
	}
}

func TestRunOtherQualityMakesNewFile(t *testing.T) {
	fakeClock(t, time.Second)
	s := testSettings(t, "gif")
	fr := &fakeRunner{fn: writeFiles}
	s.Quality = "low"
	a := final(collect(context.Background(), fr, jobs(1), s, nil))["j1"]
	s.Quality = "high"
	b := final(collect(context.Background(), fr, jobs(1), s, nil))["j1"]
	if a.Kind != "done" || b.Kind != "done" {
		t.Fatalf("kinds = %s, %s", a.Kind, b.Kind)
	}
	files, _ := filepath.Glob(filepath.Join(s.OutDir, "*.gif"))
	if len(files) != 2 {
		t.Errorf("files = %v, want 2", files)
	}
}

func TestRunStopMarksCurrentReadyAndStops(t *testing.T) {
	fakeClock(t, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fr := &fakeRunner{fn: func(ctx context.Context, o clip.Opts) error {
		o.OnEvent(clip.Event{Stage: clip.StageFind, Pct: -1})
		o.OnEvent(clip.Event{Stage: clip.StageDownload, Pct: 0.3})
		cancel()
		return fmt.Errorf("yt-dlp: %w", ctx.Err())
	}}
	evs := collect(ctx, fr, jobs(3), testSettings(t, "gif"), nil)
	if len(fr.calls) != 1 {
		t.Fatalf("ran %d jobs, want 1", len(fr.calls))
	}
	for _, e := range evs {
		if e.JobID != "j1" {
			t.Fatalf("event for unstarted job: %+v", e)
		}
		if e.Kind == "failed" || (e.Message != "" && e.Kind != "ready") {
			t.Fatalf("cancel shown as failure: %+v", e)
		}
	}
	if evs[len(evs)-1].Kind != "ready" {
		t.Errorf("last = %+v, want ready", evs[len(evs)-1])
	}
}

func TestRunBadFolder(t *testing.T) {
	fakeClock(t, time.Second)
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s := testSettings(t, "gif")
	s.OutDir = file
	fr := &fakeRunner{fn: writeFiles}
	f := final(collect(context.Background(), fr, jobs(2), s, nil))
	for _, id := range []string{"j1", "j2"} {
		if f[id].Kind != "failed" || f[id].Message != "Couldn't save to that folder. Pick another one." {
			t.Errorf("%s = %+v", id, f[id])
		}
	}
}

func TestRunThrottlesMaking(t *testing.T) {
	fakeClock(t, time.Millisecond) // 1000 events = about 1 s of fake time
	fr := &fakeRunner{fn: func(ctx context.Context, o clip.Opts) error {
		o.OnEvent(clip.Event{Stage: clip.StageFind, Pct: -1})
		for i := 1; i <= 1000; i++ {
			o.OnEvent(clip.Event{Stage: clip.StageDownload, Pct: float64(i) / 1000})
		}
		return nil
	}}
	m := making(collect(context.Background(), fr, jobs(1), testSettings(t, "gif"), nil), "j1")
	var dl []Event
	for _, e := range m {
		if e.Stage == "download" {
			dl = append(dl, e)
		}
	}
	if len(dl) > 13 || len(dl) < 2 {
		t.Fatalf("download events = %d, want about 10", len(dl))
	}
	if dl[0].StagePct != 0.001 {
		t.Errorf("first = %v, want 0.001", dl[0].StagePct)
	}
	for i, e := range dl {
		if e.StagePct > 0.99 || (i > 0 && e.StagePct < dl[i-1].StagePct) {
			t.Fatalf("stage pct %d = %v (prev %v), want non-decreasing and <= 0.99", i, e.StagePct, dl[max(i-1, 0)].StagePct)
		}
	}
	if dl[len(dl)-1].StagePct != 0.99 { // the reported 1.0, capped
		t.Errorf("last = %v, want 0.99", dl[len(dl)-1].StagePct)
	}
}

// A lone format's failure is classified by its own error, not clip's
// aggregate "1 of 1 outputs failed".
func TestRunClassifiesFormatError(t *testing.T) {
	fakeClock(t, time.Second)
	for raw, want := range map[string]string{
		"rename a b: Access is denied.": "Couldn't save to that folder. Pick another one.",
		"yt-dlp: exit status 1\nERROR: [youtube] x: Sign in to confirm you're not a bot": "This site isn't working with the app right now. An update may fix it.",
	} {
		fr := &fakeRunner{fn: func(ctx context.Context, o clip.Opts) error {
			gif := naming.OutPath(o.OutDir, o.Name, "gif")
			o.OnEvent(clip.Event{Stage: clip.StageFind, Pct: -1})
			o.OnEvent(clip.Event{Stage: clip.StageEncode, Pct: -1, Path: gif})
			o.OnEvent(clip.Event{Stage: clip.StageFailed, Pct: -1, Path: gif, Err: errors.New(raw)})
			return errors.New("1 of 1 outputs failed")
		}}
		e := final(collect(context.Background(), fr, jobs(1), testSettings(t, "gif"), nil))["j1"]
		if e.Kind != "failed" || e.Message != want {
			t.Errorf("%q: final = %+v, want failed %q", raw, e, want)
		}
	}
}

func TestRunThrottleFlushesLast(t *testing.T) {
	fakeClock(t, time.Millisecond)
	fr := &fakeRunner{fn: func(ctx context.Context, o clip.Opts) error {
		for i := 1; i <= 50; i++ {
			o.OnEvent(clip.Event{Stage: clip.StageDownload, Pct: float64(i) / 100})
		}
		return errors.New("boom")
	}}
	m := making(collect(context.Background(), fr, jobs(1), testSettings(t, "gif"), nil), "j1")
	if len(m) == 0 || m[len(m)-1].StagePct != 0.5 {
		t.Fatalf("making = %+v, want last 0.5", m)
	}
}

// progressRun reports a typical gif+mp4 run with wobbly stage fractions.
func progressRun(ctx context.Context, o clip.Opts) error {
	gif := naming.OutPath(o.OutDir, o.Name, "gif")
	mp4 := naming.OutPath(o.OutDir, o.Name, "mp4")
	for _, e := range []clip.Event{
		{Stage: clip.StageFind, Pct: -1},
		{Stage: clip.StageDownload, Pct: 0.2},
		{Stage: clip.StageDownload, Pct: 0.6},
		{Stage: clip.StageDownload, Pct: 1},
		{Stage: clip.StageEncode, Pct: -1, Path: gif},
		{Stage: clip.StageWrote, Pct: 1, Path: gif, Size: 1},
		{Stage: clip.StageEncode, Pct: 0, Path: mp4},
		{Stage: clip.StageEncode, Pct: 0.5, Path: mp4},
		{Stage: clip.StageEncode, Pct: 0.4, Path: mp4},
		{Stage: clip.StageEncode, Pct: 1, Path: mp4},
		{Stage: clip.StageWrote, Pct: 1, Path: mp4, Size: 1},
	} {
		o.OnEvent(e)
	}
	return nil
}

func TestOverallPctNeverDecreases(t *testing.T) {
	fakeClock(t, time.Second)
	fr := &fakeRunner{fn: progressRun}
	m := making(collect(context.Background(), fr, jobs(1), testSettings(t, "gif", "mp4"), nil), "j1")
	if len(m) < 5 {
		t.Fatalf("making = %+v", m)
	}
	prev := 0.0
	for _, e := range m {
		if e.Pct < prev || e.Pct < 0 {
			t.Fatalf("pct went %v -> %v (%+v)", prev, e.Pct, m)
		}
		prev = e.Pct
	}
	if prev <= 0.5 {
		t.Errorf("final making pct = %v, want most of the bar", prev)
	}
}

func TestOverallPctStaysBelow1UntilDone(t *testing.T) {
	fakeClock(t, time.Second)
	fr := &fakeRunner{fn: progressRun}
	evs := collect(context.Background(), fr, jobs(1), testSettings(t, "gif", "mp4"), nil)
	for i, e := range evs {
		if e.Kind == "making" && e.Pct >= 1 {
			t.Fatalf("making event at 100%%: %+v", e)
		}
		if e.Kind == "done" && (i == 0 || evs[i-1].Kind != "making") {
			t.Fatalf("done not preceded by making: %+v", evs)
		}
	}
	if last := evs[len(evs)-1]; last.Kind != "done" || last.Pct != 1 {
		t.Errorf("last = %+v, want done at 1", last)
	}
}

func TestOverallPctUnknownLengthIsMinus1(t *testing.T) {
	fakeClock(t, time.Second)
	fr := &fakeRunner{fn: progressRun}
	js := []Job{{ID: "j1", URL: "https://x/1", Title: "Live", Start: 30}} // no End, no duration
	m := making(collect(context.Background(), fr, js, testSettings(t, "gif", "mp4"), nil), "j1")
	stages := map[string]bool{}
	for _, e := range m {
		if e.Pct != -1 {
			t.Fatalf("pct = %v, want -1: %+v", e.Pct, e)
		}
		stages[e.Stage] = true
	}
	for _, s := range []string{"find", "download", "gif", "mp4"} {
		if !stages[s] {
			t.Errorf("stage %s never reported (%v)", s, stages)
		}
	}
	if fr.calls[0].DurationSec != 0 {
		t.Errorf("DurationSec = %v, want 0", fr.calls[0].DurationSec)
	}
}

func TestRunClipLengthFromDuration(t *testing.T) {
	fakeClock(t, time.Second)
	fr := &fakeRunner{fn: writeFiles}
	js := []Job{{ID: "j1", URL: "https://x/1", Title: "T", Start: 30, DurationSec: 100}}
	collect(context.Background(), fr, js, testSettings(t, "gif"), nil)
	if fr.calls[0].DurationSec != 70 {
		t.Errorf("DurationSec = %v, want 70", fr.calls[0].DurationSec)
	}
}

func TestRunLogsOriginalErrorWhenDebugOn(t *testing.T) {
	fakeClock(t, time.Second)
	dir := t.TempDir()
	l := debuglog.New(dir, true)
	fr := &fakeRunner{fn: func(context.Context, clip.Opts) error {
		return errors.New("ffmpeg: exit status 1\nraw-detail-xyz")
	}}
	s := testSettings(t, "gif")
	s.DebugMode = true
	e := final(collect(context.Background(), fr, jobs(1), s, l))["j1"]
	if e.Message != "Something went wrong with this one. Details were saved to the log." {
		t.Errorf("message = %q", e.Message)
	}
	logs, _ := filepath.Glob(filepath.Join(dir, "*.log"))
	if len(logs) != 1 {
		t.Fatalf("logs = %v", logs)
	}
	b, _ := os.ReadFile(logs[0])
	if !strings.Contains(string(b), "j1") || !strings.Contains(string(b), "raw-detail-xyz") {
		t.Errorf("log = %s", b)
	}
	// a nil log must not break anything
	collect(context.Background(), fr, jobs(1), s, nil)
}
