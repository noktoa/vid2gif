package progress

import (
	"errors"
	"math"
	"strings"
	"testing"

	"vid2gif/internal/clip"
	"vid2gif/internal/encode"
	"vid2gif/internal/naming"
)

func TestCostsCoverEveryStage(t *testing.T) {
	if findCost <= 0 {
		t.Error("no find cost")
	}
	for q := range encode.Presets {
		for _, st := range append([]string{"download", "download-full"}, naming.AllFormats...) {
			if perSecond[q+"/"+st] <= 0 {
				t.Errorf("no cost for %s/%s", q, st)
			}
		}
	}
}

// run is a typical gif+mp4 run with wobbly stage fractions.
var run = []clip.Event{
	{Stage: clip.StageFind, Pct: -1},
	{Stage: clip.StageDownload, Pct: 0.2},
	{Stage: clip.StageDownload, Pct: 0.6},
	{Stage: clip.StageDownload, Pct: 1},
	{Stage: clip.StageEncode, Pct: -1, Path: "o/a.gif"},
	{Stage: clip.StageWrote, Pct: 1, Path: "o/a.gif", Size: 1},
	{Stage: clip.StageEncode, Pct: 0, Path: "o/a.mp4"},
	{Stage: clip.StageEncode, Pct: 0.5, Path: "o/a.mp4"},
	{Stage: clip.StageEncode, Pct: 0.4, Path: "o/a.mp4"},
	{Stage: clip.StageEncode, Pct: 1, Path: "o/a.mp4"},
	{Stage: clip.StageWrote, Pct: 1, Path: "o/a.mp4", Size: 1},
}

func TestApplyMapsStagesAndNeverDecreases(t *testing.T) {
	tr := New("medium", []string{"gif", "mp4"}, false, 10)
	var stages []string
	prev := 0.0
	for _, e := range run {
		stage, sp, overall, ok := tr.Apply(e)
		if !ok {
			continue
		}
		stages = append(stages, stage)
		if overall < prev || overall >= 1 {
			t.Fatalf("%+v: overall %v (prev %v), want non-decreasing and below 1", e, overall, prev)
		}
		if sp > 0.99 {
			t.Fatalf("%+v: stage pct %v, want <= 0.99", e, sp)
		}
		prev = overall
	}
	if got := strings.Join(stages, ","); got != "find,download,download,download,gif,mp4,mp4,mp4,mp4" {
		t.Errorf("stages = %s", got)
	}
	if prev <= 0.5 {
		t.Errorf("last overall = %v, want most of the bar", prev)
	}
	if o := tr.Overall(); o >= 1 || o < prev {
		t.Errorf("Overall after the last write = %v, want in [%v, 1)", o, prev)
	}
}

func TestApplyUnknownLengthIsMinus1(t *testing.T) {
	tr := New("medium", []string{"gif", "mp4"}, false, 0)
	for _, e := range run {
		if _, _, overall, ok := tr.Apply(e); ok && overall != -1 {
			t.Fatalf("%+v: overall %v, want -1", e, overall)
		}
	}
}

func TestApplyUnmeasuredStageIsMinus1(t *testing.T) {
	tr := New("medium", []string{"gif"}, false, 10)
	if _, sp, _, _ := tr.Apply(clip.Event{Stage: clip.StageEncode, Pct: -1, Path: "a.gif"}); sp != -1 {
		t.Errorf("gif stage pct = %v, want -1", sp)
	}
}

func TestApplyIgnoresResultEvents(t *testing.T) {
	tr := New("medium", []string{"gif"}, false, 10)
	for _, e := range []clip.Event{
		{Stage: clip.StageWrote, Pct: 1, Path: "a.gif"},
		{Stage: clip.StageSkipped, Pct: 1, Path: "a.gif"},
		{Stage: clip.StageFailed, Pct: -1, Err: errors.New("x")},
	} {
		if _, _, _, ok := tr.Apply(e); ok {
			t.Errorf("%s reported as a making stage", e.Stage)
		}
	}
}

func TestSkipDropsFormatCost(t *testing.T) {
	a := New("medium", []string{"gif", "mp4"}, false, 10)
	b := New("medium", []string{"gif", "mp4"}, false, 10)
	b.Skip("mp4")
	a.At("download", 1)
	b.At("download", 1)
	if a.Overall() >= b.Overall() {
		t.Errorf("with mp4 skipped the download is a bigger share: %v vs %v", a.Overall(), b.Overall())
	}
}

func TestFullSourceUsesFullDownloadCost(t *testing.T) {
	small := New("medium", []string{"gif"}, false, 10)
	full := New("medium", []string{"gif"}, true, 10)
	small.At("download", 1)
	full.At("download", 1)
	if full.Overall() <= small.Overall() {
		t.Errorf("full-source download share %v, want more than %v", full.Overall(), small.Overall())
	}
}

func TestOverallCappedBelow1(t *testing.T) {
	tr := New("medium", []string{"gif"}, false, 10)
	tr.At("gif", 1)
	tr.Finish("gif")
	if o := tr.Overall(); o != 0.99 {
		t.Errorf("Overall = %v, want 0.99 until done", o)
	}
}

func TestLabel(t *testing.T) {
	for _, c := range []struct {
		stage string
		pct   float64
		want  string
	}{
		{"find", -1, "Finding the video…"},
		{"download", 0.63, "Getting the clip 63%"},
		{"gif", -1, "Making GIF…"},
		{"mp4", 0.4, "Making MP4 40%"},
		{"webp", -1, "Making WebP…"},
		{"mov", 0.29, "Making MOV 29%"},
		{"bogus", -1, "Working on it…"},
	} {
		if got := Label(c.stage, c.pct); got != c.want {
			t.Errorf("Label(%q, %v) = %q, want %q", c.stage, c.pct, got, c.want)
		}
	}
}

func TestLabelNeverUnmeasuredPctOr100(t *testing.T) {
	for _, stage := range []string{"find", "download", "gif", "webp", "mp4", "mov"} {
		for _, pct := range []float64{-1, math.NaN()} {
			if l := Label(stage, pct); strings.Contains(l, "%") || !strings.HasSuffix(l, "…") {
				t.Errorf("Label(%q, %v) = %q, want no number", stage, pct, l)
			}
		}
		for _, pct := range []float64{1, 0.999, 2} {
			if l := Label(stage, pct); strings.Contains(l, "100%") {
				t.Errorf("Label(%q, %v) = %q, want below 100%%", stage, pct, l)
			}
		}
	}
}
