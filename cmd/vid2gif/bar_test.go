package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// syncWriter is a bytes.Buffer safe for the ticker goroutine.
type syncWriter struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncWriter) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// frames splits what the bar wrote into the lines drawn with \r, padding
// trimmed, erases (blank frames) dropped.
func frames(s string) []string {
	var out []string
	for _, f := range strings.Split(s, "\r") {
		if f = strings.TrimRight(f, " "); f != "" {
			out = append(out, f)
		}
	}
	return out
}

var pctRe = regexp.MustCompile(`\]\s+(\d+)%$`)

func pctOf(t *testing.T, frame string) int {
	t.Helper()
	m := pctRe.FindStringSubmatch(frame)
	if m == nil {
		return -1
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func TestBarMeasured(t *testing.T) {
	var w bytes.Buffer
	b := newBar(&w, true)
	b.update("Getting the clip 63%", 0.41, true)
	b.tick()
	got := frames(w.String())
	if len(got) != 1 {
		t.Fatalf("frames = %q", got)
	}
	want := "Getting the clip 63%    [█████████░░░░░░░░░░░░░░░]  41%"
	if got[0] != want {
		t.Errorf("frame = %q\nwant    %q", got[0], want)
	}
}

func TestBarASCII(t *testing.T) {
	var w bytes.Buffer
	b := newBar(&w, false)
	b.update("Making GIF…", 0.5, false)
	b.tick()
	f := frames(w.String())[0]
	if !utf8.ValidString(f) || strings.ContainsAny(f, "█░▓…") || !strings.HasPrefix(f, "Making GIF...") || !strings.Contains(f, "[############") {
		t.Errorf("ascii frame = %q", f)
	}
}

func TestBarIndeterminateHasNoPercentAndMoves(t *testing.T) {
	for _, c := range []struct {
		name     string
		overall  float64
		measured bool
	}{
		{"unknown length", -1, true},
		{"unmeasured stage", 0.3, false},
	} {
		var w bytes.Buffer
		b := newBar(&w, true)
		b.update("Making GIF…", c.overall, c.measured)
		for range 5 {
			b.tick() // no update in between: it must keep moving by itself
		}
		got := frames(w.String())
		if len(got) != 5 {
			t.Fatalf("%s: frames = %q, want a redraw per tick", c.name, got)
		}
		for _, f := range got {
			if strings.Contains(f, "%") {
				t.Errorf("%s: %q shows a percentage", c.name, f)
			}
		}
		if got[0] == got[1] {
			t.Errorf("%s: sweep did not move: %q", c.name, got[:2])
		}
		if c.overall > 0 && !strings.Contains(got[0], "[███████") {
			t.Errorf("%s: known overall lost its fill: %q", c.name, got[0])
		}
	}
}

func TestBarNever100NeverBackwards(t *testing.T) {
	var w bytes.Buffer
	b := newBar(&w, true)
	for _, o := range []float64{0.1, 0.5, 0.4, 0.99, 1, 0.2} {
		b.update("Making MP4 40%", o, true)
		b.tick()
	}
	prev := 0
	for _, f := range frames(w.String()) {
		p := pctOf(t, f)
		if p < prev || p >= 100 || strings.Count(f, "░") == 0 {
			t.Fatalf("frame %q after %d%%: want non-decreasing, below 100, never full", f, prev)
		}
		prev = p
	}
	if prev != 99 {
		t.Errorf("last pct = %d, want 99", prev)
	}
}

func TestBarWidthBounded(t *testing.T) {
	var w bytes.Buffer
	b := newBar(&w, true)
	b.update(strings.Repeat("a very long stage label ", 10), 0.5, true)
	b.tick()
	b.update("short", 0.6, false) // no percent: a shorter line
	b.tick()
	for _, f := range strings.Split(w.String(), "\r") {
		if n := utf8.RuneCountInString(f); n > 72 {
			t.Errorf("line is %d columns: %q", n, f)
		}
	}
	// the shorter second line pads over the first one's leftovers
	parts := strings.Split(w.String(), "\r")
	if utf8.RuneCountInString(parts[len(parts)-1]) < utf8.RuneCountInString(parts[len(parts)-2]) {
		t.Errorf("leftovers not cleared: %q", parts)
	}
}

func TestBarOnlyTickDrawsAndIdleTickSkips(t *testing.T) {
	var w bytes.Buffer
	b := newBar(&w, true)
	b.update("Getting the clip 10%", 0.1, true)
	if w.Len() != 0 {
		t.Fatalf("update drew: %q", w.String())
	}
	b.tick()
	n := w.Len()
	b.tick() // nothing changed, nothing animating
	if w.Len() != n {
		t.Errorf("idle tick redrew: %q", w.String())
	}
}

func TestBarErasesBeforeLineAndRedrawsAfter(t *testing.T) {
	var w bytes.Buffer
	b := newBar(&w, true)
	b.update("Making GIF…", -1, false)
	b.tick()
	drawn := w.Len()
	b.Write([]byte("wrote x.gif (3 KB)\n"))
	after := w.String()[drawn:]
	if !regexp.MustCompile(`^\r +\rwrote x\.gif \(3 KB\)\n$`).MatchString(after) {
		t.Errorf("line not preceded by an erase: %q", after)
	}
	b.tick()
	if !strings.HasPrefix(w.String()[drawn+len(after):], "\rMaking GIF") {
		t.Errorf("bar not redrawn after the line: %q", w.String())
	}
}

func TestBarStopErasesAndTickerEnds(t *testing.T) {
	var w syncWriter
	b := newBar(&w, true)
	b.update("Making GIF…", -1, false)
	b.start(time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	b.stop()
	got := w.String()
	if len(frames(got)) == 0 {
		t.Fatal("ticker never drew")
	}
	if !regexp.MustCompile(`\r +\r$`).MatchString(got) {
		t.Errorf("bar not erased at the end: %q", got[max(0, len(got)-80):])
	}
	select {
	case <-b.done:
	default:
		t.Error("ticker goroutine still running after stop")
	}
	time.Sleep(20 * time.Millisecond)
	if w.String() != got {
		t.Error("output after stop: the ticker is still running")
	}
	b.Write([]byte("error line\n"))
	if w.String() != got+"error line\n" {
		t.Errorf("write after stop = %q", w.String()[len(got):])
	}
}

func TestBlocksOK(t *testing.T) {
	env := func(kv ...string) func(string) string {
		return func(k string) string {
			for i := 0; i < len(kv); i += 2 {
				if kv[i] == k {
					return kv[i+1]
				}
			}
			return ""
		}
	}
	for _, c := range []struct {
		goos string
		env  func(string) string
		want bool
	}{
		{"windows", env(), false},
		{"windows", env("WT_SESSION", "x"), true},
		{"linux", env("LANG", "en_US.UTF-8"), true},
		{"linux", env("LC_ALL", "C", "LANG", "en_US.UTF-8"), false},
		{"darwin", env("LC_CTYPE", "en_US.utf8"), true},
		{"linux", env(), false},
		{"linux", env("LANG", "C"), false},
	} {
		if got := blocksOK(c.goos, c.env); got != c.want {
			t.Errorf("blocksOK(%s, ...) = %v, want %v", c.goos, got, c.want)
		}
	}
}

func TestIsTerminalFalseForBufferAndFile(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(&bytes.Buffer{}) || isTerminal(f) {
		t.Error("a buffer or a regular file was taken for a terminal")
	}
}

func TestTailKeepsTheEnd(t *testing.T) {
	tl := &tail{max: 8}
	tl.Write([]byte("0123456789"))
	tl.Write([]byte("ab"))
	if got := string(tl.bytes()); got != "456789ab" {
		t.Errorf("tail = %q", got)
	}
}
