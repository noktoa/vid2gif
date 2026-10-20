// Package appcore is the desktop app's logic behind the methods the page
// calls: settings, lookups, runs, tool setup. It knows nothing about Wails
// (cmd/vid2gif-app is the thin Windows-only glue), so Linux CI tests it.
//
// Events sent to the page (name: payload):
//
//	setup          SetupProgress, at most ~10 a second per tool
//	setup:done     nil
//	setup:error    SetupError
//	setup:needed   nil, the tools are missing or the wrong version
//	slips:replace  Replace
//	slip:status    Status
//	run:busy       Message, Make was called while a run is going
//	run:finished   RunFinished
package appcore

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"vid2gif/internal/clip"
	"vid2gif/internal/debuglog"
	"vid2gif/internal/deps"
	"vid2gif/internal/encode"
	"vid2gif/internal/filesrv"
	"vid2gif/internal/lookup"
	"vid2gif/internal/naming"
	"vid2gif/internal/runner"
	"vid2gif/internal/settings"
)

// Emit sends one event to the page. It must not block: it is called from
// tool-output goroutines (Wails' EventsEmit only posts to the UI thread).
type Emit func(name string, data any)

type Tools struct{ YtDlp, FFmpeg, Deno string }

// Slip is one video row. A placeholder from Add has only ID and URL.
type Slip struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"`
	Title        string  `json:"title"`
	ThumbnailURL string  `json:"thumbnailUrl"`
	DurationSec  float64 `json:"durationSec"`
	Unavailable  bool    `json:"unavailable"` // private or removed playlist entry
	Reason       string  `json:"reason"`
	Failed       bool    `json:"failed"`    // the lookup failed
	Message      string  `json:"message"`   // plain sentence when Failed
	StartText    string  `json:"startText"` // from a YouTube link's ?t=, for the Start box
}

// Replace swaps the placeholder ID for Slips (a playlist gives many).
// Source is the playlist title or "Pasted links"; "" when the lookup failed.
type Replace struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Slips  []Slip `json:"slips"`
}

// PageFile is a made file as the page sees it: Path is the real file (for
// OpenFolder), URL plays it in the page ("" when it can't be served).
type PageFile struct {
	Format string `json:"format"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	URL    string `json:"url"`
}

// Status is the slip:status payload: every runner.Event field (jobId, kind,
// stage, message, pct, stagePct) plus files as PageFile, always a list.
// The outer Files shadows runner.Event.Files in JSON.
type Status struct {
	runner.Event
	Files []PageFile `json:"files"`
}

type SetupProgress struct {
	Tool  string `json:"tool"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"` // -1 when unknown
}

// SetupError: NeedsAdmin means the bin folder can't be written, so the page
// shows the Repair message instead of Try again.
type SetupError struct {
	Message    string `json:"message"`
	NeedsAdmin bool   `json:"needsAdmin"`
}

type Message struct {
	Message string `json:"message"`
}

type RunFinished struct {
	Stopped bool `json:"stopped"`
}

type BootState struct {
	NeedsSetup bool              `json:"needsSetup"`
	NeedsAdmin bool              `json:"needsAdmin"`
	Settings   settings.Settings `json:"settings"`
	Themes     []string          `json:"themes"`
	BinDir     string            `json:"binDir"` // shown read-only in Settings
}

// Times is Validate's answer: Message "" means valid; End 0 = to the end.
type Times struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Message string  `json:"message"`
}

const (
	adminMsg   = "The helper files couldn't be set up for your account. Ask an administrator to run Repair for vid2gif in Windows' installed apps, or reinstall it for just you."
	installMsg = "Couldn't download the helper files. Check your connection and try again."
	lookupMsg  = "Couldn't look this one up. Remove it and add the link again."
)

type Core struct {
	Log   *debuglog.Log
	Files *filesrv.Server
	// Header gives the first debug-log lines (app and Windows version); the
	// folders and tools follow. Written at Boot and whenever logging turns on.
	Header func() []string

	emit         Emit
	settingsPath string
	binDir       string
	pins         deps.Pins

	// The side-effecting steps, swapped in tests.
	resolve  func() (Tools, error)
	lookup   func(ctx context.Context, t Tools, link string) (string, []lookup.Video, error)
	clip     func(t Tools) runner.ClipRunner
	install  func(onProgress func(tool string, done, total int64)) error
	stat     func(string) (os.FileInfo, error)
	writable func(dir string) bool

	base    context.Context // cancelled by Shutdown
	stopAll context.CancelFunc
	sem     chan struct{} // limits concurrent lookups
	nextID  atomic.Int64
	setupMu sync.Mutex

	mu       sync.Mutex
	settings settings.Settings
	tools    *Tools             // last successful resolve
	cancel   context.CancelFunc // non-nil while a run is going
	done     chan struct{}      // closed when the latest run has ended
}

