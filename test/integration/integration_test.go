//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

var decodedTime = regexp.MustCompile(`time=(\d+):(\d+):(\d+\.\d+)`)

// checkDuration decodes the output with ffmpeg and compares its playback
// length with the source. Lowering the frame rate for GIF/WebP must drop
// frames, not change how fast the clip plays.
func checkDuration(t *testing.T, ffmpeg, path string, want float64) {
	t.Helper()
	out, _ := exec.Command(ffmpeg, "-hide_banner", "-i", path, "-f", "null", "-").CombinedOutput()
	m := decodedTime.FindAllStringSubmatch(string(out), -1)
	if len(m) == 0 {
		t.Fatalf("%s: no decoded time in ffmpeg output:\n%s", path, out)
	}
	last := m[len(m)-1]
	h, _ := strconv.Atoi(last[1])
	min, _ := strconv.Atoi(last[2])
	sec, _ := strconv.ParseFloat(last[3], 64)
	if got := float64(h*3600+min*60) + sec; math.Abs(got-want) > 0.15 {
		t.Errorf("%s plays for %.2fs; the source is %.2fs (speed must not change)", path, got, want)
	}
}

// Installs the pinned tools into a temp dir, checks the encoders the tool
// relies on exist, then encodes real sources to every format.
func TestRealToolsEncode(t *testing.T) {
	d, err := deps.Load("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	// VID2GIF_BIN lets CI and Docker keep the downloaded tools between runs;
	// Install is a no-op for a tool that already matches its pin.
	binDir := os.Getenv("VID2GIF_BIN")
	if binDir == "" {
		binDir = t.TempDir()
	}
	for _, name := range []string{"yt-dlp", "ffmpeg", "deno"} {
		if err := deps.Install(name, d[name], binDir, os.Stdout); err != nil {
			t.Fatalf("install %s: %v", name, err)
		}
	}
	ffmpeg := filepath.Join(binDir, deps.ExeName("ffmpeg"))

	// YouTube extraction depends on the pinned deno actually running.
	if out, err := exec.Command(filepath.Join(binDir, deps.ExeName("deno")), "--version").Output(); err != nil || !bytes.HasPrefix(out, []byte("deno "+d["deno"].Version+" ")) {
		t.Errorf("pinned deno --version = %q, %v", out, err)
	}

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

	// 30 fps source: the medium preset resamples GIF/WebP to 15 fps.
	fast30 := filepath.Join(work, "fast30.mp4")
	mk("-f", "lavfi", "-i", "testsrc=duration=3:size=320x240:rate=30", "-c:v", "libx264", "-pix_fmt", "yuv420p", fast30)

	enc := encode.Runner(ffmpeg)
	for _, src := range []string{withAudio, oddNoAudio, fast30} {
		for _, format := range naming.AllFormats {
			src, format := src, format
			t.Run(fmt.Sprintf("%s-%s", filepath.Base(src), format), func(t *testing.T) {
				out := filepath.Join(work, filepath.Base(src)+"."+format)
				if err := enc(context.Background(), src, out, encode.Opts{Format: format, Preset: encode.Presets["medium"]}); err != nil {
					t.Fatal(err)
				}
				checkSignature(t, out, format)
				checkDuration(t, ffmpeg, out, 3)
			})
		}
	}
}
