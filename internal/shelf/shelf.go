// Package shelf saves the clips on the shelf (ready, later, made before) in a
// small SQLite file so they survive closing the app.
package shelf

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Lane string

const (
	LaneReady Lane = "ready"
	LaneLater Lane = "later"
	LaneMade  Lane = "made"
)

// freshNotice is shown when the saved file could not be used.
const freshNotice = "Couldn't open your saved shelf. Starting fresh. A copy was kept."

type File struct {
	Format string `json:"format"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
}

type Clip struct {
	ID            string    `json:"id"`
	Lane          Lane      `json:"lane"`
	URL           string    `json:"url"`
	Title         string    `json:"title"`
	ThumbnailURL  string    `json:"thumbnailUrl"`
	DurationSec   float64   `json:"durationSec"`
	Source        string    `json:"source"`
	Unavailable   bool      `json:"unavailable"`
	Reason        string    `json:"reason"`
	StartText     string    `json:"startText"`
	EndText       string    `json:"endText"`
	EndKind       string    `json:"endKind"`
	Start         *float64  `json:"start"`
	End           float64   `json:"end"`
	TimeMsg       string    `json:"timeMsg"`
	Checked       string    `json:"checked"`
	Formats       []string  `json:"formats"`
	Quality       string    `json:"quality"`
	ResultKind    string    `json:"resultKind"`
	ResultMessage string    `json:"resultMessage"`
	Files         []File    `json:"files"`
	CreatedAt     time.Time `json:"createdAt"`
	MadeAt        time.Time `json:"madeAt"`
}

// Store is safe for concurrent use. Every write is one transaction.
type Store struct {
	mu   sync.Mutex
	path string
	db   *sql.DB
}

// NewID returns an ID that stays unique across launches.
func NewID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one connection keeps the pragmas and writes serial
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Open opens the store at path. If the file cannot be used (damaged, or from
// a newer app) it is renamed to <path>.bad-<unix> rather than deleted, a fresh
// store is started, and notice says so.
func Open(path string) (s *Store, notice string, err error) {
	db, err := openDB(path)
	if err != nil {
		stamp := fmt.Sprintf(".bad-%d", time.Now().Unix())
		for _, suf := range []string{"", "-wal", "-shm"} {
			if e := os.Rename(path+suf, path+stamp+suf); e != nil && suf == "" {
				return nil, "", fmt.Errorf("open shelf: %w", err)
			}
		}
		if db, err = openDB(path); err != nil {
			return nil, "", fmt.Errorf("open fresh shelf: %w", err)
		}
		notice = freshNotice
	}
	return &Store{path: path, db: db}, notice, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

const cols = `id, lane, position, created_at, made_at, url, title, thumbnail_url, duration_sec, source,
	unavailable, reason, start_text, end_text, end_kind, start_sec, end_sec, time_msg, checked,
	formats, quality, result_kind, result_message, files`

// Load returns every clip: ready, then later, then made, each in position order.
func (s *Store) Load() ([]Clip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT ` + cols + ` FROM clips ORDER BY
		CASE lane WHEN 'ready' THEN 0 WHEN 'later' THEN 1 ELSE 2 END, position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Clip
	for rows.Next() {
		var c Clip
		var pos int
		var created, made int64
		var start sql.NullFloat64
		var formats, files string
		if err := rows.Scan(&c.ID, &c.Lane, &pos, &created, &made, &c.URL, &c.Title, &c.ThumbnailURL,
			&c.DurationSec, &c.Source, &c.Unavailable, &c.Reason, &c.StartText, &c.EndText, &c.EndKind,
			&start, &c.End, &c.TimeMsg, &c.Checked, &formats, &c.Quality, &c.ResultKind,
			&c.ResultMessage, &files); err != nil {
			return nil, err
		}
		if start.Valid {
			c.Start = &start.Float64
		}
		if created != 0 {
			c.CreatedAt = time.UnixMilli(created)
		}
		if made != 0 {
			c.MadeAt = time.UnixMilli(made)
		}
		if err := json.Unmarshal([]byte(formats), &c.Formats); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(files), &c.Files); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func millis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// Put inserts or updates a clip. A new clip, or one whose lane changed, goes to
// the end of its lane; otherwise it keeps its place.
func (s *Store) Put(c Clip) error {
	formats, err := json.Marshal(c.Formats)
	if err != nil {
		return err
	}
	files, err := json.Marshal(c.Files)
	if err != nil {
		return err
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	return s.write(func(tx *sql.Tx) error {
		var oldLane string
		var pos int
		err := tx.QueryRow(`SELECT lane, position FROM clips WHERE id = ?`, c.ID).Scan(&oldLane, &pos)
		if err == sql.ErrNoRows || (err == nil && oldLane != string(c.Lane)) {
			pos, err = nextPosition(tx, c.Lane)
		}
		if err != nil {
			return err
		}
		var start any
		if c.Start != nil {
			start = *c.Start
		}
		_, err = tx.Exec(`INSERT OR REPLACE INTO clips (`+cols+`)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			c.ID, string(c.Lane), pos, millis(c.CreatedAt), millis(c.MadeAt), c.URL, c.Title, c.ThumbnailURL,
			c.DurationSec, c.Source, c.Unavailable, c.Reason, c.StartText, c.EndText, c.EndKind,
			start, c.End, c.TimeMsg, c.Checked, string(formats), c.Quality, c.ResultKind,
			c.ResultMessage, string(files))
		return err
	})
}

func nextPosition(tx *sql.Tx, lane Lane) (int, error) {
	var pos int
	err := tx.QueryRow(`SELECT COALESCE(MAX(position), 0) + 1 FROM clips WHERE lane = ?`, string(lane)).Scan(&pos)
	return pos, err
}

// Finish records a clip's result and moves it to the end of made.
func (s *Store) Finish(id, kind, message string, files []File) error {
	js, err := json.Marshal(files)
	if err != nil {
		return err
	}
	return s.write(func(tx *sql.Tx) error {
		pos, err := nextPosition(tx, LaneMade)
		if err != nil {
			return err
		}
		res, err := tx.Exec(`UPDATE clips SET lane = 'made', position = ?, made_at = ?,
			result_kind = ?, result_message = ?, files = ? WHERE id = ?`,
			pos, time.Now().UnixMilli(), kind, message, string(js), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("finish: no clip %q", id)
		}
		return nil
	})
}

func (s *Store) Remove(id string) error {
	return s.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM clips WHERE id = ?`, id)
		return err
	})
}

func (s *Store) ClearMade() error {
	return s.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM clips WHERE lane = 'made'`)
		return err
	})
}

// Wipe closes the store, deletes its files, and reopens it empty.
func (s *Store) Wipe() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Close()
	for _, suf := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(s.path + suf); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	db, err := openDB(s.path)
	if err != nil {
		return err
	}
	s.db = db
	return nil
}

func (s *Store) write(fn func(*sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
