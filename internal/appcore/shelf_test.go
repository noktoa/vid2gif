package appcore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"vid2gif/internal/debuglog"
	"vid2gif/internal/deps"
	"vid2gif/internal/lookup"
	"vid2gif/internal/shelf"
)

// reboot closes c and starts a new Core on the same folder, as a relaunch does.
func reboot(t *testing.T, c *Core) (*Core, *recorder) {
	t.Helper()
	c.store.Close()
	rec := &recorder{}
	n := New(rec.emit, debuglog.New(filepath.Join(filepath.Dir(c.settingsPath), "logs"), false), c.settingsPath, c.binDir, deps.Pins{})
	n.resolve = func() (Tools, error) { return Tools{"yt", "ff", "deno"}, nil }
	t.Cleanup(func() { n.store.Close() })
	return n, rec
}

func TestLookupSavesResolvedClipsInOrderToLater(t *testing.T) {
	c, rec := newTestCore(t)
	s := c.Current()
	s.Formats, s.Quality = []string{"gif", "mp4"}, "high"
	if err := c.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	c.lookup = func(context.Context, Tools, string) (string, []lookup.Video, error) {
		<-release
		return "My list", []lookup.Video{{URL: "https://v/a", Title: "A"}, {URL: "https://v/b", Title: "B"}, {URL: "https://v/c", Title: "C"}}, nil
	}
	c.Add("https://list.example/p")
	s.Formats, s.Quality = []string{"webp"}, "low" // changed after Add: the snapshot must win
	if err := c.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	close(release)
	rec.wait(t, "slips:replace")

	n, _ := reboot(t, c)
	got := n.Boot().Shelf.Clips
	if len(got) != 3 {
		t.Fatalf("clips = %d, want 3", len(got))
	}
	for i, title := range []string{"A", "B", "C"} {
		g := got[i]
		if g.Title != title || g.Lane != shelf.LaneLater || g.Source != "My list" ||
			!slices.Equal(g.Formats, []string{"gif", "mp4"}) || g.Quality != "high" {
			t.Errorf("clip %d = %+v", i, g.Clip)
		}
	}
}

func TestFailedLookupIsNotSaved(t *testing.T) {
	c, rec := newTestCore(t)
	c.lookup = func(context.Context, Tools, string) (string, []lookup.Video, error) {
		return "", nil, errors.New("ERROR: Private video")
	}
	c.Add("https://bad.example/v")
	rec.wait(t, "slips:replace")
	n, _ := reboot(t, c)
	if got := n.Boot().Shelf.Clips; len(got) != 0 {
		t.Errorf("clips = %+v, want none", got)
	}
}

