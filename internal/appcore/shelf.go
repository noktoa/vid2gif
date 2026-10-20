package appcore

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"vid2gif/internal/shelf"
)

const storeMsg = "Couldn't save your shelf just now. It will keep working until you close the app."

// ShelfClip is a saved clip as the page sees it. The outer Files shadows
// shelf.Clip.Files in JSON, as Status does.
type ShelfClip struct {
	shelf.Clip
	Files        []PageFile `json:"files"`
	FilesMissing bool       `json:"filesMissing"`
}

// ShelfState is the saved shelf at launch. Notice is "" unless the saved file
// could not be used.
type ShelfState struct {
	Clips  []ShelfClip `json:"clips"`
	Notice string      `json:"notice"`
}

// openShelf opens dir's shelf.db. If that fails the app keeps working with an
// in-memory shelf, and the notice says the saved one is gone.
func (c *Core) openShelf(dir string) {
	c.scratchDir = filepath.Join(dir, "scratch")
	s, notice, err := shelf.Open(filepath.Join(dir, "shelf.db"))
	if err != nil {
		c.Log.Printf("shelf: %v", err)
		notice = storeMsg
		if s, _, err = shelf.Open(":memory:"); err != nil {
			panic(err) // an in-memory database failing means the SQLite driver is broken
		}
	}
	c.store, c.shelfNotice = s, notice
}

// storeFailed logs a store error and tells the page once; the shelf then
// carries on in memory for the page, though later writes may still fail.
func (c *Core) storeFailed(err error) error {
	c.Log.Printf("shelf: %v", err)
	c.noticeOnce.Do(func() { c.send("shelf:notice", Message{storeMsg}) })
	return err
}

// validID is a whitelist (shelf.NewID gives hex): an ID from the page is also a
// folder name under scratch, so "." ".." and separators must never pass.
func validID(id string) bool {
	for _, r := range id {
		if r != '-' && r != '_' && (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return id != ""
}

// Save writes clips in the ready and later lanes. An existing row keeps its
// result, made time and created time: only Go sets those.
func (c *Core) Save(clips []shelf.Clip) error {
	for _, cl := range clips {
		if cl.Lane != shelf.LaneReady && cl.Lane != shelf.LaneLater {
			return errors.New("That clip can't be saved there.")
		}
		if !validID(cl.ID) {
			return errors.New("That clip can't be saved.")
		}
	}
	old, err := c.store.Load()
	if err != nil {
		return c.storeFailed(err)
	}
	for _, cl := range clips { // a stale page must not pull a made clip back
		if i := slices.IndexFunc(old, func(o shelf.Clip) bool { return o.ID == cl.ID }); i >= 0 && old[i].Lane == shelf.LaneMade {
			return errors.New("That clip can't be saved there.")
		}
	}
	for _, cl := range clips {
		cl.CreatedAt, cl.MadeAt, cl.ResultKind, cl.ResultMessage, cl.Files = time.Time{}, time.Time{}, "", "", nil
		if i := slices.IndexFunc(old, func(o shelf.Clip) bool { return o.ID == cl.ID }); i >= 0 {
			o := old[i]
			cl.CreatedAt, cl.MadeAt, cl.ResultKind, cl.ResultMessage, cl.Files = o.CreatedAt, o.MadeAt, o.ResultKind, o.ResultMessage, o.Files
		}
		if err := c.store.Put(cl); err != nil {
			return c.storeFailed(err)
		}
	}
	return nil
}

// Remove drops a clip and its kept download.
func (c *Core) Remove(id string) {
	if !validID(id) {
		return
	}
	if err := c.store.Remove(id); err != nil {
		c.storeFailed(err)
	}
	if err := os.RemoveAll(filepath.Join(c.scratchDir, id)); err != nil {
		c.Log.Printf("shelf: remove scratch %s: %v", id, err)
	}
}

func (c *Core) ClearMade() error {
	if err := c.store.ClearMade(); err != nil {
		return c.storeFailed(err)
	}
	return nil
}

// shelfState loads the saved clips for Boot. Every folder a made file sits in
// is allowed, so its preview plays after a relaunch.
func (c *Core) shelfState() ShelfState {
	st := ShelfState{Clips: []ShelfClip{}, Notice: c.shelfNotice}
	saved, err := c.store.Load()
	if err != nil {
		c.Log.Printf("shelf: %v", err)
		st.Notice = storeMsg
		return st
	}
	for _, cl := range saved {
		sc := ShelfClip{Clip: cl, Files: []PageFile{}}
		if sc.Formats == nil {
			sc.Formats = []string{}
		}
		for _, f := range cl.Files {
			c.Files.Allow(filepath.Dir(f.Path))
			pf := PageFile{Format: f.Format, Path: f.Path, Size: f.Size}
			if _, err := c.stat(f.Path); err != nil {
				sc.FilesMissing = true
			} else {
				pf.URL, _ = c.Files.URL(f.Path)
			}
			sc.Files = append(sc.Files, pf)
		}
		st.Clips = append(st.Clips, sc)
	}
	return st
}
