package service

import (
	"math"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestImportMigrationUsageDisabledAndIdempotent(t *testing.T) {
	svc := newAccountsDB(t)
	_, err := svc.SaveAccountWithoutInbound(map[string]any{"email": "migration-user", "enable": false, "totalGB": float64(1000), "expiryTime": float64(0)})
	if err != nil {
		t.Fatal(err)
	}
	if err = ImportMigrationUsage("migration-user", 11, 22, 55); err != nil {
		t.Fatal(err)
	}
	if err = ImportMigrationUsage("migration-user", 11, 22, 55); err != nil {
		t.Fatalf("identical retry: %v", err)
	}
	var traffic xray.ClientTraffic
	if err = database.GetDB().Where("email = ?", "migration-user").First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Up != 11 || traffic.Down != 22 || traffic.AllTime != 55 || traffic.Enable {
		t.Fatalf("unexpected counters: %+v", traffic)
	}
	if err = ImportMigrationUsage("migration-user", 12, 22, 55); err == nil {
		t.Fatal("existing counters were overwritten")
	}
	if err = ImportMigrationUsage("migration-user", -1, 0, 0); err == nil {
		t.Fatal("negative usage accepted")
	}
	if err = ImportMigrationUsage("migration-user", math.MaxInt64, 1, 0); err == nil {
		t.Fatal("overflow accepted")
	}
	if err = database.GetDB().Model(&traffic).Update("enable", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = ImportMigrationUsage("migration-user", 11, 22, 55); err == nil {
		t.Fatal("enabled traffic accepted")
	}
}