func New(emit Emit, log *debuglog.Log, settingsPath, binDir string, pins deps.Pins) *Core {
	s := settings.Load(settingsPath)
	if f := cleanFormats(s.Formats); len(f) > 0 {
		s.Formats = f
	} else {
		s.Formats = settings.Default().Formats
	}
	base, stopAll := context.WithCancel(context.Background())
	c := &Core{
		Log: log, Files: filesrv.New(),
		emit: emit, settingsPath: settingsPath, binDir: binDir, pins: pins,
		base: base, stopAll: stopAll,
		sem:      make(chan struct{}, 4),
		settings: s,
	}
	c.resolve = func() (Tools, error) {
		return ResolveTools(binDir, pins, func(s string) { log.Printf("warning: %s", s) })
	}
	c.lookup = func(ctx context.Context, t Tools, link string) (string, []lookup.Video, error) {
		return lookup.Looker{YtDlp: t.YtDlp, Deno: t.Deno, Log: log}.Lookup(ctx, link)
	}
	c.clip = func(t Tools) runner.ClipRunner {
		// The app has no console: yt-dlp's own output goes nowhere (the debug
		// log, when on, gets it through WithLog).
		return clip.New(t.YtDlp, t.FFmpeg, t.Deno, "", io.Discard).WithToolOutput(io.Discard).WithLog(log)
	}
	c.stat = os.Stat
	c.writable = canWrite
	c.install = func(on func(string, int64, int64)) error {
		return deps.InstallAllProgress(pins, binDir, logWriter{log}, on)
	}
	return c
}

// logWriter puts the installer's progress lines in the debug log.
type logWriter struct{ l *debuglog.Log }

func (w logWriter) Write(p []byte) (int, error) {
	w.l.Printf("%s", strings.TrimSpace(string(p)))
	return len(p), nil
}

// ResolveTools finds each tool in binDir, then on PATH. One in binDir must
// match its pin; one found only on PATH is the user's own and is used even if
// it doesn't, with warn (may be nil) told how it differs. The three version
// checks run at once (yt-dlp alone takes most of a second).
func ResolveTools(binDir string, pins deps.Pins, warn func(string)) (Tools, error) {
	r := deps.NewResolver(binDir)
	var p, notes [3]string
	var errs [3]error
	var wg sync.WaitGroup
	for i, name := range []string{"yt-dlp", "ffmpeg", "deno"} {
		wg.Go(func() { p[i], notes[i], errs[i] = r.ResolveOwn(name, pins[name]) })
	}
	wg.Wait()
	if err := errors.Join(errs[:]...); err != nil {
		return Tools{}, err
	}
	for _, n := range notes {
		if n != "" && warn != nil {
			warn(n)
		}
	}
	return Tools{p[0], p[1], p[2]}, nil
}

// send drops events after Shutdown: the window is going away.
func (c *Core) send(name string, data any) {
	if c.base.Err() == nil {
		c.emit(name, data)
	}
}

func (c *Core) Current() settings.Settings {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.settings
	s.Formats = slices.Clone(s.Formats)
	return s
}

func (c *Core) setTools(t Tools) {
	c.mu.Lock()
	c.tools = &t
	c.mu.Unlock()
}

// cachedTools saves Add a version check per link; Make always re-resolves.
func (c *Core) cachedTools() (Tools, error) {
	c.mu.Lock()
	t := c.tools
	c.mu.Unlock()
	if t != nil {
		return *t, nil
	}
	got, err := c.resolve()
	if err == nil {
		c.setTools(got)
	}
	return got, err
}

func (c *Core) Boot() BootState {
	s := c.Current()
	c.Files.Allow(s.OutDir)
	st := BootState{Settings: s, Themes: settings.Themes, BinDir: c.binDir}
	t, err := c.resolve()
	if err != nil {
		c.Log.Printf("tools: %v", err)
		st.NeedsSetup = true
		st.NeedsAdmin = !c.writable(c.binDir)
	} else {
		c.setTools(t)
	}
	c.logHeader()
	return st
}

