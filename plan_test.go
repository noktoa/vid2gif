package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseFormats(t *testing.T) {
	got, err := parseFormats("GIF, mp4,gif,webp")
	if err != nil || !slices.Equal(got, []string{"gif", "mp4", "webp"}) {
		t.Errorf("got %v, %v", got, err)
	}
	for _, bad := range []string{"", "gif,avi", "gif,,mp4", "png"} {
		if got, err := parseFormats(bad); err == nil {
			t.Errorf("parseFormats(%q) = %v; want error", bad, got)
		}
	}
}

func TestSanitizeTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"My: Video/Name?", "My__Video_Name_"},
		{"  spaced   out  ", "spaced_out"},
		{"Title...", "Title"},
		{"a\tb\nc", "a_b_c"},
		{"", "clip"},
		{"   ", "clip"},
		{"...", "clip"},
		{`<>:"/\|?*`, "_________"},
		{"日本語のタイトル", "日本語のタイトル"},
		{strings.Repeat("a", 100), strings.Repeat("a", 80)},
		{strings.Repeat("日", 100), strings.Repeat("日", 66)}, // 198 bytes: Linux file names max out at 255 bytes
		{strings.Repeat("😀", 100), strings.Repeat("😀", 50)},
	}
	for _, c := range cases {
		if got := sanitizeTitle(c.in); got != c.want {
			t.Errorf("sanitizeTitle(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestBuildName(t *testing.T) {
	if got := buildName("My: Video", 83, 88.5); got != "My__Video_83-88.5" {
		t.Errorf("got %q", got)
	}
}

func TestPlanOutputs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "demo.gif"), []byte("x"), 0o644)

	todo, skipped := planOutputs(dir, "demo", []string{"gif", "mp4"}, false)
	if !slices.Equal(todo, []string{"mp4"}) || !slices.Equal(skipped, []string{"gif"}) {
		t.Errorf("todo=%v skipped=%v", todo, skipped)
	}
	todo, skipped = planOutputs(dir, "demo", []string{"gif", "mp4"}, true)
	if !slices.Equal(todo, []string{"gif", "mp4"}) || len(skipped) != 0 {
		t.Errorf("overwrite: todo=%v skipped=%v", todo, skipped)
	}
	if got := outPath(dir, "demo", "webp"); got != filepath.Join(dir, "demo.webp") {
		t.Errorf("outPath = %q", got)
	}
}

func TestSanitizeTitleFitsLinuxFileNameLimit(t *testing.T) {
	for _, title := range []string{strings.Repeat("日", 100), strings.Repeat("😀", 100), strings.Repeat("a", 500)} {
		// longest possible suffix: "_" + two 5-digit times with decimals + ".webp"
		name := buildName(title, 3725.5, 3730.5) + ".webp"
		if len(name) > 255 || !utf8.ValidString(name) {
			t.Errorf("name for %.8q... is %d bytes (valid utf8: %v); want <= 255", title, len(name), utf8.ValidString(name))
		}
	}
}
