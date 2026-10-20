package appcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"vid2gif/internal/clip"
	"vid2gif/internal/debuglog"
	"vid2gif/internal/deps"
	"vid2gif/internal/lookup"
	"vid2gif/internal/runner"
	"vid2gif/internal/settings"
)

type event struct {
	name string
	data any
}

type recorder struct {
	mu  sync.Mutex
	evs []event
}

func (r *recorder) emit(name string, data any) {
	r.mu.Lock()
	r.evs = append(r.evs, event{name, data})
	r.mu.Unlock()
}

func (r *recorder) all() []event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.evs)
}

func (r *recorder) names() []string {
	var out []string
	for _, e := range r.all() {
		out = append(out, e.name)
	}
	return out
}

// wait returns the first event called name, failing after a second.
func (r *recorder) wait(t *testing.T, name string) event {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, e := range r.all() {
			if e.name == name {
				return e
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no %q event; got %v", name, r.names())
	return event{}
}

func newTestCore(t *testing.T) (*Core, *recorder) {
	t.Helper()
	dir := t.TempDir()
	rec := &recorder{}
	c := New(rec.emit, debuglog.New(filepath.Join(dir, "logs"), false), filepath.Join(dir, "settings.json"), filepath.Join(dir, "bin"), deps.Pins{})
	c.resolve = func() (Tools, error) { return Tools{"yt", "ff", "deno"}, nil }
	s := c.Current()
	s.OutDir = filepath.Join(t.TempDir(), "out") // outside the data root
	if err := c.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	return c, rec
}

type fakeClip func(ctx context.Context, o clip.Opts) error

func (f fakeClip) Run(ctx context.Context, o clip.Opts) error { return f(ctx, o) }

func TestSaveSettingsValidatesAndFiltersFormats(t *testing.T) {
	c, _ := newTestCore(t)
	good := c.Current()

	for _, bad := range []func(*settings.Settings){
		func(s *settings.Settings) { s.Theme = "neon" },
		func(s *settings.Settings) { s.Quality = "ultra" },
		func(s *settings.Settings) { s.Formats = []string{"bmp"} },
		func(s *settings.Settings) { s.Formats = nil },
		func(s *settings.Settings) { s.OutDir = " " },
	} {
		s := good
		s.Formats = slices.Clone(good.Formats)
		bad(&s)
		if err := c.SaveSettings(s); err == nil {
			t.Errorf("SaveSettings(%+v) = nil, want error", s)
		}
	}

	s := good
	s.Formats = []string{"GIF", "gif", "bmp", " mp4 "}
	s.Theme = "mint"
	if err := c.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if got := c.Current().Formats; !slices.Equal(got, []string{"gif", "mp4"}) {
		t.Errorf("formats = %v, want [gif mp4]", got)
	}
	if got := settings.Load(c.settingsPath); got.Theme != "mint" || !slices.Equal(got.Formats, []string{"gif", "mp4"}) {
		t.Errorf("persisted %+v", got)
	}
}

func TestSaveSettingsAppliesDebugModeAndAllowsOutDir(t *testing.T) {
	c, _ := newTestCore(t)
	s := c.Current()
	s.DebugMode = true
	s.OutDir = filepath.Join(t.TempDir(), "elsewhere")
	if err := c.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	c.Log.Printf("hello")
	if files, _ := filepath.Glob(filepath.Join(c.Log.Dir(), "*.log")); len(files) != 1 {
		t.Errorf("log files = %v, want one after enabling debug mode", files)
	}
	if _, ok := c.Files.URL(filepath.Join(s.OutDir, "a.gif")); !ok {
		t.Error("new OutDir is not allowed for previews")
	}
}

func TestSaveSettingsRefusesUnusableFolders(t *testing.T) {
	c, _ := newTestCore(t)
	good := c.Current()
	dataRoot := filepath.Dir(c.settingsPath)
	root := filepath.VolumeName(good.OutDir) + string(filepath.Separator)
	for _, dir := range []string{`\\host\share\clips`, `\\?\C:\clips`, `\\.\C:\clips`, root, `clips`, dataRoot, filepath.Join(dataRoot, "logs")} {
		s := good
		s.OutDir = dir
		err := c.SaveSettings(s)
		if err == nil || err.Error() != "That folder can't be used. Pick another one." {
			t.Errorf("SaveSettings(OutDir %q) = %v", dir, err)
		}
		if c.Current().OutDir != good.OutDir {
			t.Fatalf("memory changed to %q", c.Current().OutDir)
		}
	}
}

func TestSaveSettingsFailedWriteLeavesMemory(t *testing.T) {
	c, _ := newTestCore(t)
	before := c.Current()
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	c.settingsPath = filepath.Join(blocker, "settings.json") // its folder is a file
	s := before
	s.Theme = "lilac"
	s.OutDir = filepath.Join(t.TempDir(), "new")
	s.DebugMode = true
	if err := c.SaveSettings(s); err == nil {
		t.Fatal("save into a file's folder succeeded")
	}
	if got := c.Current(); got.Theme != before.Theme || got.OutDir != before.OutDir || got.DebugMode {
		t.Errorf("memory changed after a failed save: %+v", got)
	}
}

func TestDebugHeaderWrittenWhenTroubleshootingTurnsOn(t *testing.T) {
	c, _ := newTestCore(t)
	c.Header = func() []string { return []string{"vid2gif test-version on Windows 99"} }
	s := c.Current()
	s.DebugMode = true
	if err := c.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(c.Log.Dir(), "*.log"))
	if len(files) != 1 {
		t.Fatalf("log files = %v", files)
	}
	b, _ := os.ReadFile(files[0])
	for _, want := range []string{"vid2gif test-version on Windows 99", "folders:", "tool: yt-dlp", "tool: ffmpeg", "tool: deno"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("log lacks %q:\n%s", want, b)
		}
	}
	// Saving again with it still on does not repeat the header.
	if err := c.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(files[0])
	if strings.Count(string(b2), "test-version") != 1 {
		t.Errorf("header repeated:\n%s", b2)
	}
}