// logHeader writes the lines a troubleshooting log starts with (spec, debug
// mode). A disabled log ignores them.
func (c *Core) logHeader() {
	if c.Header != nil {
		for _, l := range c.Header() {
			c.Log.Printf("%s", l)
		}
	}
	s := c.Current()
	c.Log.Printf("folders: settings %s, logs %s, tools %s, output %s", c.settingsPath, c.Log.Dir(), c.binDir, s.OutDir)
	c.mu.Lock()
	t := c.tools
	c.mu.Unlock()
	for i, name := range []string{"yt-dlp", "ffmpeg", "deno"} {
		where := "not installed or not the pinned version"
		if t != nil {
			where = "using " +[3]string{t.YtDlp, t.FFmpeg, t.Deno}[i]
		}
		c.Log.Printf("tool: %s %s (pinned), %s", name, c.pins[name].Version, where)
	}
}

// canWrite is false only for a permission error: other failures are left to
// the install to report.
func canWrite(dir string) bool {
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		var f *os.File
		if f, err = os.CreateTemp(dir, ".write-check-*"); err == nil {
			f.Close()
			os.Remove(f.Name())
		}
	}
	return !errors.Is(err, fs.ErrPermission)
}

// Install downloads the pinned tools. A second call while one runs is ignored.
func (c *Core) Install() {
	if !c.setupMu.TryLock() {
		return
	}
	defer c.setupMu.Unlock()
	on, flush := throttle(func(p SetupProgress) { c.send("setup", p) }, 100*time.Millisecond, time.Now)
	err := c.install(on)
	flush()
	if err == nil {
		var t Tools
		if t, err = c.resolve(); err == nil {
			c.setTools(t)
			c.send("setup:done", nil)
			return
		}
	}
	c.Log.Printf("install: %v", err)
	if errors.Is(err, fs.ErrPermission) && !c.writable(c.binDir) {
		c.send("setup:error", SetupError{adminMsg, true})
		return
	}
	c.send("setup:error", SetupError{installMsg, false})
}

// throttle passes on at most one event per gap per tool; the latest skipped
// one is sent when the tool changes, when done reaches total, or on flush.
// Not safe for concurrent use (the installer reports from one goroutine).
func throttle(emit func(SetupProgress), gap time.Duration, now func() time.Time) (on func(tool string, done, total int64), flush func()) {
	var pending *SetupProgress
	var last time.Time
	var tool string
	flush = func() {
		if pending != nil {
			emit(*pending)
			pending = nil
		}
	}
	on = func(name string, done, total int64) {
		p := SetupProgress{name, done, total}
		if name != tool {
			flush()
			tool, last = name, time.Time{}
		}
		if t := now(); t.Sub(last) >= gap || done == total {
			emit(p)
			pending, last = nil, t
			return
		}
		pending = &p
	}
	return on, flush
}

// Add returns placeholders at once; each lookup then sends slips:replace.
func (c *Core) Add(text string) []Slip {
	out := []Slip{}
	for _, link := range lookup.ParseInput(text) {
		p := Slip{ID: c.id(), URL: link}
		out = append(out, p)
		go c.lookupOne(p)
	}
	return out
}

func (c *Core) id() string { return "s" + strconv.FormatInt(c.nextID.Add(1), 10) }

func (c *Core) lookupOne(p Slip) {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()
	r := Replace{ID: p.ID}
	t, err := c.cachedTools()
	if err != nil {
		c.Log.Printf("lookup %s: tools: %v", p.URL, err)
		c.send("setup:needed", nil)
		r.Slips = []Slip{{ID: c.id(), URL: p.URL, Failed: true, Message: lookupMsg}}
		c.send("slips:replace", r)
		return
	}
	source, vids, err := c.lookup(c.base, t, p.URL)
	if c.base.Err() != nil {
		return
	}
	if err != nil {
		c.Log.Printf("lookup %s: %v", p.URL, err)
		r.Slips = []Slip{{ID: c.id(), URL: p.URL, Failed: true, Message: runner.PlainError(err, c.Current().DebugMode)}}
		c.send("slips:replace", r)
		return
	}
	r.Source = source
	for _, v := range vids {
		r.Slips = append(r.Slips, Slip{
			ID: c.id(), URL: v.URL, Title: v.Title, ThumbnailURL: v.ThumbnailURL,
			DurationSec: v.DurationSec, Unavailable: v.Unavailable, Reason: v.Reason,
		})
	}
	if len(r.Slips) == 1 && !r.Slips[0].Unavailable { // a playlist link's t= isn't any one video's
		r.Slips[0].StartText = lookup.StartFromLink(p.URL)
	}
	c.send("slips:replace", r)
}

func (c *Core) Validate(start, end string, endIsLength bool, durationSec float64) Times {
	s, e, msg := runner.Validate(start, end, endIsLength, durationSec)
	return Times{s, e, msg}
}

