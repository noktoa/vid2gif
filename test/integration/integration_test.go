//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"vid2gif/internal/deps"
	"vid2gif/internal/encode"
	"vid2gif/internal/naming"
)

func checkSignature(t *testing.T, path, format string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || len(b) < 12 {
		t.Fatalf("%s: unreadable or too small (%v, %d bytes)", path, err, len(b))
	}
	switch format {
	case "gif":
		if !bytes.HasPrefix(b, []byte("GIF89a")) {
			t.Errorf("%s: not a GIF89a", path)
		}
	case "webp":
		if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
			t.Errorf("%s: not a WebP", path)
		}
	case "mp4", "mov":
		if string(b[4:8]) != "ftyp" {
			t.Errorf("%s: no ftyp box", path)
		}
	}
}

// Installs the pinned tools into a temp dir, checks the encoders the tool
// relies on exist, then encodes real sources to every format.
func TestRealToolsEncode(t *testing.T) {
	d, err := deps.Load("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	for _, name := range []string{"yt-dlp", "ffmpeg"} {
		if err := deps.Install(name, d[name], binDir, os.Stdout); err != nil {
			t.Fatalf("install %s: %v", name, err)
		}
	}
	ffmpeg := filepath.Join(binDir, deps.ExeName("ffmpeg"))

	encoders, err := exec.Command(ffmpeg, "-hide_banner", "-encoders").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, enc := range []string{"libx264", "libwebp_anim"} {
		if !bytes.Contains(encoders, []byte(enc)) {
			t.Errorf("pinned ffmpeg lacks %s", enc)
		}
	}

	work := t.TempDir()
	mk := func(args ...string) {
		t.Helper()
		full := append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)
		if out, err := exec.Command(ffmpeg, full...).CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg %v: %v\n%s", args, err, out)
		}
	}
	withAudio := filepath.Join(work, "audio.mp4")
	mk("-f", "lavfi", "-i", "testsrc=duration=3:size=320x240:rate=15", "-f", "lavfi", "-i", "sine=duration=3",
		"-shortest", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", withAudio)
	// Odd dimensions and no audio track: the two cases mp4/mov must survive.
	oddNoAudio := filepath.Join(work, "odd.mkv")
	mk("-f", "lavfi", "-i", "testsrc=duration=3:size=321x241:rate=15", "-c:v", "ffv1", "-pix_fmt", "yuv444p", oddNoAudio)

	enc := encode.Runner(ffmpeg)
	for _, src := range []string{withAudio, oddNoAudio} {
		for _, format := range naming.AllFormats {
			src, format := src, format
			t.Run(fmt.Sprintf("%s-%s", filepath.Base(src), format), func(t *testing.T) {
				out := filepath.Join(work, filepath.Base(src)+"."+format)
				if err := enc(context.Background(), src, out, encode.Opts{Format: format, Preset: encode.Presets["medium"]}); err != nil {
					t.Fatal(err)
				}
				checkSignature(t, out, format)
			})
		}
	}
}
