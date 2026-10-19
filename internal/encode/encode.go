// Package encode builds ffmpeg arguments for each output format and runs them.
package encode

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type Preset struct {
	fps, width  int
	dither      string
	webpQuality int
	crf         int
}

var Presets = map[string]Preset{
	"low":    {10, 320, "bayer", 50, 28},
	"medium": {15, 480, "bayer", 70, 23},
	"high":   {24, 720, "sierra2_4a", 90, 18},
}

// PresetWidth is the output width of a quality (0 if unknown): the clip is
// never scaled above it, so a source narrower than this is wasted download.
func PresetWidth(quality string) int { return Presets[quality].width }

type Opts struct {
	Format string
	Preset Preset

	// OnProgress, if set, makes ffmpeg report progress on stdout (read it with
	// NewProgressWriter). Only worth setting for mp4/mov: gif and webp encodes
	// report nothing usable. DurationSec is the clip length.
	OnProgress  func(frac float64)
	DurationSec float64
}

// Args builds the ffmpeg argv (without the program name) to encode the
// already-trimmed clip `in` to `out`. The single quotes inside -vf are
// ffmpeg filter-graph quoting, so the commas in min() survive.
func Args(in, out string, o Opts) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", in}
	switch o.Format {
	case "gif", "webp":
		vf := fmt.Sprintf("fps=%d,scale=w='min(%d,iw)':h=-1:flags=lanczos", o.Preset.fps, o.Preset.width)
		if o.Format == "gif" {
			vf += ",split[a][b];[a]palettegen[p];[b][p]paletteuse=dither=" + o.Preset.dither
			args = append(args, "-an", "-vf", vf, "-loop", "0")
		} else {
			args = append(args, "-an", "-vf", vf, "-c:v", "libwebp_anim",
				"-quality", strconv.Itoa(o.Preset.webpQuality), "-loop", "0")
		}
	case "mp4", "mov":
		// yuv420p needs even dimensions
		args = append(args, "-c:v", "libx264", "-crf", strconv.Itoa(o.Preset.crf),
			"-preset", "medium", "-pix_fmt", "yuv420p", "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
			"-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart")
	}
	if o.OnProgress != nil {
		args = append(args, "-progress", "pipe:1", "-nostats")
	}
	return append(args, out)
}

// ParseProgress reads one line of ffmpeg's -progress output. It is ok only
// for out_time_us=<n> with a known duration; the fraction is clamped to 0..1.
func ParseProgress(line string, durationSec float64) (frac float64, ok bool) {
	v, found := strings.CutPrefix(strings.TrimSpace(line), "out_time_us=")
	us, err := strconv.ParseInt(v, 10, 64)
	if !found || err != nil || durationSec <= 0 {
		return 0, false
	}
	return min(max(float64(us)/1e6/durationSec, 0), 1), true
}

// NewProgressWriter returns a writer for ffmpeg's -progress stdout that calls
// fn with the clip fraction, never lower than the one before (ffmpeg steps
// back about 1 s once per MP4 encode). fn runs on the goroutine that writes.
func NewProgressWriter(durationSec float64, fn func(frac float64)) io.Writer {
	return &progressWriter{dur: durationSec, fn: fn}
}

type progressWriter struct {
	dur  float64
	fn   func(float64)
	buf  []byte
	last float64
}

func (w *progressWriter) Write(b []byte) (int, error) {
	w.buf = append(w.buf, b...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			return len(b), nil
		}
		if f, ok := ParseProgress(string(w.buf[:i]), w.dur); ok {
			w.last = max(w.last, f)
			w.fn(w.last)
		}
		w.buf = w.buf[i+1:]
	}
}

// Runner returns a function that encodes in to out with the given ffmpeg
// binary; ffmpeg's own output is included in the error when it fails.
func Runner(ffmpeg string) func(ctx context.Context, in, out string, o Opts) error {
	return func(ctx context.Context, in, out string, o Opts) error {
		cmd := exec.CommandContext(ctx, ffmpeg, Args(in, out, o)...)
		var b bytes.Buffer
		cmd.Stdout, cmd.Stderr = &b, &b // like CombinedOutput
		if o.OnProgress != nil {
			cmd.Stdout = NewProgressWriter(o.DurationSec, o.OnProgress)
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ffmpeg: %w\n%s", err, strings.TrimSpace(b.String()))
		}
		return nil
	}
}
