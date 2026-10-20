package deps

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealDepsJSON(t *testing.T) {
	d, err := Load("../../deps.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"yt-dlp", "ffmpeg", "deno"} {
		tl, ok := d[name]
		if !ok || tl.Version == "" {
			t.Fatalf("%s missing or has no version", name)
		}
		for _, key := range []string{"windows-amd64", "linux-amd64", "linux-arm64"} {
			a, err := tl.AssetFor(key)
			if err != nil {
				t.Fatalf("%s %s: %v", name, key, err)
			}
			if !strings.HasPrefix(a.URL, "https://") || len(a.SHA256) != 64 {
				t.Errorf("%s %s: bad url/sha: %+v", name, key, a)
			}
			if a.Archive != "none" && a.Archive != "zip" && a.Archive != "tar.xz" {
				t.Errorf("%s %s: bad archive %q", name, key, a.Archive)
			}
		}
	}
}

func TestAssetForUnsupportedPlatform(t *testing.T) {
	_, err := Tool{Platforms: map[string]Asset{}}.AssetFor("darwin-arm64")
	if err == nil || !strings.Contains(err.Error(), "unsupported platform darwin-arm64") || !strings.Contains(err.Error(), "--no-pin") {
		t.Errorf("err = %v", err)
	}
}

func TestVersionMatches(t *testing.T) {
	cases := []struct {
		name, pin, out string
		want           bool
	}{
		{"yt-dlp", "2026.08.19", "2026.08.19\n", true},
		{"yt-dlp", "2026.08.19", "2026.03.03\n", false},
		{"yt-dlp", "2026.08.19", "2026.08.19.1\n", false},
		{"ffmpeg", "N-127588-g0b19c65afe", "ffmpeg version N-127588-g0b19c65afe-20261016 Copyright (c) 2000-2026\nbuilt with gcc\n", true},
		{"ffmpeg", "N-127588-g0b19c65afe", "ffmpeg version N-123074-g4e32fb4c2a-20260228 Copyright\n", false},
		{"ffmpeg", "N-127588-g0b19c65afe", "ffmpeg version N-127588-g0b19c65afe-20261016 Copyright\r\n", true},
		{"ffmpeg", "N-127588-g0b19c65afe", "", false},
		{"deno", "2.9.7", "deno 2.9.7 (stable, release, x86_64-pc-windows-msvc)\nv8 15.0.245.2-rusty\ntypescript 5.9.2\n", true},
		{"deno", "2.9.7", "deno 2.9.70 (stable, release, x86_64-pc-windows-msvc)\n", false},
		{"deno", "2.9.7", "deno 2.8.0 (stable, release, x86_64-pc-windows-msvc)\n", false},
	}
	for _, c := range cases {
		if got := versionMatches(c.name, c.pin, c.out); got != c.want {
			t.Errorf("versionMatches(%q, %q, %q) = %v; want %v", c.name, c.pin, c.out, got, c.want)
		}
	}
}

var ytTool = Tool{Version: "2026.08.19"}

func resolverWith(binDir string, pathHit string, ver string) Resolver {
	return Resolver{
		binDir: binDir,
		lookPath: func(string) (string, error) {
			if pathHit == "" {
				return "", errors.New("not found")
			}
			return pathHit, nil
		},
		version: func(string, string) (string, error) { return ver, nil },
	}
}

func makeBin(t *testing.T, base, name string) string {
	t.Helper()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(base, ExeName(name))
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolvePrefersBin(t *testing.T) {
	base := t.TempDir()
	want := makeBin(t, base, "yt-dlp")
	got, err := resolverWith(base, "/usr/bin/yt-dlp", "2026.08.19\n").Resolve("yt-dlp", ytTool, false)
	if err != nil || got != want {
		t.Errorf("got %q, %v; want %q", got, err, want)
	}
}

func TestResolveFallsBackToPath(t *testing.T) {
	got, err := resolverWith(t.TempDir(), "/usr/bin/yt-dlp", "2026.08.19\n").Resolve("yt-dlp", ytTool, false)
	if err != nil || got != "/usr/bin/yt-dlp" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestResolveMismatch(t *testing.T) {
	_, err := resolverWith(t.TempDir(), "/usr/bin/yt-dlp", "2025.01.01\n").Resolve("yt-dlp", ytTool, false)
	if err == nil || !strings.Contains(err.Error(), "pinned 2026.08.19") || !strings.Contains(err.Error(), "found 2025.01.01") || !strings.Contains(err.Error(), "deps install") {
		t.Errorf("err = %v", err)
	}
}

func TestResolveNoPinSkipsVersionCheck(t *testing.T) {
	r := resolverWith(t.TempDir(), "/usr/bin/yt-dlp", "")
	r.version = func(string, string) (string, error) {
		t.Fatal("version must not be checked with --no-pin")
		return "", nil
	}
	got, err := r.Resolve("yt-dlp", Tool{}, true)
	if err != nil || got != "/usr/bin/yt-dlp" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestResolveNotFound(t *testing.T) {
	_, err := resolverWith(t.TempDir(), "", "").Resolve("yt-dlp", ytTool, false)
	if err == nil || !strings.Contains(err.Error(), "deps install") {
		t.Errorf("err = %v", err)
	}
}

func TestResolveOwn(t *testing.T) {
	// bin is held to the pin
	base := t.TempDir()
	makeBin(t, base, "yt-dlp")
	if _, _, err := resolverWith(base, "", "2025.01.01\n").ResolveOwn("yt-dlp", ytTool); err == nil {
		t.Error("a mismatched tool in bin must fail")
	}
	// PATH is the user's own: a different version runs, with a note
	got, note, err := resolverWith(t.TempDir(), "/usr/bin/yt-dlp", "2025.01.01\n").ResolveOwn("yt-dlp", ytTool)
	if err != nil || got != "/usr/bin/yt-dlp" || !strings.Contains(note, "2025.01.01") || !strings.Contains(note, "2026.08.19") {
		t.Errorf("got %q, %q, %v", got, note, err)
	}
	if _, note, _ := resolverWith(t.TempDir(), "/usr/bin/yt-dlp", "2026.08.19\n").ResolveOwn("yt-dlp", ytTool); note != "" {
		t.Errorf("matching version has note %q", note)
	}
	_, _, err = resolverWith(t.TempDir(), "", "").ResolveOwn("yt-dlp", ytTool)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want not-exist", err)
	}
}
