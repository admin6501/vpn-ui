package database

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestIdentityIndexesUpgradeWithoutChangingHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"accounts", "client_traffics"} {
		if err := db.Exec("DROP INDEX idx_" + table + "_email_key").Error; err != nil {
			t.Fatal(err)
		}
	}
	accounts := []model.Account{{Email: " Mixed "}, {Email: "mixed"}}
	if err := db.Create(&accounts).Error; err != nil {
		t.Fatal(err)
	}
	traffic := xray.ClientTraffic{Email: " Mixed ", Up: 123, Down: 456, AllTime: 789}
	if err := db.Create(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	defer CloseDB()
	for _, table := range []string{"accounts", "client_traffics"} {
		var plan []struct{ Detail string }
		if err := db.Raw("EXPLAIN QUERY PLAN SELECT id FROM "+table+" WHERE LOWER(TRIM(email)) = ?", "mixed").Scan(&plan).Error; err != nil {
			t.Fatal(err)
		}
		details := fmt.Sprint(plan)
		if !strings.Contains(details, "SEARCH") || !strings.Contains(details, "idx_"+table+"_email_key") || strings.Contains(details, "SCAN") {
			t.Fatalf("%s identity lookup scans history: %s", table, details)
		}
	}
	var count int64
	db.Model(&model.Account{}).Where("LOWER(TRIM(email)) = ?", "mixed").Count(&count)
	if count != 2 {
		t.Fatalf("legacy identities changed: %d", count)
	}
	var after xray.ClientTraffic
	if err := db.First(&after, traffic.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.Email != traffic.Email || after.Up != 123 || after.Down != 456 || after.AllTime != 789 {
		t.Fatalf("history changed: %+v", after)
	}
}

func BenchmarkMembershipIdentityUpdates(b *testing.B) {
	if err := InitDB(filepath.Join(b.TempDir(), "panel.db")); err != nil {
		b.Fatal(err)
	}
	defer CloseDB()
	if err := db.Exec(`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<5000)
 INSERT INTO accounts(email) SELECT 'user'||n FROM seq`).Error; err != nil {
		b.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO account_inbounds(account_id,inbound_id) SELECT id,1 FROM accounts
 UNION ALL SELECT id,2 FROM accounts UNION ALL SELECT id,3 FROM accounts`).Error; err != nil {
		b.Fatal(err)
	}
	for _, indexed := range []bool{false, true} {
		name := "legacy-scan"
		if indexed {
			name = "indexed"
			if err := db.Exec("CREATE INDEX idx_accounts_email_key ON accounts(LOWER(TRIM(email)))").Error; err != nil {
				b.Fatal(err)
			}
		} else {
			if err := db.Exec("DROP INDEX idx_accounts_email_key").Error; err != nil {
				b.Fatal(err)
			}
		}
		b.Run(name, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				tx := db.Begin()
				for id := 1; id <= 5000; id++ {
					for inbound := 1; inbound <= 3; inbound++ {
						if err := tx.Exec(`UPDATE account_inbounds SET up=COALESCE(up,0)+1 WHERE inbound_id=?
    AND account_id IN (SELECT id FROM accounts WHERE LOWER(TRIM(email))=?)`, inbound, fmt.Sprintf("user%d", id)).Error; err != nil {
							tx.Rollback()
							b.Fatal(err)
						}
					}
				}
				if err := tx.Commit().Error; err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
