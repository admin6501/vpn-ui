package database

import (
	"errors"
	"time"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
)

// SnapshotTransaction retries a database-only transaction when a concurrent
// WAL commit invalidates the snapshot before its first write. SQLite cannot
// wait out SQLITE_BUSY_SNAPSHOT. The entire transaction must read again.
func SnapshotTransaction(db *gorm.DB, update func(*gorm.DB) error) error {
	const maxAttempts = 4
	for attempt := 1; ; attempt++ {
		err := db.Transaction(update)
		var sqliteErr sqlite3.Error
		if attempt >= maxAttempts || !errors.As(err, &sqliteErr) || sqliteErr.ExtendedCode != sqlite3.ErrBusySnapshot {
			return err
		}
		time.Sleep(time.Duration(attempt*15) * time.Millisecond)
	}
}
