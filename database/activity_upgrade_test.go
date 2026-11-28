package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
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
		"DROP TRIGGER client_history_remove",
		"DROP TRIGGER client_history_account_inbounds_insert",
		"DROP TRIGGER client_history_account_inbounds_delete",
		"DROP TRIGGER client_history_account_groups_insert",
		"DROP TRIGGER client_history_account_groups_delete",
		"DROP TRIGGER client_history_membership_status",
		"DROP TRIGGER assign_unattributed_client_owner",
		"DROP TRIGGER panel_client_activity",
		"DROP TRIGGER panel_payment_reminder_changed",
		"DROP INDEX idx_pending_payments_actor",
		"DROP INDEX idx_panel_payment_reminders_actor_id",
		"ALTER TABLE panel_payment_reminders DROP COLUMN actor_id",
		"ALTER TABLE accounts DROP COLUMN inferred_owner_id",
		"ALTER TABLE accounts DROP COLUMN auto_renew_actor_id",
		"DROP INDEX idx_panel_client_events_actor_id",
		"DROP INDEX idx_account_event_actor_history",
		"ALTER TABLE panel_client_events DROP COLUMN actor_id",
		"ALTER TABLE panel_client_events DROP COLUMN actor",
		"ALTER TABLE account_audit_context DROP COLUMN actor_id",
		"ALTER TABLE account_events DROP COLUMN actor_id",
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

func TestLegacyRenewalNoticesRecoverRecordedActorOnUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { CloseDB() })
	var owner model.User
	if err := db.Where("is_panel_owner").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	manager := model.User{Username: "original-creator", Enable: true, IsSuperAdmin: true}
	if err := db.Create(&manager).Error; err != nil {
		t.Fatal(err)
	}
	account := model.Account{Email: "old-renewal", OwnerAdminID: owner.Id, CreatedAt: time.Now().UnixMilli()}
	if err := AccountAuditTransaction(db, func(tx *gorm.DB) error {
		return tx.Create(&account).Error
	}, AccountAudit{Operation: "recorded-creation", ActorID: manager.Id}); err != nil {
		t.Fatal(err)
	}
	// The previous version put the actor in Activity and left account history
	// without that field. Its automatic renewals went to the stored panel owner.
	if err := db.Model(&model.AccountEvent{}).Where("account_id=?", account.Id).Update("actor_id", 0).Error; err != nil {
		t.Fatal(err)
	}
	old := model.PanelClientEvent{AccountID: account.Id, OwnerID: owner.Id, Owner: owner.Username,
		Email: account.Email, Kind: "autoRenewed", At: time.Now().UnixMilli()}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&old, old.Id).Error; err != nil || old.OwnerID != manager.Id || old.Owner != manager.Username {
		t.Fatalf("historical renewal was not attributed to its manager: %+v %v", old, err)
	}
	var creation model.AccountEvent
	if err := db.Where("account_id=? AND kind='created'", account.Id).First(&creation).Error; err != nil || creation.ActorID != manager.Id {
		t.Fatalf("creation provenance wasn't retained with history: %+v %v", creation, err)
	}
}

func TestLegacyActivityRecoversFallbackOwnerAndRenewalOperatorOnUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { CloseDB() })
	var owner model.User
	if err := db.Where("is_panel_owner").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	editor := model.User{Username: "unattributed-client-editor", Enable: true, IsReseller: true}
	operator := model.User{Username: "automatic-renewal-operator", Enable: true, IsSuperAdmin: true}
	for _, user := range []*model.User{&editor, &operator} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	created := time.Now().Add(-time.Minute).UnixMilli()
	account := model.Account{Email: "unattributed-legacy-client", OwnerAdminID: owner.Id, AutoRenew: true, CreatedAt: created}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	// The previous panel stored the editor in history without using it for ownership or notices.
	if err := db.Exec("DROP TRIGGER assign_unattributed_client_owner").Error; err != nil {
		t.Fatal(err)
	}
	for _, event := range []model.AccountEvent{
		{AccountID: account.Id, Kind: "updated", Changes: "comment", ActorID: editor.Id, At: created + 100},
		{AccountID: account.Id, Kind: "updated", Changes: "autoRenew", ActorID: operator.Id, At: created + 200},
		{AccountID: account.Id, Kind: "autoRenewed", At: created + 300},
	} {
		if err := db.Create(&event).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&account, account.Id).Error; err != nil || account.InferredOwnerID != editor.Id ||
		account.AutoRenewActorID != operator.Id {
		t.Fatalf("legacy account attribution was not recovered: %+v, %v", account, err)
	}
	var renewal model.PanelClientEvent
	if err := db.Where("account_id=? AND kind='autoRenewed'", account.Id).First(&renewal).Error; err != nil ||
		renewal.OwnerID != editor.Id || renewal.ActorID != operator.Id {
		t.Fatalf("historical renewal attribution was not recovered: %+v, %v", renewal, err)
	}
	var payment model.PanelPaymentReminder
	if err := db.First(&payment, account.Id).Error; err != nil || payment.ActorID != operator.Id {
		t.Fatalf("historical payment was not assigned to the renewal operator: %+v, %v", payment, err)
	}
}
