package database

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// HoldAccountAudit is used only inside a write transaction. SQLite serializes
// writers, so each operation marker is isolated and a rollback restores it.
func HoldAccountAudit(tx *gorm.DB, operation ...string) (func() error, error) {
	id := ""
	if len(operation) > 0 {
		id = operation[0]
	}
	if id == "" {
		id = uuid.NewString()
	}
	result := tx.Exec("UPDATE account_audit_context SET operation=? WHERE id=1 AND operation=''", id)
	if result.Error != nil {
		return nil, result.Error
	}
	return func() error {
		if result.RowsAffected == 0 {
			return nil
		}
		return tx.Exec("UPDATE account_audit_context SET operation='' WHERE id=1").Error
	}, nil
}

func AccountAuditTransaction(db *gorm.DB, work func(*gorm.DB) error, operation ...string) error {
	return SnapshotTransaction(db, func(tx *gorm.DB) error {
		release, err := HoldAccountAudit(tx, operation...)
		if err != nil {
			return err
		}
		if err := work(tx); err != nil {
			return err
		}
		return release()
	})
}