func TestStatusConvertsFilesToURLs(t *testing.T) {
	c, _ := newTestCore(t)
	out := c.Current().OutDir
	c.Files.Allow(out)
	p := filepath.Join(out, "clip.gif")

	st := c.status(runner.Event{JobID: "1", Kind: "done", Files: []runner.File{{Format: "gif", Path: p, Size: 9}}})
	if len(st.Files) != 1 || st.Files[0].Path != p || !strings.HasPrefix(st.Files[0].URL, "/out/") || st.Files[0].Size != 9 {
		t.Errorf("files = %+v", st.Files)
	}

	b, _ := json.Marshal(c.status(runner.Event{JobID: "1", Kind: "making"}))
	if !strings.Contains(string(b), `"files":[]`) || !strings.Contains(string(b), `"jobId":"1"`) {
		t.Errorf("json = %s, want files [] and the runner fields", b)
	}
}

func TestStatusHintsAtOffPinToolsOnlyForToolTrouble(t *testing.T) {
	c, _ := newTestCore(t)
	site := runner.PlainError(errors.New("ERROR: http error 403"), false)
	gone := runner.PlainError(errors.New("ERROR: private video"), false)

	if got := c.status(runner.Event{Kind: "failed", Message: site}).Message; got != site {
		t.Errorf("tools on their pins: message %q, want it unchanged", got)
	}
	c.offPin.Store(true)
	if got := c.status(runner.Event{Kind: "failed", Message: site}).Message; got != site+" "+offPinMsg {
		t.Errorf("off-pin tools: message %q, want the hint appended", got)
	}
	if got := c.status(runner.Event{Kind: "failed", Message: gone}).Message; got != gone {
		t.Errorf("private video: message %q, want it unchanged", got)
	}
	if got := c.status(runner.Event{Kind: "making"}).Message; got != "" {
		t.Errorf("no message: got %q", got)
	}
}