// Make starts a run unless one is going (then run:busy) or the app is
// closing. The tools are re-checked first; if they fail, setup:needed.
func (c *Core) Make(jobs []runner.Job) {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		c.send("run:busy", Message{"Clips are already being made."})
		return
	}
	if c.base.Err() != nil {
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(c.base)
	done := make(chan struct{})
	c.cancel, c.done = cancel, done
	s := c.settings
	c.mu.Unlock()
	go c.run(ctx, cancel, done, jobs, s)
}

func (c *Core) run(ctx context.Context, cancel context.CancelFunc, done chan struct{}, jobs []runner.Job, s settings.Settings) {
	defer func() {
		stopped := ctx.Err() != nil
		cancel()
		c.mu.Lock()
		c.cancel = nil
		c.mu.Unlock()
		close(done)
		c.send("run:finished", RunFinished{stopped})
	}()
	t, err := c.resolve()
	if err != nil {
		c.Log.Printf("make: tools: %v", err)
		c.send("setup:needed", nil)
		return
	}
	c.setTools(t)
	if !c.Files.Allow(s.OutDir) {
		c.Log.Printf("make: previews unavailable for output folder %q", s.OutDir)
	}
	runner.Run(ctx, c.clip(t), jobs, s, c.Log, func(e runner.Event) { c.send("slip:status", c.status(e)) })
}

func (c *Core) status(e runner.Event) Status {
	st := Status{Event: e, Files: make([]PageFile, 0, len(e.Files))}
	st.Event.Files = nil
	for _, f := range e.Files {
		u, _ := c.Files.URL(f.Path)
		st.Files = append(st.Files, PageFile{f.Format, f.Path, f.Size, u})
	}
	return st
}

// Stop cancels the current run, if any; the run then sends run:finished.
func (c *Core) Stop() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
}

// Shutdown cancels everything and waits up to wait for the run to clean up
// its temp folder.
func (c *Core) Shutdown(wait time.Duration) {
	c.stopAll()
	c.mu.Lock()
	done := c.done
	c.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(wait):
	}
}

// SaveSettings checks s, keeps only known formats once each, writes it to
// disk and only then applies it (debug log switch, preview folder), so a
// failed save never leaves Make using settings the window doesn't show.
func (c *Core) SaveSettings(s settings.Settings) error {
	if !slices.Contains(settings.Themes, s.Theme) {
		return errors.New("That colour isn't available.")
	}
	if _, ok := encode.Presets[s.Quality]; !ok {
		return errors.New("That quality isn't available.")
	}
	if s.Formats = cleanFormats(s.Formats); len(s.Formats) == 0 {
		return errors.New("Pick at least one format.")
	}
	if !settings.ValidOutDir(s.OutDir, filepath.Dir(c.settingsPath)) {
		c.Log.Printf("settings: refused output folder %q", s.OutDir)
		return errors.New("That folder can't be used. Pick another one.")
	}
	if err := s.Save(c.settingsPath); err != nil {
		c.Log.Printf("settings: %v", err)
		return errors.New("Couldn't save your settings.")
	}
	c.mu.Lock()
	turnedOn := s.DebugMode && !c.settings.DebugMode
	c.settings = s
	c.mu.Unlock()
	c.Log.SetEnabled(s.DebugMode)
	c.Files.Allow(s.OutDir)
	if turnedOn {
		c.logHeader()
	}
	c.Log.Printf("settings: %+v", s)
	return nil
}

func cleanFormats(in []string) []string {
	out := []string{}
	for _, f := range in {
		f = strings.ToLower(strings.TrimSpace(f))
		if slices.Contains(naming.AllFormats, f) && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// OpenablePath decides whether OpenFolder may show p: the output folder or
// anything under it, a made file (one the preview server issued a URL for),
// or the helper-files folder. Titles reach the page, so p is untrusted: UNC
// and device paths (\\host, \\?\, \\.\) and relative paths are refused before
// anything touches the disk (a stat of a UNC path already connects to it).
func (c *Core) OpenablePath(p string) (path string, isDir, ok bool) {
	if p == "" || !settings.LocalAbs(p) {
		return "", false, false
	}
	p = filepath.Clean(p)
	made := c.Files.Issued(p)
	if !made && !within(c.Current().OutDir, p) && !within(c.binDir, p) {
		return "", false, false
	}
	fi, err := c.stat(p)
	if err != nil {
		return "", false, false
	}
	return p, fi.IsDir(), true
}

// within reports whether p is dir or below it, lexically (filepath.Rel is
// case-insensitive on Windows).
func within(dir, p string) bool {
	if dir == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(dir), p)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
