package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestClientActivityDetailsUpgradePreservesExistingRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { CloseDB() })
	account := model.Account{Email: "old-account", TotalGB: 10, CreatedAt: time.Now().UnixMilli()}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	// Reproduce the two audit tables and triggers from the previous release.
	for _, statement := range []string{
		"DROP TRIGGER client_history_updated",
		"DROP TRIGGER client_history_created",
		"DROP TRIGGER panel_client_activity",
		"DROP TRIGGER panel_payment_reminder_changed",
		"ALTER TABLE account_events DROP COLUMN details",
		"ALTER TABLE panel_client_events DROP COLUMN details",
		`CREATE TRIGGER client_history_updated AFTER UPDATE ON accounts BEGIN
			INSERT INTO account_events(account_id,at,kind,changes) VALUES(NEW.id,NEW.created_at,'updated','quota');
		END`,
		`CREATE TRIGGER panel_client_activity AFTER INSERT ON account_events BEGIN
			INSERT INTO panel_client_events(account_id,owner_id,owner,email,kind,changes,at)
			SELECT id,0,'',email,NEW.kind,NEW.changes,NEW.at FROM accounts WHERE id=NEW.account_id;
		END`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&account).Update("total_gb", 20).Error; err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&account).Update("total_gb", 30).Error; err != nil {
		t.Fatal(err)
	}
	var rows []model.PanelClientEvent
	if err := db.Where("account_id=? AND kind='updated'", account.Id).Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Changes != "quota" || rows[0].Details != nil {
		t.Fatalf("previous audit row was not preserved: %+v", rows)
	}
	if details := rows[1].Details; details == nil || details.Quota == nil ||
		*details.Quota != (model.ClientValueChange{Before: 20, After: 30}) {
		t.Fatalf("trigger was not upgraded: %+v", details)
	}
}
