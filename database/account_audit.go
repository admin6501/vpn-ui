package database

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AccountAudit carries the identity of the authenticated panel user and ties
// the separate writes of a single request together.
type AccountAudit struct {
	Operation string
	ActorID   int
}

const groupDefinitionAuditPrefix = "group-definition:"

// GroupDefinitionAudit records the administrator who edited a group while
// distinguishing inherited membership changes from direct client edits.
func GroupDefinitionAudit(actorID int) AccountAudit {
	return AccountAudit{ActorID: actorID, Operation: groupDefinitionAuditPrefix + uuid.NewString()}
}

// HoldAccountAudit is used only inside a write transaction. SQLite serializes
// writers, so each operation marker is isolated and a rollback restores it.
func HoldAccountAudit(tx *gorm.DB, operation ...AccountAudit) (func() error, error) {
	audit := AccountAudit{}
	if len(operation) > 0 {
		audit = operation[0]
	}
	if audit.Operation == "" {
		audit.Operation = uuid.NewString()
	}
	result := tx.Exec("UPDATE account_audit_context SET operation=?, actor_id=? WHERE id=1 AND operation=''", audit.Operation, audit.ActorID)
	if result.Error != nil {
		return nil, result.Error
	}
	return func() error {
		if result.RowsAffected == 0 {
			return nil
		}
		return tx.Exec("UPDATE account_audit_context SET operation='', actor_id=0 WHERE id=1").Error
	}, nil
}

func AccountAuditTransaction(db *gorm.DB, work func(*gorm.DB) error, operation ...AccountAudit) error {
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
