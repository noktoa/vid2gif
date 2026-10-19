package encode

import (
	"io"
	"sort"
	"slices"
	"testing"
)

func argAfter(args []string, key string) string {
	for i, a := range args {
		if a == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestFfmpegArgsGifMedium(t *testing.T) {
	got := Args("in.mkv", "out.gif", Opts{Format: "gif", Preset: Presets["medium"]})
	want := []string{
		"-hide_banner", "-loglevel", "error", "-y", "-i", "in.mkv", "-an",
		"-vf", "fps=15,scale=w='min(480,iw)':h=-1:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse=dither=bayer",
		"-loop", "0", "out.gif",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestFfmpegArgsGifHigh(t *testing.T) {
	vf := argAfter(Args("in", "out.gif", Opts{Format: "gif", Preset: Presets["high"]}), "-vf")
	want := "fps=24,scale=w='min(720,iw)':h=-1:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse=dither=sierra2_4a"
	if vf != want {
		t.Errorf("vf = %q; want %q", vf, want)
	}
}

func TestFfmpegArgsWebp(t *testing.T) {
	got := Args("in", "out.webp", Opts{Format: "webp", Preset: Presets["high"]})
	if argAfter(got, "-vf") != "fps=24,scale=w='min(720,iw)':h=-1:flags=lanczos" {
		t.Errorf("vf = %q", argAfter(got, "-vf"))
	}
	if argAfter(got, "-c:v") != "libwebp_anim" || argAfter(got, "-quality") != "90" || argAfter(got, "-loop") != "0" {
		t.Errorf("bad webp args: %q", got)
	}
	if !slices.Contains(got, "-an") {
		t.Error("webp must drop audio")
	}
}

func TestFfmpegArgsMp4KeepsSource(t *testing.T) {
	got := Args("in.mkv", "out.mp4", Opts{Format: "mp4", Preset: Presets["medium"]})
	want := []string{
		"-hide_banner", "-loglevel", "error", "-y", "-i", "in.mkv",
		"-c:v", "libx264", "-crf", "23", "-preset", "medium", "-pix_fmt", "yuv420p",
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", "out.mp4",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestFfmpegArgsMovLowCRF(t *testing.T) {
	got := Args("in", "out.mov", Opts{Format: "mov", Preset: Presets["low"]})
	if argAfter(got, "-crf") != "28" {
		t.Errorf("crf = %q; want 28", argAfter(got, "-crf"))
	}
}

func TestParseProgressOutTime(t *testing.T) {
	if f, ok := ParseProgress("out_time_us=2500000", 5); !ok || f != 0.5 {
		t.Errorf("got %v, %v; want 0.5", f, ok)
	}
	if _, ok := ParseProgress("out_time_us=2500000", 0); ok {
		t.Error("unknown duration must not be ok")
	}
	if f, ok := ParseProgress("out_time_us=9000000", 5); !ok || f != 1 {
		t.Errorf("above 1: got %v, %v; want 1", f, ok)
	}
	for _, line := range []string{"out_time_us=N/A", "frame=3", "progress=continue", ""} {
		if _, ok := ParseProgress(line, 5); ok {
			t.Errorf("%q must not be ok", line)
		}
	}
}

// Measured 2026-10-19: ffmpeg steps back about 1 s once per MP4 encode.
func TestEncodeFractionNeverDecreases(t *testing.T) {
	var got []float64
	w := NewProgressWriter(60, func(f float64) { got = append(got, f) })
	// split mid-line to cover buffering across writes
	io.WriteString(w, "frame=1\nout_time_us=55000000\nprogress=continue\nout_time_us=54000")
	io.WriteString(w, "000\nout_time_us=56000000\n")
	if len(got) != 3 || !sort.Float64sAreSorted(got) || got[0] != 55.0/60 || got[2] != 56.0/60 {
		t.Errorf("fractions = %v", got)
	}
}

func TestArgsUnchangedWithoutProgress(t *testing.T) {
	for _, f := range []string{"gif", "webp", "mp4", "mov"} {
		if slices.Contains(Args("in", "out."+f, Opts{Format: f, Preset: Presets["medium"]}), "-progress") {
			t.Errorf("%s: no OnProgress, but -progress was added", f)
		}
	}
}

func TestRunnerAddsProgressFlags(t *testing.T) {
	o := Opts{Format: "mp4", Preset: Presets["medium"], OnProgress: func(float64) {}}
	got := Args("in.mkv", "out.mp4", o)
	n := len(got)
	if !slices.Equal(got[n-4:], []string{"-progress", "pipe:1", "-nostats", "out.mp4"}) {
		t.Errorf("tail = %q; want progress flags before the output path", got[n-4:])
	}
}
