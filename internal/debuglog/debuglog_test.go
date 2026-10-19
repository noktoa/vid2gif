package debuglog

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func logFiles(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "vid2gif-*.log"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func readAll(t *testing.T, dir string) string {
	t.Helper()
	var sb strings.Builder
	for _, f := range logFiles(t, dir) {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
	}
	return sb.String()
}

func TestDisabledCreatesNoFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	l := New(dir, false)
	l.Printf("hello %d", 1)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory should not exist, stat err = %v", err)
	}
}

func TestEnabledWritesTimestampedLine(t *testing.T) {
	dir := t.TempDir()
	l := New(dir, true)
	l.Printf("hello %d", 42)
	if n := len(logFiles(t, dir)); n != 1 {
		t.Fatalf("want 1 log file, got %d", n)
	}
	got := readAll(t, dir)
	if !regexp.MustCompile(`time=\d{4}-\d{2}-\d{2}T`).MatchString(got) || !strings.Contains(got, "hello 42") {
		t.Errorf("line = %q", got)
	}
	if l.Dir() != dir {
		t.Errorf("Dir() = %q; want %q", l.Dir(), dir)
	}
}

func TestSetEnabledMidRun(t *testing.T) {
	dir := t.TempDir()
	l := New(dir, false)
	l.Printf("before")
	if n := len(logFiles(t, dir)); n != 0 {
		t.Fatalf("file created while disabled")
	}
	l.SetEnabled(true)
	l.Printf("during")
	l.SetEnabled(false)
	l.Printf("after")
	l.SetEnabled(true)
	l.Printf("again")
	got := readAll(t, dir)
	if strings.Contains(got, "before") || strings.Contains(got, "after") {
		t.Errorf("disabled writes leaked: %q", got)
	}
	if !strings.Contains(got, "during") || !strings.Contains(got, "again") {
		t.Errorf("missing enabled writes: %q", got)
	}
	if n := len(logFiles(t, dir)); n != 1 {
		t.Errorf("one file per run, got %d", n)
	}
}

func TestNilLogIsSafe(t *testing.T) {
	var l *Log
	l.SetEnabled(true)
	l.Printf("x %s", "y")
	if l.Dir() != "" {
		t.Errorf("Dir() = %q", l.Dir())
	}
}

func TestRedactSignedMediaURL(t *testing.T) {
	in := "GET https://rr3---sn-abc.googlevideo.com/videoplayback?expire=1790000000&ip=203.0.113.9&sig=AOq0QJ8w&id=o-xyz done"
	got := Redact(in)
	want := "GET https://rr3---sn-abc.googlevideo.com/videoplayback done"
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestRedactAnyHostLosesQuery(t *testing.T) {
	got := Redact("see https://cdn.example.org/a/b.mp4?token=secret#t=3")
	if got != "see https://cdn.example.org/a/b.mp4" {
		t.Errorf("got %q", got)
	}
}

func TestRedactKeepsYouTubeVideoID(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&si=trackme&list=PL123&t=30s": "https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PL123",
		"https://youtu.be/dQw4w9WgXcQ?si=trackme&t=5":                             "https://youtu.be/dQw4w9WgXcQ",
		"https://m.youtube.com/watch?v=abc&ip=1.2.3.4":                            "https://m.youtube.com/watch?v=abc",
		"https://notyoutube.com/watch?v=abc":                                      "https://notyoutube.com/watch",
	}
	for in, want := range cases {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestRedactBareParams(t *testing.T) {
	got := Redact("args: --extractor-args ip=10.0.0.1&expire=123 sig=abc keep=me")
	for _, bad := range []string{"10.0.0.1", "expire=123", "sig=abc"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q still in %q", bad, got)
		}
	}
	if !strings.Contains(got, "keep=me") {
		t.Errorf("over-redacted: %q", got)
	}
}

func TestRedactPlainTextUntouched(t *testing.T) {
	in := "encoding clip 3 of 5 (no urls here)"
	if got := Redact(in); got != in {
		t.Errorf("got %q", got)
	}
}

func TestPrintfRedacts(t *testing.T) {
	dir := t.TempDir()
	l := New(dir, true)
	l.Printf("fetching %s", "https://x.googlevideo.com/videoplayback?sig=SECRET")
	got := readAll(t, dir)
	if strings.Contains(got, "SECRET") {
		t.Errorf("unredacted: %q", got)
	}
}

func TestRotationKeepsFiveNewest(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 8; i++ {
		name := fmt.Sprintf("vid2gif-20260101-00000%d.log", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	l := New(dir, true)
	if n := len(logFiles(t, dir)); n != 4 {
		t.Fatalf("after New want 4 old files, got %d", n)
	}
	l.Printf("run")
	files := logFiles(t, dir)
	if len(files) != 5 {
		t.Fatalf("want 5 files, got %d: %v", len(files), files)
	}
	for _, gone := range []string{"000001", "000002", "000003", "000004"} {
		if _, err := os.Stat(filepath.Join(dir, "vid2gif-20260101-"+gone+".log")); err == nil {
			t.Errorf("%s should have been deleted", gone)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("unrelated file removed: %v", err)
	}
}

func TestConcurrentPrintf(t *testing.T) {
	dir := t.TempDir()
	l := New(dir, false)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				l.Printf("line %d", j)
				if j%7 == 0 {
					l.SetEnabled(j%2 == 0)
				}
			}
		}()
	}
	wg.Wait()
	l.SetEnabled(true)
	l.Printf("final")
	if !strings.Contains(readAll(t, dir), "final") {
		t.Error("final line missing")
	}
}

func TestRedactPathStyleSignedParams(t *testing.T) {
	in := "fetch https://manifest.googlevideo.com/api/manifest/hls_playlist/expire/1790000000/ip/203.0.113.9/id/ABC/sig/AAAsecretSig/file/index.m3u8 ok"
	got := Redact(in)
	for _, bad := range []string{"1790000000", "203.0.113.9", "AAAsecretSig"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q still in %q", bad, got)
		}
	}
	want := "fetch https://manifest.googlevideo.com/api/manifest/hls_playlist/id/ABC/file/index.m3u8 ok"
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
	if got := Redact("https://h.example/a/IP/9.9.9.9/b"); got != "https://h.example/a/b" {
		t.Errorf("case-insensitive key: got %q", got)
	}
}

func TestRedactUserinfo(t *testing.T) {
	cases := map[string]string{
		"https://user:pass@host.example/x?y=1":              "https://host.example/x",
		"http://tok@host.example/x":                         "http://host.example/x",
		"https://user:pass@www.youtube.com/watch?v=abc&z=1": "https://www.youtube.com/watch?v=abc",
	}
	for in, want := range cases {
		got := Redact(in)
		if got != want || strings.Contains(got, "user:pass") {
			t.Errorf("Redact(%q) = %q; want %q", in, got, want)
		}
	}
}
