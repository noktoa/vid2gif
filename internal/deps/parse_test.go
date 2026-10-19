package deps

import (
	"path/filepath"
	"testing"

	"vid2gif"
)

func TestParseDepsJSON(t *testing.T) {
	p, err := Parse(vid2gif.DepsJSON)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"yt-dlp", "ffmpeg", "deno"} {
		if p[name].Version == "" {
			t.Errorf("embedded pins have no %s", name)
		}
	}
}

func TestBinDir(t *testing.T) {
	base, abs := t.TempDir(), t.TempDir()
	env := func(v string) func(string) string { return func(string) string { return v } }
	if got, want := BinDir(base, env("")), filepath.Join(base, "bin"); got != want {
		t.Errorf("default = %q; want %q", got, want)
	}
	if got := BinDir(base, env(abs)); got != abs {
		t.Errorf("absolute = %q; want %q", got, abs)
	}
	if got, want := BinDir(base, env("tools")), filepath.Join(base, "tools"); got != want {
		t.Errorf("relative = %q; want %q", got, want)
	}
}
