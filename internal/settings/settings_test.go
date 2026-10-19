package settings

import (
	"os"
	"path/filepath"
	"slices"
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
	want := Settings{Theme: "mint", OutDir: `C:\clips`, Formats: []string{"gif", "mp4"}, Quality: "high", FullSource: true, DebugMode: true}
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
	got := Load(writeFile(t, `{"theme":"neon","outDir":"D:\\keep","formats":["mp4"],"quality":"high"}`))
	if got.Theme != "grey" {
		t.Errorf("theme = %q", got.Theme)
	}
	if got.OutDir != `D:\keep` || !slices.Equal(got.Formats, []string{"mp4"}) || got.Quality != "high" {
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