func TestThrottleSetup(t *testing.T) {
	var got []SetupProgress
	clock := time.Unix(0, 0)
	on, flush := throttle(func(p SetupProgress) { got = append(got, p) }, 100*time.Millisecond, func() time.Time { return clock })
	for i := int64(1); i <= 1000; i++ { // 1000 reads in one second
		clock = clock.Add(time.Millisecond)
		on("deno", i, 2000)
	}
	on("ffmpeg", 1, 10)
	on("ffmpeg", 5, 10)
	flush()
	var deno []SetupProgress
	for _, p := range got {
		if p.Tool == "deno" {
			deno = append(deno, p)
		}
	}
	if len(deno) > 12 {
		t.Errorf("%d deno events for one second, want about 10", len(deno))
	}
	if last := deno[len(deno)-1]; last.Done != 1000 {
		t.Errorf("last deno event %+v, want done 1000 (flushed on tool change)", last)
	}
	if last := got[len(got)-1]; last.Tool != "ffmpeg" || last.Done != 5 {
		t.Errorf("last event %+v, want ffmpeg 5 (flushed)", last)
	}
}

func TestMakeStopIsNotAFailureAndRejectsSecondMake(t *testing.T) {
	c, rec := newTestCore(t)
	started := make(chan struct{})
	c.clip = func(Tools) runner.ClipRunner {
		return fakeClip(func(ctx context.Context, o clip.Opts) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}
	jobs := []runner.Job{{ID: "a", URL: "u", Title: "t", Start: 1, End: 2}, {ID: "b", URL: "u", Start: 1}}
	c.Make(jobs)
	<-started
	c.Make(jobs) // still running
	rec.wait(t, "run:busy")
	c.Stop()
	c.Stop()
	fin := rec.wait(t, "run:finished")
	if !fin.data.(RunFinished).Stopped {
		t.Error("run:finished not marked stopped")
	}
	for _, e := range rec.all() {
		if st, ok := e.data.(Status); ok && (st.Kind == "failed" || st.JobID == "b") {
			t.Errorf("unexpected status after stop: %+v", st)
		}
	}
	// A new run can start once the old one finished.
	c.clip = func(Tools) runner.ClipRunner { return fakeClip(func(context.Context, clip.Opts) error { return nil }) }
	before := len(rec.all())
	c.Make(jobs[:1])
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !slices.Contains(names(rec.all()[before:]), "run:finished") {
		time.Sleep(5 * time.Millisecond)
	}
	if !slices.Contains(names(rec.all()[before:]), "run:finished") {
		t.Fatal("second run never finished")
	}
}

func names(evs []event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, e.name)
	}
	return out
}

func TestMakeNeedsSetupWhenToolsMissing(t *testing.T) {
	c, rec := newTestCore(t)
	c.resolve = func() (Tools, error) { return Tools{}, fs.ErrNotExist }
	ran := false
	c.clip = func(Tools) runner.ClipRunner {
		return fakeClip(func(context.Context, clip.Opts) error { ran = true; return nil })
	}
	c.Make([]runner.Job{{ID: "a", URL: "u", Start: 1}})
	rec.wait(t, "run:finished")
	if !slices.Contains(rec.names(), "setup:needed") || ran {
		t.Errorf("events %v, ran %v; want setup:needed and no run", rec.names(), ran)
	}
}

func TestShutdownCancelsAndWaitsForCleanup(t *testing.T) {
	c, rec := newTestCore(t)
	started := make(chan struct{})
	var cleaned bool
	var mu sync.Mutex
	c.clip = func(Tools) runner.ClipRunner {
		return fakeClip(func(ctx context.Context, o clip.Opts) error {
			close(started)
			<-ctx.Done()
			time.Sleep(50 * time.Millisecond) // temp folder removal
			mu.Lock()
			cleaned = true
			mu.Unlock()
			return ctx.Err()
		})
	}
	c.Make([]runner.Job{{ID: "a", URL: "u", Start: 1, End: 2}})
	<-started
	c.Shutdown(2 * time.Second)
	mu.Lock()
	defer mu.Unlock()
	if !cleaned {
		t.Error("Shutdown returned before the run cleaned up")
	}
	c.Make([]runner.Job{{ID: "a"}}) // after shutdown: ignored, no panic
	for _, e := range rec.all() {
		if e.name == "slip:status" && e.data.(Status).Kind == "failed" {
			t.Errorf("shutdown produced a failure: %+v", e.data)
		}
	}
}

