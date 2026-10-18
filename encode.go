package main

import (
	"fmt"
	"strconv"
)

type preset struct {
	fps, width  int
	dither      string
	webpQuality int
	crf         int
}

var presets = map[string]preset{
	"low":    {10, 320, "bayer", 50, 28},
	"medium": {15, 480, "bayer", 70, 23},
	"high":   {24, 720, "sierra2_4a", 90, 18},
}

type encodeOpts struct {
	format string
	preset preset
}

// ffmpegArgs builds the ffmpeg argv (without the program name) to encode the
// already-trimmed clip `in` to `out`. The single quotes inside -vf are
// ffmpeg filter-graph quoting, so the commas in min() survive.
func ffmpegArgs(in, out string, o encodeOpts) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", in}
	switch o.format {
	case "gif", "webp":
		vf := fmt.Sprintf("fps=%d,scale=w='min(%d,iw)':h=-1:flags=lanczos", o.preset.fps, o.preset.width)
		if o.format == "gif" {
			vf += ",split[a][b];[a]palettegen[p];[b][p]paletteuse=dither=" + o.preset.dither
			args = append(args, "-an", "-vf", vf, "-loop", "0")
		} else {
			args = append(args, "-an", "-vf", vf, "-c:v", "libwebp_anim",
				"-quality", strconv.Itoa(o.preset.webpQuality), "-loop", "0")
		}
	case "mp4", "mov":
		// yuv420p needs even dimensions
		args = append(args, "-c:v", "libx264", "-crf", strconv.Itoa(o.preset.crf),
			"-preset", "medium", "-pix_fmt", "yuv420p", "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
			"-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart")
	}
	return append(args, out)
}
