// Package encode builds ffmpeg arguments for each output format and runs them.
package encode

import (
	"context"
	"fmt"
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

type Opts struct {
	Format string
	Preset Preset
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
	return append(args, out)
}

// Runner returns a function that encodes in to out with the given ffmpeg
// binary; ffmpeg's own output is included in the error when it fails.
func Runner(ffmpeg string) func(ctx context.Context, in, out string, o Opts) error {
	return func(ctx context.Context, in, out string, o Opts) error {
		b, err := exec.CommandContext(ctx, ffmpeg, Args(in, out, o)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ffmpeg: %w\n%s", err, strings.TrimSpace(string(b)))
		}
		return nil
	}
}
