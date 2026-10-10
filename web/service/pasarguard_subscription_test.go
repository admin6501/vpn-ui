package service

import (
	"github.com/mhsanaei/3x-ui/v2/compat/pasarguard"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"testing"
	"time"
)

func TestPasarGuardSubscriptionDisplayKeepsNativeIDAndScopedRows(t *testing.T) {
	newAccountsDB(t)
	db := database.GetDB()
	created := time.Now().Add(-time.Hour)
	if err := db.Create(&model.PasarGuardSecret{ID: 1, SigningKey: "test-imported-secret"}).Error; err != nil {
		t.Fatal(err)
	}
	bindings := []model.PasarGuardSubscription{{SourceUserID: 101, AccountID: 1, Username: "owned", SubIDAtImport: "native-owned", CreatedAt: created}, {SourceUserID: 102, AccountID: 2, Username: "other-owner", SubIDAtImport: "native-other", CreatedAt: created}}
	if err := db.Create(&bindings).Error; err != nil {
		t.Fatal(err)
	}
	rows := []AccountRow{{Id: 1, SubID: "native-owned"}, {Id: 3, SubID: "native-new"}, {Id: 2, SubID: "rotated-other"}}
	if err := attachImportedSubscriptionTokens(db, rows); err != nil {
		t.Fatal(err)
	}
	if rows[0].SubID != "native-owned" || rows[0].LegacySubID == "" || rows[1].LegacySubID != "" || rows[2].LegacySubID != "" {
		t.Fatal("native IDs changed or revoked/unimported binding exposed")
	}
	payload, err := pasarguard.Verify(rows[0].LegacySubID, "test-imported-secret", time.Now())
	if err != nil || payload.UserID != 101 {
		t.Fatal("wrong imported identity", err)
	}
}