func TestAddReturnsPlaceholdersThenReplaces(t *testing.T) {
	c, rec := newTestCore(t)
	c.lookup = func(ctx context.Context, _ Tools, link string) (string, []lookup.Video, error) {
		if strings.Contains(link, "bad") {
			return "", nil, errors.New("yt-dlp: exit status 1\nERROR: Private video")
		}
		return "My list", []lookup.Video{{ID: "x", URL: "https://v/x", Title: "X", DurationSec: 10}, {ID: "y", Title: "[Private video]", Unavailable: true, Reason: "private"}}, nil
	}
	slips := c.Add("https://good.example/list https://bad.example/v junk")
	if len(slips) != 2 || slips[0].URL != "https://good.example/list" || slips[0].ID == "" || slips[0].ID == slips[1].ID {
		t.Fatalf("placeholders = %+v", slips)
	}
	deadline := time.Now().Add(time.Second)
	var reps []Replace
	for time.Now().Before(deadline) && len(reps) < 2 {
		reps = nil
		for _, e := range rec.all() {
			if r, ok := e.data.(Replace); ok {
				reps = append(reps, r)
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(reps) != 2 {
		t.Fatalf("replace events = %+v", reps)
	}
	for _, r := range reps {
		switch r.ID {
		case slips[0].ID:
			if r.Source != "My list" || len(r.Slips) != 2 || r.Slips[0].Title != "X" || !r.Slips[1].Unavailable {
				t.Errorf("playlist replace = %+v", r)
			}
		case slips[1].ID:
			if len(r.Slips) != 1 || !r.Slips[0].Failed || r.Slips[0].Message == "" || strings.Contains(r.Slips[0].Message, "yt-dlp") {
				t.Errorf("failed replace = %+v", r)
			}
		default:
			t.Errorf("replace for unknown id %q", r.ID)
		}
	}
}

func TestAddFillsStartFromYouTubeLink(t *testing.T) {
	c, rec := newTestCore(t)
	c.lookup = func(ctx context.Context, _ Tools, link string) (string, []lookup.Video, error) {
		return "Pasted links", []lookup.Video{{ID: "x", URL: "https://www.youtube.com/watch?v=x", Title: "X", DurationSec: 600}}, nil
	}
	c.Add("https://youtu.be/x?t=90")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, e := range rec.all() {
			if r, ok := e.data.(Replace); ok {
				if got := r.Slips[0].StartText; got != "1:30" {
					t.Errorf("StartText = %q, want 1:30", got)
				}
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no slips:replace")
}

func TestInstallEvents(t *testing.T) {
	perm := fmt.Errorf("deno: %w", &fs.PathError{Op: "mkdir", Path: `C:\Program Files\vid2gif\bin`, Err: fs.ErrPermission})
	for _, tc := range []struct {
		err       error
		writable  bool
		wantEvent string
		wantAdmin bool
	}{
		{nil, true, "setup:done", false},
		{perm, false, "setup:error", true},
		// A permission error on some other file while bin is writable
		// (antivirus, a locked exe) is not an admin problem.
		{perm, true, "setup:error", false},
		{errors.New("ffmpeg: dial tcp: no such host"), true, "setup:error", false},
	} {
		c, rec := newTestCore(t)
		c.writable = func(string) bool { return tc.writable }
		c.install = func(on func(string, int64, int64)) error {
			on("deno", 5, 10)
			return tc.err
		}
		c.Install()
		e := rec.wait(t, tc.wantEvent)
		if tc.err != nil {
			se := e.data.(SetupError)
			if se.NeedsAdmin != tc.wantAdmin || se.Message == "" || strings.Contains(se.Message, "Program Files") || strings.Contains(se.Message, "ffmpeg") {
				t.Errorf("%v: setup:error = %+v", tc.err, se)
			}
		}
		if !slices.Contains(rec.names(), "setup") {
			t.Errorf("no setup progress event: %v", rec.names())
		}
	}
}

func TestBootReportsNeedsSetup(t *testing.T) {
	c, _ := newTestCore(t)
	if st := c.Boot(); st.NeedsSetup || len(st.Themes) != 3 || st.Settings.Theme == "" {
		t.Errorf("boot = %+v", st)
	}
	c.resolve = func() (Tools, error) { return Tools{}, fs.ErrNotExist }
	if st := c.Boot(); !st.NeedsSetup || st.NeedsAdmin {
		t.Errorf("boot = %+v, want NeedsSetup and not NeedsAdmin (temp bin dir is writable)", st)
	}
}

func TestResolveToolsFallsBackToPATH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := ResolveTools(t.TempDir(), deps.Pins{}, nil)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want not-exist when bin and PATH are both empty", err)
	}
	// The user's own tools on PATH run even though they aren't the pin
	// (an empty one here), each with a warning.
	onPath := t.TempDir()
	for _, n := range []string{"yt-dlp", "ffmpeg", "deno"} {
		if err := os.WriteFile(filepath.Join(onPath, deps.ExeName(n)), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", onPath)
	var warned []string
	tools, err := ResolveTools(t.TempDir(), deps.Pins{}, func(s string) { warned = append(warned, s) })
	if err != nil || len(warned) != 3 || filepath.Dir(tools.YtDlp) != onPath {
		t.Errorf("tools %+v, err %v, warnings %v; want the PATH tools and 3 warnings", tools, err, warned)
	}
}

func TestValidateReturnsSeconds(t *testing.T) {
	c, _ := newTestCore(t)
	if got := c.Validate("1:12", "1:19.5", false, 0); got != (Times{Start: 72, End: 79.5}) {
		t.Errorf("Validate = %+v", got)
	}
	if got := c.Validate("", "", false, 0); got.Message == "" {
		t.Error("empty start accepted")
	}
}

func TestOpenablePath(t *testing.T) {
	c, _ := newTestCore(t)
	out := c.Current().OutDir
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(out, "a.gif")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	made := filepath.Join(t.TempDir(), "old.gif") // an earlier output folder
	if err := os.WriteFile(made, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.Files.Allow(filepath.Dir(made))
	never := filepath.Join(filepath.Dir(made), "never.gif") // in the folder, never made
	if err := os.WriteFile(never, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.status(runner.Event{Kind: "done", Files: []runner.File{{Format: "gif", Path: made}}}) // issues its URL
	stats := 0
	c.stat = func(p string) (os.FileInfo, error) { stats++; return os.Stat(p) }

	for _, p := range []string{``, `rel\a.gif`, `rel/a.gif`, `\\host\share\a.gif`, `//host/share/a.gif`, `\\?\C:\x\a.gif`, `\\.\pipe\x`} {
		if _, _, ok := c.OpenablePath(p); ok {
			t.Errorf("OpenablePath(%q) accepted", p)
		}
	}
	if stats != 0 {
		t.Errorf("stat called %d times for rejected UNC/relative paths", stats)
	}
	if _, _, ok := c.OpenablePath(filepath.Join(t.TempDir(), "x")); ok {
		t.Error("path outside OutDir accepted")
	}
	if _, isDir, ok := c.OpenablePath(out); !ok || !isDir {
		t.Errorf("OutDir: ok %v isDir %v", ok, isDir)
	}
	if p, isDir, ok := c.OpenablePath(file); !ok || isDir || p != file {
		t.Errorf("file under OutDir: %q ok %v isDir %v", p, ok, isDir)
	}
	if _, _, ok := c.OpenablePath(made); !ok {
		t.Error("made file in an allowed folder rejected")
	}
	if _, _, ok := c.OpenablePath(never); ok {
		t.Error("never-made file in an old output folder accepted")
	}
	if err := os.MkdirAll(c.binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, isDir, ok := c.OpenablePath(c.binDir); !ok || !isDir {
		t.Error("helper-files folder not openable")
	}
	if runtime.GOOS == "windows" {
		if _, _, ok := c.OpenablePath(strings.ToUpper(file)); !ok {
			t.Error("case difference rejected on Windows")
		}
	}
}
