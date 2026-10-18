package encode

import (
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
	got := Args("in.mkv", "out.gif", Opts{"gif", Presets["medium"]})
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
	vf := argAfter(Args("in", "out.gif", Opts{"gif", Presets["high"]}), "-vf")
	want := "fps=24,scale=w='min(720,iw)':h=-1:flags=lanczos,split[a][b];[a]palettegen[p];[b][p]paletteuse=dither=sierra2_4a"
	if vf != want {
		t.Errorf("vf = %q; want %q", vf, want)
	}
}

func TestFfmpegArgsWebp(t *testing.T) {
	got := Args("in", "out.webp", Opts{"webp", Presets["high"]})
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
	got := Args("in.mkv", "out.mp4", Opts{"mp4", Presets["medium"]})
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
	got := Args("in", "out.mov", Opts{"mov", Presets["low"]})
	if argAfter(got, "-crf") != "28" {
		t.Errorf("crf = %q; want 28", argAfter(got, "-crf"))
	}
}
