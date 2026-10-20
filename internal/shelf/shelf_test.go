package shelf

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shelf.db")
	s, notice, err := Open(path)
	if err != nil || notice != "" {
		t.Fatalf("Open: notice %q, err %v", notice, err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func ids(clips []Clip) []string {
	var out []string
	for _, c := range clips {
		out = append(out, c.ID)
	}
	return out
}

func TestRoundTripEveryField(t *testing.T) {
	s, _ := openTemp(t)
	start := 12.5
	full := Clip{
		ID: "a", Lane: LaneMade, URL: "https://x/1", Title: "T", ThumbnailURL: "https://x/t.jpg",
		DurationSec: 61.25, Source: "Pasted links", Unavailable: true, Reason: "private",
		StartText: "0:12.5", EndText: "+5", EndKind: "length", Start: &start, End: 17.5,
		TimeMsg: "ok", Checked: "0:12.5|+5", Formats: []string{"gif", "mp4"}, Quality: "high",
		ResultKind: "done", ResultMessage: "Saved", Files: []File{{Format: "gif", Path: `C:\a.gif`, Size: 99}},
		CreatedAt: time.UnixMilli(1700000000000), MadeAt: time.UnixMilli(1700000005000),
	}
	bare := Clip{ID: "b", Lane: LaneLater, URL: "https://x/2", Formats: []string{"gif"}, CreatedAt: time.UnixMilli(1700000001000)}
	for _, c := range []Clip{full, bare} {
		if err := s.Put(c); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d clips", len(got))
	}
	// Load orders ready, later, made: bare (later) comes before full (made).
	if !reflect.DeepEqual(got[0], bare) {
		t.Errorf("bare = %+v", got[0])
	}
	if got[1].Start == nil || *got[1].Start != start {
		t.Errorf("Start = %v", got[1].Start)
	}
	if !reflect.DeepEqual(got[1], full) {
		t.Errorf("full = %+v", got[1])
	}
}

func TestPlaylistRowsKeepOrderAndSameLinkTwiceIsTwoClips(t *testing.T) {
	s, _ := openTemp(t)
	a, b, c := NewID(), NewID(), NewID()
	if a == b {
		t.Fatal("NewID repeated")
	}
	for _, id := range []string{a, b, c} {
		if err := s.Put(Clip{ID: id, Lane: LaneLater, URL: "https://same"}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := s.Load()
	if !reflect.DeepEqual(ids(got), []string{a, b, c}) {
		t.Errorf("order = %v", ids(got))
	}
}

func TestPutOfExistingClipWithNewLaneMovesItToTheEnd(t *testing.T) {
	s, _ := openTemp(t)
	for _, id := range []string{"1", "2"} {
		s.Put(Clip{ID: id, Lane: LaneLater})
	}
	s.Put(Clip{ID: "3", Lane: LaneReady})
	s.Put(Clip{ID: "1", Lane: LaneReady, Title: "moved"})
	got, _ := s.Load()
	if !reflect.DeepEqual(ids(got), []string{"3", "1", "2"}) {
		t.Errorf("order = %v", ids(got))
	}
	// Same lane keeps its place.
	s.Put(Clip{ID: "3", Lane: LaneReady, Title: "edited"})
	got, _ = s.Load()
	if !reflect.DeepEqual(ids(got), []string{"3", "1", "2"}) || got[0].Title != "edited" {
		t.Errorf("after edit = %+v", got)
	}
}

func TestFinishMovesToMadeWithResult(t *testing.T) {
	s, _ := openTemp(t)
	s.Put(Clip{ID: "1", Lane: LaneReady})
	files := []File{{Format: "gif", Path: "p.gif", Size: 5}}
	if err := s.Finish("1", "done", "All set", files); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Load()
	c := got[0]
	if c.Lane != LaneMade || c.ResultKind != "done" || c.ResultMessage != "All set" ||
		!reflect.DeepEqual(c.Files, files) || c.MadeAt.IsZero() || time.Since(c.MadeAt) > time.Minute {
		t.Errorf("clip = %+v", c)
	}
	if err := s.Finish("nope", "done", "", nil); err == nil {
		t.Error("Finish of unknown id returned nil")
	}
}

func TestClearMadeKeepsReadyAndLater(t *testing.T) {
	s, _ := openTemp(t)
	s.Put(Clip{ID: "r", Lane: LaneReady})
	s.Put(Clip{ID: "l", Lane: LaneLater})
	s.Put(Clip{ID: "m", Lane: LaneMade})
	if err := s.ClearMade(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Load()
	if !reflect.DeepEqual(ids(got), []string{"r", "l"}) {
		t.Errorf("left = %v", ids(got))
	}
	if err := s.Remove("l"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Load()
	if !reflect.DeepEqual(ids(got), []string{"r"}) {
		t.Errorf("after Remove = %v", ids(got))
	}
}

func TestWipeEmptiesStoreAndRemovesFiles(t *testing.T) {
	s, path := openTemp(t)
	s.Put(Clip{ID: "1", Lane: LaneReady})
	if err := s.Wipe(); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || len(got) != 0 {
		t.Fatalf("after Wipe: %v, %v", got, err)
	}
	if err := s.Put(Clip{ID: "2", Lane: LaneReady}); err != nil {
		t.Fatalf("store unusable after Wipe: %v", err)
	}
	// The old rows are gone from disk, not just hidden.
	s.Close()
	s2, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, _ = s2.Load()
	if !reflect.DeepEqual(ids(got), []string{"2"}) {
		t.Errorf("reopened = %v", ids(got))
	}
}

func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMigrationsRunOnceAndReopenIsIdempotent(t *testing.T) {
	s, path := openTemp(t)
	s.Put(Clip{ID: "1", Lane: LaneReady})
	s.Close()
	for i := 0; i < 2; i++ {
		s2, notice, err := Open(path)
		if err != nil || notice != "" {
			t.Fatalf("reopen %d: %q, %v", i, notice, err)
		}
		if v := userVersion(t, s2.db); v != len(migrations) {
			t.Errorf("user_version = %d, want %d", v, len(migrations))
		}
		got, _ := s2.Load()
		if len(got) != 1 {
			t.Errorf("reopen %d lost rows: %v", i, got)
		}
		s2.Close()
	}
}

func assertMovedAside(t *testing.T, path string) {
	t.Helper()
	bad, _ := filepath.Glob(path + ".bad-*")
	if len(bad) != 1 {
		t.Errorf("bad files = %v", bad)
	}
}

func TestCorruptFileIsMovedAsideAndReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shelf.db")
	if err := os.WriteFile(path, []byte(strings.Repeat("this is not a database. ", 400)), 0o644); err != nil {
		t.Fatal(err)
	}
	s, notice, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if notice != "Couldn't open your saved shelf. Starting fresh. A copy was kept." {
		t.Errorf("notice = %q", notice)
	}
	assertMovedAside(t, path)
	got, err := s.Load()
	if err != nil || len(got) != 0 {
		t.Fatalf("not empty: %v, %v", got, err)
	}
	if err := s.Put(Clip{ID: "1", Lane: LaneReady}); err != nil {
		t.Errorf("not usable: %v", err)
	}
}

func TestTooNewSchemaIsMovedAside(t *testing.T) {
	s, path := openTemp(t)
	if _, err := s.db.Exec("PRAGMA user_version = 999"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, notice, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if notice == "" {
		t.Error("no notice")
	}
	assertMovedAside(t, path)
	if v := userVersion(t, s2.db); v != len(migrations) {
		t.Errorf("fresh store user_version = %d", v)
	}
}

func TestWriteAfterCloseReturnsErrorNotPanic(t *testing.T) {
	s, _ := openTemp(t)
	s.Close()
	if err := s.Put(Clip{ID: "1", Lane: LaneReady}); err == nil {
		t.Error("Put after Close returned nil")
	}
	if _, err := s.Load(); err == nil {
		t.Error("Load after Close returned nil")
	}
}
