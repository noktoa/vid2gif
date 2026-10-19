package main

import (
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	barCells   = 24
	labelWidth = 22 // "Getting the clip 99%" plus room; longer labels are cut
	sweepCells = 3
)

// tickEvery paces the only redraws: at most 8 a second. Tests stretch it and
// draw with tick().
var tickEvery = time.Second / 8

// bar is one progress line on a terminal, redrawn in place with \r: stage
// label, bar, and the overall percent when it is known and the stage is
// measured. Otherwise a sweep moves across the unfilled part on every tick.
// It never shows 100% or goes backwards, never estimates time, stays under
// 72 columns and uses no ANSI codes. Lines written through it erase the bar
// first; the next tick draws it again below them.
type bar struct {
	mu       sync.Mutex
	w        io.Writer
	blocks   bool    // █ ░ ▓, else # - =
	label    string  // from progress.Label
	overall  float64 // -1 = unknown
	measured bool    // the stage reports its own fraction
	frame    int
	dirty    bool
	shown    int // columns of bar text on screen; 0 = none
	stopped  bool
	quit     chan struct{}
	done     chan struct{}
}

func newBar(w io.Writer, blocks bool) *bar {
	return &bar{w: w, blocks: blocks, overall: -1}
}

// update takes the latest stage label and overall fraction (-1 = unknown).
func (b *bar) update(label string, overall float64, measured bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.label, b.measured, b.dirty = label, measured, true
	b.overall = max(b.overall, min(overall, 0.99))
}

// Write passes a whole line through, erasing the bar first.
func (b *bar) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.erase()
	b.dirty = true
	return b.w.Write(p)
}

func (b *bar) erase() {
	if b.shown > 0 {
		io.WriteString(b.w, "\r"+strings.Repeat(" ", b.shown)+"\r")
		b.shown = 0
	}
}

// tick redraws when something changed or the sweep is moving.
func (b *bar) tick() {
	b.mu.Lock()
	defer b.mu.Unlock()
	sweeping := b.overall < 0 || !b.measured
	if b.stopped || b.label == "" || (!b.dirty && !sweeping && b.shown > 0) {
		return
	}
	line := b.line()
	n := utf8.RuneCountInString(line)
	if n < b.shown { // clear what a longer previous line left
		line += strings.Repeat(" ", b.shown-n)
		n = b.shown
	}
	io.WriteString(b.w, "\r"+line)
	b.shown, b.dirty = n, false
	b.frame++
}

func (b *bar) line() string {
	fill, empty, sweep, label := "█", "░", "▓", b.label
	if !b.blocks {
		fill, empty, sweep, label = "#", "-", "=", strings.ReplaceAll(label, "…", "...")
	}
	if r := []rune(label); len(r) > labelWidth {
		label = string(r[:labelWidth])
	}
	label += strings.Repeat(" ", labelWidth-utf8.RuneCountInString(label))

	filled := 0
	if b.overall >= 0 {
		filled = int(b.overall * barCells) // overall <= 0.99: never full
	}
	cells := make([]string, barCells)
	for i := range cells {
		cells[i] = empty
		if i < filled {
			cells[i] = fill
		}
	}
	pct := ""
	if b.overall >= 0 && b.measured {
		pct = "  " + strconv.Itoa(min(99, int(math.Round(b.overall*100)))) + "%"
	} else if room := barCells - filled; room > 0 {
		// bounce a short block across the unfilled cells
		k := min(sweepCells, room)
		pos, span := 0, room-k
		if span > 0 {
			pos = b.frame % (2 * span)
			if pos > span {
				pos = 2*span - pos
			}
		}
		for i := range k {
			cells[filled+pos+i] = sweep
		}
	}
	return label + "  [" + strings.Join(cells, "") + "]" + pct
}

// start draws on a ticker until stop.
func (b *bar) start(every time.Duration) {
	b.quit, b.done = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(b.done)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-b.quit:
				return
			case <-t.C:
				b.tick()
			}
		}
	}()
}

// stop ends the ticker, waits for it and erases the bar. Later writes pass
// straight through.
func (b *bar) stop() {
	if b.quit != nil {
		close(b.quit)
		<-b.done
		b.quit = nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopped = true
	b.erase()
}

// blocksOK guesses whether the terminal shows █ ░ ▓. Conservative: on
// Windows only in Windows Terminal (WT_SESSION set; the classic console's
// font may lack them), elsewhere only with a UTF-8 locale (the first of
// LC_ALL, LC_CTYPE, LANG that is set). Anything else gets the ASCII bar.
func blocksOK(goos string, getenv func(string) string) bool {
	if goos == "windows" {
		return getenv("WT_SESSION") != ""
	}
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := strings.ToLower(getenv(k)); v != "" {
			return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
		}
	}
	return false
}

// isTerminal reports whether w is a console or tty: a character device and,
// on Windows, a real console (NUL is a character device too).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0 && isConsole(f)
}

// tail keeps the last max bytes written to it; safe for concurrent writes
// (yt-dlp's stdout and stderr are copied by separate goroutines).
type tail struct {
	mu  sync.Mutex
	b   []byte
	max int
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = t.b[len(t.b)-t.max:]
	}
	return len(p), nil
}

func (t *tail) bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.b
}