func TestBootReturnsSavedLanesAndTypedTimes(t *testing.T) {
	c, _ := newTestCore(t)
	err := c.Save([]shelf.Clip{
		{ID: "r1", Lane: shelf.LaneReady, URL: "u1", StartText: "0:12", EndText: "5", EndKind: "length"},
		{ID: "l1", Lane: shelf.LaneLater, URL: "u2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	n, _ := reboot(t, c)
	got := n.Boot().Shelf.Clips
	if len(got) != 2 || got[0].ID != "r1" || got[1].ID != "l1" {
		t.Fatalf("clips = %+v", got)
	}
	r := got[0]
	if r.StartText != "0:12" || r.EndText != "5" || r.EndKind != "length" || r.Start != nil || r.TimeMsg != "" {
		t.Errorf("typed times changed: %+v", r.Clip)
	}
	if r.Formats == nil || r.Files == nil {
		t.Errorf("formats/files must be [] not null: %#v %#v", r.Formats, r.Files)
	}
}

func TestEmptyShelfBootsAsEmptyList(t *testing.T) {
	c, _ := newTestCore(t)
	if got := c.Boot().Shelf.Clips; got == nil || len(got) != 0 {
		t.Errorf("clips = %#v, want empty non-nil", got)
	}
}

func TestSaveRejectsMadeLaneAndKeepsExistingResult(t *testing.T) {
	c, _ := newTestCore(t)
	if err := c.Save([]shelf.Clip{{ID: "a", Lane: shelf.LaneReady, URL: "u"}}); err != nil {
		t.Fatal(err)
	}
	if err := c.store.Finish("a", "done", "all good", []shelf.File{{Format: "gif", Path: "x.gif", Size: 3}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Save([]shelf.Clip{{ID: "a", Lane: shelf.LaneMade, URL: "u"}}); err == nil {
		t.Error("Save accepted the made lane")
	}
	// A stale page sends a made clip back as ready, with a forged result, in a batch with a new clip.
	if err := c.Save([]shelf.Clip{{ID: "n", Lane: shelf.LaneLater, URL: "u"}, {ID: "a", Lane: shelf.LaneReady, URL: "u", ResultKind: "failed", ResultMessage: "forged"}}); err == nil {
		t.Error("Save pulled a made clip back to ready")
	}
	all, _ := c.store.Load()
	if len(all) != 1 {
		t.Fatalf("clips = %+v, want only the made one (batch must not be half written)", all)
	}
	g := all[0]
	if g.Lane != shelf.LaneMade || g.ResultKind != "done" || g.ResultMessage != "all good" ||
		len(g.Files) != 1 || g.MadeAt.IsZero() || g.CreatedAt.IsZero() {
		t.Errorf("made clip changed: %+v", g)
	}
}

func TestSaveKeepsCreatedTimeOfAnExistingRow(t *testing.T) {
	c, _ := newTestCore(t)
	c.Save([]shelf.Clip{{ID: "a", Lane: shelf.LaneLater, URL: "u"}})
	before, _ := c.store.Load()
	if err := c.Save([]shelf.Clip{{ID: "a", Lane: shelf.LaneReady, URL: "u", StartText: "1"}}); err != nil {
		t.Fatal(err)
	}
	after, _ := c.store.Load()
	if len(after) != 1 || after[0].Lane != shelf.LaneReady || after[0].StartText != "1" || !after[0].CreatedAt.Equal(before[0].CreatedAt) {
		t.Errorf("before %+v, after %+v", before, after)
	}
}

func TestIDsFromThePageCannotEscapeScratch(t *testing.T) {
	c, _ := newTestCore(t)
	dir := filepath.Dir(c.settingsPath)
	for _, id := range []string{"..", ".", "a/b", `a\b`, "", "../keep"} {
		if err := c.Save([]shelf.Clip{{ID: id, Lane: shelf.LaneReady, URL: "u"}}); err == nil {
			t.Errorf("Save accepted ID %q", id)
		}
		c.Remove(id)
	}
	for _, f := range []string{"settings.json", "shelf.db"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s did not survive: %v", f, err)
		}
	}
	if all, _ := c.store.Load(); len(all) != 0 {
		t.Errorf("clips = %+v", all)
	}
}

func TestShutdownClosesTheStoreQuietly(t *testing.T) {
	c, rec := newTestCore(t)
	c.Shutdown(time.Second)
	c.Shutdown(time.Second) // a second call is harmless
	if err := c.store.Put(shelf.Clip{ID: "late", Lane: shelf.LaneLater}); err == nil {
		t.Error("Put after Shutdown succeeded")
	}
	c.storeFailed(errors.New("late write")) // the path a late lookup would take
	for _, e := range rec.all() {
		if e.name == "shelf:notice" {
			t.Error("sent a notice after Shutdown")
		}
	}
}

func TestSaveAcceptsANewRowForMakeAgain(t *testing.T) {
	c, _ := newTestCore(t)
	c.Save([]shelf.Clip{{ID: "a", Lane: shelf.LaneReady, URL: "u"}})
	c.store.Finish("a", "done", "", nil)
	if err := c.Save([]shelf.Clip{{ID: "b", Lane: shelf.LaneReady, URL: "u", ResultKind: "done", Files: []shelf.File{{Path: "forged"}}}}); err != nil {
		t.Fatal(err)
	}
	all, _ := c.store.Load()
	if len(all) != 2 || all[0].ID != "b" || all[0].Lane != shelf.LaneReady || all[0].ResultKind != "" || len(all[0].Files) != 0 ||
		all[1].ID != "a" || all[1].Lane != shelf.LaneMade {
		t.Errorf("clips = %+v", all)
	}
}

func TestRemoveDeletesScratchFolder(t *testing.T) {
	c, _ := newTestCore(t)
	c.Save([]shelf.Clip{{ID: "a", Lane: shelf.LaneLater, URL: "u"}})
	scratch := filepath.Join(filepath.Dir(c.settingsPath), "scratch", "a")
	os.MkdirAll(scratch, 0o755)
	os.WriteFile(filepath.Join(scratch, "dl.mp4"), []byte("x"), 0o644)
	keep := filepath.Join(filepath.Dir(c.settingsPath), "keep")
	os.MkdirAll(keep, 0o755)

	c.Remove("../keep") // an id from the page can't reach outside scratch
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("Remove escaped the scratch folder: %v", err)
	}
	c.Remove("a")
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Errorf("scratch folder still there: %v", err)
	}
	if all, _ := c.store.Load(); len(all) != 0 {
		t.Errorf("clips = %+v", all)
	}
}

func TestClearMadeKeepsOtherLanes(t *testing.T) {
	c, _ := newTestCore(t)
	c.Save([]shelf.Clip{{ID: "a", Lane: shelf.LaneReady, URL: "u"}, {ID: "b", Lane: shelf.LaneLater, URL: "u"}, {ID: "m", Lane: shelf.LaneReady, URL: "u"}})
	c.store.Finish("m", "done", "", nil)
	if err := c.ClearMade(); err != nil {
		t.Fatal(err)
	}
	all, _ := c.store.Load()
	if len(all) != 2 || all[0].ID != "a" || all[1].ID != "b" {
		t.Errorf("clips = %+v", all)
	}
}

func TestBootMarksMissingFilesAndAllowsEveryHistoryFolder(t *testing.T) {
	c, _ := newTestCore(t)
	gone := filepath.Join(t.TempDir(), "gone.gif")
	otherDir := t.TempDir()
	there := filepath.Join(otherDir, "there.gif")
	os.WriteFile(there, []byte("GIF89a"), 0o644)
	c.Save([]shelf.Clip{{ID: "a", Lane: shelf.LaneReady, URL: "u"}, {ID: "b", Lane: shelf.LaneReady, URL: "u"}})
	c.store.Finish("a", "done", "", []shelf.File{{Format: "gif", Path: gone, Size: 1}})
	c.store.Finish("b", "done", "", []shelf.File{{Format: "gif", Path: there, Size: 6}})

	n, _ := reboot(t, c)
	got := n.Boot().Shelf.Clips
	if len(got) != 2 {
		t.Fatalf("clips = %+v", got)
	}
	if !got[0].FilesMissing || got[0].Files[0].Path != gone {
		t.Errorf("missing file not flagged: %+v", got[0])
	}
	b := got[1]
	if b.FilesMissing || b.Files[0].URL == "" || !n.Files.Issued(there) {
		t.Errorf("file in another folder: %+v", b)
	}
}

func TestCorruptStoreStillBoots(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "shelf.db"), []byte("this is not a database, not even close, "+string(make([]byte, 2000))), 0o644)
	rec := &recorder{}
	c := New(rec.emit, debuglog.New(filepath.Join(dir, "logs"), false), filepath.Join(dir, "settings.json"), filepath.Join(dir, "bin"), deps.Pins{})
	t.Cleanup(func() { c.store.Close() })
	c.resolve = func() (Tools, error) { return Tools{"yt", "ff", "deno"}, nil }
	got := c.Boot().Shelf
	if got.Notice == "" || len(got.Clips) != 0 {
		t.Errorf("shelf = %+v, want a notice and no clips", got)
	}
	if err := c.Save([]shelf.Clip{{ID: "a", Lane: shelf.LaneReady, URL: "u"}}); err != nil {
		t.Errorf("Save on the fresh store: %v", err)
	}
}

func TestStoreWriteFailureSendsOneNotice(t *testing.T) {
	c, rec := newTestCore(t)
	c.store.Close() // every write now fails
	for range 2 {
		if err := c.Save([]shelf.Clip{{ID: "a", Lane: shelf.LaneReady, URL: "u"}}); err == nil {
			t.Error("Save on a closed store = nil")
		}
	}
	var notices []any
	for _, e := range rec.all() {
		if e.name == "shelf:notice" {
			notices = append(notices, e.data)
		}
	}
	want := Message{"Couldn't save your shelf just now. It will keep working until you close the app."}
	if len(notices) != 1 || notices[0] != want {
		t.Errorf("notices = %v, want one %v", notices, want)
	}
}
