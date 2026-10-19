package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadMissingFileGivesDefault(t *testing.T) {
	got := Load(filepath.Join(t.TempDir(), "nope.json"))
	if !slices.Equal(got.Formats, Default().Formats) || got.Theme != "grey" || got.OutDir != Default().OutDir {
		t.Errorf("got %+v", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	want := Settings{Theme: "mint", OutDir: filepath.Join(t.TempDir(), "clips"), Formats: []string{"gif", "mp4"}, Quality: "high", FullSource: true, DebugMode: true}
	if err := want.Save(p); err != nil {
		t.Fatal(err)
	}
	got := Load(p)
	if got.Theme != want.Theme || got.OutDir != want.OutDir || !slices.Equal(got.Formats, want.Formats) ||
		got.Quality != want.Quality || got.FullSource != want.FullSource || got.DebugMode != want.DebugMode {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

func TestLoadCorruptFileGivesDefault(t *testing.T) {
	got := Load(writeFile(t, `{not json`))
	if got.Theme != "grey" || got.Quality != "medium" || !slices.Equal(got.Formats, []string{"gif"}) {
		t.Errorf("got %+v", got)
	}
}

func TestLoadUnknownThemeFallsBackToGrey(t *testing.T) {
	keep := filepath.Join(t.TempDir(), "keep")
	q, _ := json.Marshal(keep)
	got := Load(writeFile(t, `{"theme":"neon","outDir":`+string(q)+`,"formats":["mp4"],"quality":"high"}`))
	if got.Theme != "grey" {
		t.Errorf("theme = %q", got.Theme)
	}
	if got.OutDir != keep || !slices.Equal(got.Formats, []string{"mp4"}) || got.Quality != "high" {
		t.Errorf("good fields were discarded: %+v", got)
	}
}

func TestLoadEmptyFormatsFallsBackToGif(t *testing.T) {
	got := Load(writeFile(t, `{"theme":"lilac","formats":[]}`))
	if !slices.Equal(got.Formats, []string{"gif"}) || got.Theme != "lilac" {
		t.Errorf("got %+v", got)
	}
}

func TestLoadUnknownQualityFallsBackToMedium(t *testing.T) {
	got := Load(writeFile(t, `{"quality":"ultra","theme":"mint"}`))
	if got.Quality != "medium" || got.Theme != "mint" {
		t.Errorf("got %+v", got)
	}
}

func TestSaveCreatesParentFolder(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a", "b", "settings.json")
	if err := Default().Save(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Error(err)
	}
}

func TestDefaultIsFasterDownload(t *testing.T) {
	if Default().FullSource {
		t.Error("FullSource should default to false")
	}
}

func TestDefaultDebugModeOff(t *testing.T) {
	if Default().DebugMode {
		t.Error("DebugMode should default to false")
	}
}

func TestValidOutDir(t *testing.T) {
	data := filepath.Join(t.TempDir(), "vid2gif")
	good := filepath.Join(t.TempDir(), "Clips")
	root := filepath.VolumeName(good) + string(filepath.Separator)
	type tc struct {
		dir  string
		want bool
	}
	cases := []tc{
		{good, true},
		{"", false},
		{"  ", false},
		{`clips`, false},
		{`.\clips`, false},
		{`\\host\share\clips`, false},
		{`//host/share/clips`, false},
		{`\\?\C:\clips`, false},
		{`\\.\C:\clips`, false},
		{root, false},
		{data, false},
		{filepath.Join(data, "clips"), false},
		{data + "-other", true}, // a sibling with the same prefix is fine
	}
	if runtime.GOOS == "windows" {
		cases = append(cases, tc{strings.ToUpper(filepath.Join(data, "clips")), false}, tc{`C:`, false}, tc{`C:clips`, false})
	}
	for _, c := range cases {
		if got := ValidOutDir(c.dir, data); got != c.want {
			t.Errorf("ValidOutDir(%q) = %v, want %v", c.dir, got, c.want)
		}
	}
}

func TestLoadInvalidOutDirFallsBackKeepingOtherFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	inside, _ := json.Marshal(filepath.Join(dir, "clips")) // inside the data root
	for _, out := range []string{`"relative\\clips"`, `"\\\\host\\share"`, string(inside)} {
		if err := os.WriteFile(p, []byte(`{"theme":"mint","outDir":`+out+`,"quality":"high"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		got := Load(p)
		if got.OutDir != Default().OutDir || got.Theme != "mint" || got.Quality != "high" {
			t.Errorf("outDir %s: got %+v", out, got)
		}
	}
}
