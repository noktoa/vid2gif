// Package runner makes a queue of clips one after another and turns the
// pipeline's events into the slip states the app shows.
package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"vid2gif/internal/clip"
	"vid2gif/internal/debuglog"
	"vid2gif/internal/naming"
	"vid2gif/internal/settings"
)

// Job is one slip to make. DurationSec is the whole video's length (0 = unknown).
type Job struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Start       float64 `json:"start"`
	End         float64 `json:"end"`
	DurationSec float64 `json:"durationSec"`
}

type File struct {
	Format string `json:"format"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
}

// Event is one slip update. Kind is "making" (Stage is find, download or a
// format; Pct is the overall 0..1, -1 when the clip length is unknown;
// StagePct is the stage's own 0..1, -1 when unmeasured), "done", "partial"
// (Message names the failed format), "failed", "already" or "ready"
// (cancelled, back to waiting).
type Event struct {
	JobID    string  `json:"jobId"`
	Kind     string  `json:"kind"`
	Stage    string  `json:"stage"`
	Message  string  `json:"message"`
	Pct      float64 `json:"pct"`
	StagePct float64 `json:"stagePct"`
	Files    []File  `json:"files"`
}

type ClipRunner interface {
	Run(ctx context.Context, o clip.Opts) error
}

var now = time.Now // replaced by tests

// minGap throttles making events to about 10 a second per job.
const minGap = 100 * time.Millisecond

// Run makes jobs in order with the settings' formats, quality, folder and
// download size. A failed job never stops the next; a cancel marks the
// current job ready and leaves the rest untouched. emit is called from the
// pipeline's tool-output goroutines too, so it must not block. log may be nil.
func Run(ctx context.Context, cr ClipRunner, jobs []Job, s settings.Settings, log *debuglog.Log, emit func(Event)) {
	formats := pickFormats(s.Formats)
	for _, j := range jobs {
		if ctx.Err() != nil {
			return
		}
		// clip.Run would create the folder itself; checking here gives a path
		// that is a file (or under one) the folder message on every OS.
		if err := os.MkdirAll(s.OutDir, 0o755); err != nil {
			log.Printf("job %s: output folder: %v", j.ID, err)
			emit(Event{JobID: j.ID, Kind: "failed", Message: folderMsg, Pct: -1, StagePct: -1})
			continue
		}
		runJob(ctx, cr, j, s, formats, log, emit)
	}
}

func runJob(ctx context.Context, cr ClipRunner, j Job, s settings.Settings, formats []string, log *debuglog.Log, emit func(Event)) {
	length := clipLen(j)
	p := newProgress(s, formats, length)
	var wrote, skipped []File
	var failed []string
	var formatErr error // first StageFailed error: clip returns only "N of M outputs failed"

	// OnEvent runs under clip's mutex: only cheap bookkeeping and emit here.
	var pending *Event
	var lastAt time.Time
	lastStage := ""
	making := func(stage string, stagePct float64) {
		e := Event{JobID: j.ID, Kind: "making", Stage: stage, Pct: p.at(stage, stagePct), StagePct: capStage(stagePct)}
		t := now()
		if stage != lastStage || t.Sub(lastAt) >= minGap {
			emit(e)
			pending, lastAt, lastStage = nil, t, stage
			return
		}
		pending = &e
	}
	onEvent := func(e clip.Event) {
		format := strings.TrimPrefix(filepath.Ext(e.Path), ".")
		switch e.Stage {
		case clip.StageFind, clip.StageDownload:
			making(e.Stage, e.Pct)
		case clip.StageEncode:
			making(format, e.Pct)
		case clip.StageWrote:
			p.finish(format)
			wrote = append(wrote, File{format, e.Path, e.Size})
		case clip.StageSkipped:
			p.skip(format)
			skipped = append(skipped, File{Format: format, Path: e.Path})
		case clip.StageFailed:
			if formatErr == nil {
				formatErr = e.Err
			}
			if format != "" {
				p.finish(format)
				failed = append(failed, strings.ToUpper(format))
			}
		}
	}

	err := cr.Run(ctx, clip.Opts{
		URL: j.URL, Start: j.Start, End: j.End,
		Formats:     formats,
		OutDir:      s.OutDir,
		Name:        naming.BuildName(j.Title, j.Start, j.End, s.Quality),
		Quality:     s.Quality,
		FullSource:  s.FullSource,
		OnEvent:     onEvent,
		DurationSec: length,
	})
	// clip.Run has returned, so onEvent is no longer called: flush the last.
	if pending != nil {
		emit(*pending)
	}

	done := Event{JobID: j.ID, Pct: -1, StagePct: -1}
	switch {
	case err != nil && (errors.Is(err, context.Canceled) || ctx.Err() != nil):
		done.Kind = "ready"
	case err != nil:
		log.Printf("job %s: %v", j.ID, err)
		if formatErr != nil && formatErr != err {
			log.Printf("job %s: %v", j.ID, formatErr)
			err = formatErr // the real cause, for PlainError
		}
		if len(wrote) > 0 {
			done.Kind, done.Files = "partial", wrote
			done.Message = PlainError(err, s.DebugMode)
			if len(failed) > 0 {
				done.Message = "The " + strings.Join(failed, " and ") + " couldn't be made."
			}
		} else {
			done.Kind, done.Message = "failed", PlainError(err, s.DebugMode)
		}
	default:
		for i := range skipped {
			if st, err := os.Stat(skipped[i].Path); err == nil {
				skipped[i].Size = st.Size()
			}
		}
		done.Kind, done.Pct, done.Files = "done", 1, append(wrote, skipped...)
		if len(wrote) == 0 && len(skipped) > 0 {
			done.Kind = "already"
		}
	}
	emit(done)
}

// capStage keeps a measured stage label below 100% (a stage can report 1
// before the clip is done, e.g. the mp4 faststart rewrite); -1 stays -1.
func capStage(f float64) float64 {
	if f < 0 {
		return -1
	}
	return min(f, 0.99)
}

// pickFormats keeps known formats once each, in order; none left means gif.
func pickFormats(in []string) []string {
	var out []string
	for _, f := range in {
		f = strings.ToLower(strings.TrimSpace(f))
		if slices.Contains(naming.AllFormats, f) && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return []string{"gif"}
	}
	return out
}

// clipLen is Stop minus Start, or the video's length minus Start when Stop is
// blank; 0 when unknown.
func clipLen(j Job) float64 {
	if j.End > 0 {
		return j.End - j.Start
	}
	if j.DurationSec > j.Start {
		return j.DurationSec - j.Start
	}
	return 0
}

// progress turns stage fractions into one overall fraction: (finished stage
// costs + current fraction × current cost) ÷ total cost.
type progress struct {
	stages []string
	cost   map[string]float64
	idx    int     // current stage
	frac   float64 // of the current stage; held at 0 while unmeasured
	last   float64 // overall, never decreases
	known  bool
}

func newProgress(s settings.Settings, formats []string, length float64) *progress {
	dl := "download"
	if s.FullSource || slices.ContainsFunc(formats, func(f string) bool { return f == "mp4" || f == "mov" }) {
		dl = "download-full"
	}
	p := &progress{
		stages: append([]string{"find", "download"}, formats...),
		cost:   map[string]float64{"find": findCost, "download": perSecond[s.Quality+"/"+dl] * length},
		known:  length > 0,
	}
	for _, f := range formats {
		p.cost[f] = perSecond[s.Quality+"/"+f] * length
	}
	return p
}

// at records that stage reported frac (-1 = unmeasured) and returns the
// overall fraction: -1 when the length is unknown, else below 1 (only "done"
// is 100%) and never lower than before. An absent stage is simply passed.
func (p *progress) at(stage string, frac float64) float64 {
	if i := slices.Index(p.stages, stage); i > p.idx {
		p.idx, p.frac = i, 0
	}
	if p.idx < len(p.stages) && p.stages[p.idx] == stage && frac >= 0 {
		p.frac = max(p.frac, min(frac, 1))
	}
	return p.overall()
}

func (p *progress) overall() float64 {
	total, done := 0.0, 0.0
	for i, s := range p.stages {
		total += p.cost[s]
		if i < p.idx {
			done += p.cost[s]
		} else if i == p.idx {
			done += p.frac * p.cost[s]
		}
	}
	if !p.known || total <= 0 {
		return -1
	}
	// ponytail: 0.99 cap keeps the bar off 100% until done; finer cap not needed for a 1-px bar.
	p.last = max(p.last, min(done/total, 0.99))
	return p.last
}

// finish marks a format's stage complete (written or failed).
func (p *progress) finish(format string) {
	if i := slices.Index(p.stages, format); i >= p.idx {
		p.idx, p.frac = i+1, 0
	}
}

// skip drops an already-made format from the run's cost.
func (p *progress) skip(format string) {
	if i := slices.Index(p.stages, format); i > p.idx {
		p.stages = slices.Delete(p.stages, i, i+1)
	}
}
