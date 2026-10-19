// Package progress turns a clip's stage events into one overall fraction and
// a stage label, the same way for the desktop app and the CLI.
package progress

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"vid2gif/internal/clip"
)

// Tracker turns stage fractions into one overall fraction: (finished stage
// costs + current fraction × current cost) ÷ total cost.
type Tracker struct {
	stages []string
	cost   map[string]float64
	idx    int     // current stage
	frac   float64 // of the current stage; held at 0 while unmeasured
	last   float64 // overall, never decreases
	known  bool
}

// New sizes the stages from the cost table. length is the clip's length in
// seconds; 0 (unknown) makes every overall fraction -1.
func New(quality string, formats []string, fullSource bool, length float64) *Tracker {
	dl := "download"
	if fullSource || slices.ContainsFunc(formats, func(f string) bool { return f == "mp4" || f == "mov" }) {
		dl = "download-full"
	}
	t := &Tracker{
		stages: append([]string{"find", "download"}, formats...),
		cost:   map[string]float64{"find": findCost, "download": perSecond[quality+"/"+dl] * length},
		known:  length > 0,
	}
	for _, f := range formats {
		t.cost[f] = perSecond[quality+"/"+f] * length
	}
	return t
}

// Apply feeds one clip event. ok is true for a making stage (find, download,
// or a format being encoded): stage is its name, stagePct its own fraction
// capped below 1 (-1 = unmeasured) and overall the run's (see At). Written,
// skipped and failed formats only move the tracker on.
func (t *Tracker) Apply(e clip.Event) (stage string, stagePct, overall float64, ok bool) {
	format := strings.TrimPrefix(filepath.Ext(e.Path), ".")
	switch e.Stage {
	case clip.StageFind, clip.StageDownload:
		stage = e.Stage
	case clip.StageEncode:
		stage = format
	case clip.StageWrote:
		t.Finish(format)
		return
	case clip.StageSkipped:
		t.Skip(format)
		return
	case clip.StageFailed:
		if format != "" {
			t.Finish(format)
		}
		return
	default:
		return
	}
	return stage, CapStage(e.Pct), t.At(stage, e.Pct), true
}

// At records that stage reported frac (-1 = unmeasured) and returns the
// overall fraction: -1 when the length is unknown, else below 1 (only "done"
// is 100%) and never lower than before. An absent stage is simply passed.
func (t *Tracker) At(stage string, frac float64) float64 {
	if i := slices.Index(t.stages, stage); i > t.idx {
		t.idx, t.frac = i, 0
	}
	if t.idx < len(t.stages) && t.stages[t.idx] == stage && frac >= 0 {
		t.frac = max(t.frac, min(frac, 1))
	}
	return t.Overall()
}

// Overall is the current overall fraction (see At).
func (t *Tracker) Overall() float64 {
	total, done := 0.0, 0.0
	for i, s := range t.stages {
		total += t.cost[s]
		if i < t.idx {
			done += t.cost[s]
		} else if i == t.idx {
			done += t.frac * t.cost[s]
		}
	}
	if !t.known || total <= 0 {
		return -1
	}
	// ponytail: 0.99 cap keeps the bar off 100% until done; finer cap not needed for a 1-px bar.
	t.last = max(t.last, min(done/total, 0.99))
	return t.last
}

// Finish marks a format's stage complete (written or failed).
func (t *Tracker) Finish(format string) {
	if i := slices.Index(t.stages, format); i >= t.idx {
		t.idx, t.frac = i+1, 0
	}
}

// Skip drops an already-made format from the run's cost.
func (t *Tracker) Skip(format string) {
	if i := slices.Index(t.stages, format); i > t.idx {
		t.stages = slices.Delete(t.stages, i, i+1)
	}
}

// CapStage keeps a measured stage label below 100% (a stage can report 1
// before the clip is done, e.g. the mp4 faststart rewrite); -1 stays -1.
func CapStage(f float64) float64 {
	if f < 0 {
		return -1
	}
	return min(f, 0.99)
}

var stageNames = map[string]string{
	"find": "Finding the video", "download": "Getting the clip",
	"gif": "Making GIF", "webp": "Making WebP", "mp4": "Making MP4", "mov": "Making MOV",
}

// Label is the stage label the app shows (frontend/state.js stageLabel): a
// percentage only for a measured stage (stagePct >= 0), never 100%.
func Label(stage string, stagePct float64) string {
	name, ok := stageNames[stage]
	if !ok {
		name = "Working on it"
	}
	if !(stagePct >= 0) {
		return name + "…"
	}
	return fmt.Sprintf("%s %d%%", name, min(99, int(math.Round(stagePct*100))))
}
