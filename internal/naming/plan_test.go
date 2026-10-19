package naming

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseFormats(t *testing.T) {
	got, err := ParseFormats("GIF, mp4,gif,webp")
	if err != nil || !slices.Equal(got, []string{"gif", "mp4", "webp"}) {
		t.Errorf("got %v, %v", got, err)
	}
	for _, bad := range []string{"", "gif,avi", "gif,,mp4", "png"} {
		if got, err := ParseFormats(bad); err == nil {
			t.Errorf("ParseFormats(%q) = %v; want error", bad, got)
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
	if got := BuildName("My: Video", 83, 88.5, "medium"); got != "My__Video_83-88.5" {
		t.Errorf("got %q", got)
	}
	// End 0 means "until the end of the video"
	if got := BuildName("My: Video", 83, 0, "medium"); got != "My__Video_83-end" {
		t.Errorf("got %q", got)
	}
}

func TestBuildNameMediumUnchanged(t *testing.T) {
	if got := BuildName("My: Video", 83, 88.5, ""); got != "My__Video_83-88.5" {
		t.Errorf("empty quality: got %q", got)
	}
}

func TestBuildNameLowAndHighHaveSuffix(t *testing.T) {
	if got := BuildName("My: Video", 83, 88.5, "low"); got != "My__Video_83-88.5_low" {
		t.Errorf("got %q", got)
	}
	if got := BuildName("My: Video", 83, 88.5, "high"); got != "My__Video_83-88.5_high" {
		t.Errorf("got %q", got)
	}
}

func TestBuildNameEndLabelWithQuality(t *testing.T) {
	if got := BuildName("title", 83, 0, "high"); got != "title_83-end_high" {
		t.Errorf("got %q", got)
	}
}

func TestPlanOutputsOtherQualityIsNotSkipped(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x_1-2.gif"), []byte("x"), 0o644)
	todo, skipped := PlanOutputs(dir, BuildName("x", 1, 2, "high"), []string{"gif"}, false)
	if !slices.Equal(todo, []string{"gif"}) || len(skipped) != 0 {
		t.Errorf("todo=%v skipped=%v", todo, skipped)
	}
}

func TestPlanOutputsSameQualityIsSkipped(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x_1-2.gif"), []byte("x"), 0o644)
	todo, skipped := PlanOutputs(dir, BuildName("x", 1, 2, "medium"), []string{"gif"}, false)
	if len(todo) != 0 || !slices.Equal(skipped, []string{"gif"}) {
		t.Errorf("todo=%v skipped=%v", todo, skipped)
	}
}

func TestPlanOutputs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "demo.gif"), []byte("x"), 0o644)

	todo, skipped := PlanOutputs(dir, "demo", []string{"gif", "mp4"}, false)
	if !slices.Equal(todo, []string{"mp4"}) || !slices.Equal(skipped, []string{"gif"}) {
		t.Errorf("todo=%v skipped=%v", todo, skipped)
	}
	todo, skipped = PlanOutputs(dir, "demo", []string{"gif", "mp4"}, true)
	if !slices.Equal(todo, []string{"gif", "mp4"}) || len(skipped) != 0 {
		t.Errorf("overwrite: todo=%v skipped=%v", todo, skipped)
	}
	if got := OutPath(dir, "demo", "webp"); got != filepath.Join(dir, "demo.webp") {
		t.Errorf("OutPath = %q", got)
	}
}

func TestSanitizeTitleFitsLinuxFileNameLimit(t *testing.T) {
	for _, title := range []string{strings.Repeat("日", 100), strings.Repeat("😀", 100), strings.Repeat("a", 500)} {
		// longest possible suffix: "_" + two 5-digit times with decimals + ".webp"
		name := BuildName(title, 3725.5, 3730.5, "medium") + ".webp"
		if len(name) > 255 || !utf8.ValidString(name) {
			t.Errorf("name for %.8q... is %d bytes (valid utf8: %v); want <= 255", title, len(name), utf8.ValidString(name))
		}
	}
}
