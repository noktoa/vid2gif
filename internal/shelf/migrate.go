package shelf

import (
	"database/sql"
	"fmt"
)

// migrations run in order; PRAGMA user_version records how many have run.
// Never edit one that has shipped: append a new one.
var migrations = []func(*sql.Tx) error{
	func(tx *sql.Tx) error {
		_, err := tx.Exec(`CREATE TABLE clips (
			id TEXT PRIMARY KEY,
			lane TEXT NOT NULL,
			position INTEGER NOT NULL,
			created_at INTEGER NOT NULL DEFAULT 0,
			made_at INTEGER NOT NULL DEFAULT 0,
			url TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			thumbnail_url TEXT NOT NULL DEFAULT '',
			duration_sec REAL NOT NULL DEFAULT 0,
			source TEXT NOT NULL DEFAULT '',
			unavailable INTEGER NOT NULL DEFAULT 0,
			reason TEXT NOT NULL DEFAULT '',
			start_text TEXT NOT NULL DEFAULT '',
			end_text TEXT NOT NULL DEFAULT '',
			end_kind TEXT NOT NULL DEFAULT '',
			start_sec REAL,
			end_sec REAL NOT NULL DEFAULT 0,
			time_msg TEXT NOT NULL DEFAULT '',
			checked TEXT NOT NULL DEFAULT '',
			formats TEXT NOT NULL DEFAULT '[]',
			quality TEXT NOT NULL DEFAULT '',
			result_kind TEXT NOT NULL DEFAULT '',
			result_message TEXT NOT NULL DEFAULT '',
			files TEXT NOT NULL DEFAULT '[]'
		)`)
		return err
	},
}

// migrate brings db up to date, one transaction per migration. It fails if
// the file was written by a newer app.
func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v > len(migrations) {
		return fmt.Errorf("shelf schema version %d is newer than this app knows (%d)", v, len(migrations))
	}
	for i := v; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if err := migrations[i](tx); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
