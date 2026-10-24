package service

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"github.com/op/go-logging"
)

func TestRestoreTwoDifferentNativeBackups(t *testing.T) {
	for _, protocol := range []model.Protocol{model.VLESS, model.Shadowsocks} {
		for _, canonical := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/canonical=%t", protocol, canonical), func(t *testing.T) {
				logger.InitLogger(logging.CRITICAL)
				dir := t.TempDir()
				t.Setenv("VPNUI_DB_FOLDER", dir)
				t.Setenv("VPNUI_OFFLINE", "1")
				t.Setenv("VPNUI_NODE_AGENT", "1")
				t.Cleanup(func() { _ = database.CloseDB() })
				backups := []string{}
				for i := 1; i <= 2; i++ {
					file := filepath.Join(dir, fmt.Sprintf("backup-%d.db", i))
					if err := database.InitDB(config.GetDBPath()); err != nil {
						t.Fatal(err)
					}
					ib := model.Inbound{UserId: 1, Tag: fmt.Sprintf("backup-%d", i), Remark: fmt.Sprintf("Group %d", i), Port: 46000 + i, Protocol: protocol, Settings: fmt.Sprintf(`{"method":"chacha20-ietf-poly1305","password":"listener-password","clients":[{"password":"preserved-password","id":"11111111-1111-4111-8111-11111111111%d","email":"backup-%d","enable":true}]}`, i, i), StreamSettings: `{}`}
					if err := database.GetDB().Create(&ib).Error; err != nil {
						t.Fatal(err)
					}
					// Central counters can legitimately have no legacy home inbound.
					if err := database.GetDB().Create(&xray.ClientTraffic{Email: fmt.Sprintf("backup-%d", i), InboundId: 0, Up: int64(i) * 12345, Down: int64(i) * 67890, AllTime: int64(i) * 99999, Total: 10 * gb, Enable: true}).Error; err != nil {
						t.Fatal(err)
					}
					if err := SetNodeCorePaused("xray", true); err != nil {
						t.Fatal(err)
					}
					if canonical {
						(&AccountService{}).MigrationAccounts()
						if err := (&GroupService{}).Migrate(); err != nil {
							t.Fatal(err)
						}
					}
					if err := database.CloseDB(); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(config.GetDBPath(), file); err != nil {
						t.Fatal(err)
					}
					backups = append(backups, file)
				}
				if err := database.InitDB(config.GetDBPath()); err != nil {
					t.Fatal(err)
				}
				svc := &ServerService{}
				for i, file := range backups {
					src, err := os.Open(file)
					if err != nil {
						t.Fatal(err)
					}
					err = svc.ImportDB(src)
					src.Close()
					if err != nil {
						t.Fatal(err)
					}
					var ibs []model.Inbound
					if err := database.GetDB().Find(&ibs).Error; err != nil {
						t.Fatal(err)
					}
					if len(ibs) != 1 || ibs[0].Tag != fmt.Sprintf("backup-%d", i+1) {
						t.Fatal("restore did not replace inbounds", ibs)
					}
					var accounts []model.Account
					if err := database.GetDB().Find(&accounts).Error; err != nil {
						t.Fatal(err)
					}
					if len(accounts) != 1 || accounts[0].Email != fmt.Sprintf("backup-%d", i+1) {
						t.Fatalf("restore %d did not publish restored customers: %+v", i+1, accounts)
					}
					var usage xray.ClientTraffic
					if err := database.GetDB().Where("email = ?", accounts[0].Email).First(&usage).Error; err != nil {
						t.Fatal("native restore deleted shared traffic", err)
					}
					if usage.Up != int64(i+1)*12345 || usage.Down != int64(i+1)*67890 || usage.AllTime != int64(i+1)*99999 || usage.Total != 10*gb || !usage.Enable {
						t.Fatalf("native restore changed shared traffic or quota: %+v", usage)
					}
					if !GroupsMigrated(database.GetDB()) || !AdminGroupsMigrated(database.GetDB()) {
						t.Fatal("restored backup group migrations were not completed")
					}
					var groups []model.ClientGroup
					if err := database.GetDB().Find(&groups).Error; err != nil {
						t.Fatal(err)
					}
					if len(groups) != 1 || !strings.Contains(groups[0].Name, fmt.Sprintf("Group %d", i+1)) {
						t.Fatal("restore left the wrong groups", groups)
					}
					var memberships []model.AccountGroup
					if err := database.GetDB().Find(&memberships).Error; err != nil {
						t.Fatal(err)
					}
					if len(memberships) != 1 || memberships[0].AccountID != accounts[0].Id || memberships[0].GroupID != groups[0].ID {
						t.Fatal("restored customer did not retain group membership", memberships)
					}
				}
			})
		}
	}
}

func TestRestoreMigrationFailureReopensPreviousDatabaseAndAllowsRetry(t *testing.T) {
	logger.InitLogger(logging.CRITICAL)
	dir := t.TempDir()
	t.Setenv("VPNUI_DB_FOLDER", dir)
	t.Setenv("VPNUI_OFFLINE", "1")
	t.Setenv("VPNUI_NODE_AGENT", "1")
	t.Cleanup(func() { _ = database.CloseDB() })
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	seed := func(tag string) {
		t.Helper()
		ib := model.Inbound{UserId: 1, Tag: tag, Port: 46101, Protocol: model.VLESS, Settings: fmt.Sprintf(`{"clients":[{"id":"11111111-1111-4111-8111-111111111111","email":"%s","enable":true}]}`, tag), StreamSettings: `{}`}
		if err := database.GetDB().Create(&ib).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.GetDB().Create(&model.InboundAccess{UserId: 1, InboundId: ib.Id}).Error; err != nil {
			t.Fatal(err)
		}
		if err := SetNodeCorePaused("xray", true); err != nil {
			t.Fatal(err)
		}
	}
	seed("replacement")
	svc := &ServerService{}
	valid, err := svc.GetDb()
	if err != nil {
		t.Fatal(err)
	}
	validPath := filepath.Join(dir, "valid.db")
	if err := os.WriteFile(validPath, valid, 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Exec(`CREATE TRIGGER reject_restore_grants BEFORE INSERT ON admin_group_accesses BEGIN SELECT RAISE(ABORT,'rejected restore grant'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.CloseDB(); err != nil {
		t.Fatal(err)
	}
	rejected := filepath.Join(dir, "rejected.db")
	if err := os.Rename(config.GetDBPath(), rejected); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	seed("previous")
	(&AccountService{}).MigrationAccounts()
	if err := (&GroupService{}).Migrate(); err != nil {
		t.Fatal(err)
	}
	// A structurally valid SQLite file can still have an incompatible schema.
	schemaPath := filepath.Join(dir, "incompatible.db")
	if err := os.WriteFile(schemaPath, valid, 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite3", schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"DROP TABLE accounts", "CREATE VIEW accounts AS SELECT 1 AS id"} {
		if _, err := raw.Exec(statement); err != nil {
			raw.Close()
			t.Fatal(err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	broken, err := os.Open(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	importErr := svc.ImportDB(broken)
	broken.Close()
	if importErr == nil {
		t.Fatal("incompatible schema was accepted")
	}
	var previous []model.Account
	if err := database.GetDB().Find(&previous).Error; err != nil {
		t.Fatal("schema failure left the database closed", err)
	}
	if len(previous) != 1 || previous[0].Email != "previous" {
		t.Fatal("schema failure lost the previous customer", previous)
	}
	src, err := os.Open(rejected)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.ImportDB(src)
	src.Close()
	if err == nil || !strings.Contains(err.Error(), "rejected restore grant") {
		t.Fatal("migration failure was not returned", err)
	}
	var accounts []model.Account
	if err := database.GetDB().Find(&accounts).Error; err != nil {
		t.Fatal("previous database was left closed", err)
	}
	if len(accounts) != 1 || accounts[0].Email != "previous" {
		t.Fatal("failed restore lost the previous customer", accounts)
	}
	if err := database.GetDB().Create(&model.Setting{Key: "restoreRecoveryWritable", Value: "yes"}).Error; err != nil {
		t.Fatal("recovered database cannot be written", err)
	}
	src, err = os.Open(validPath)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.ImportDB(src)
	src.Close()
	if err != nil {
		t.Fatal("retry failed", err)
	}
	accounts = nil
	if err := database.GetDB().Find(&accounts).Error; err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].Email != "replacement" {
		t.Fatal("retry did not restore the replacement customer", accounts)
	}
	invalidPath := filepath.Join(dir, "invalid.db")
	if err := os.WriteFile(invalidPath, []byte("this is not SQLite"), 0600); err != nil {
		t.Fatal(err)
	}
	src, err = os.Open(invalidPath)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.ImportDB(src)
	src.Close()
	if err == nil {
		t.Fatal("invalid upload was accepted")
	}
	accounts = nil
	if err := database.GetDB().Find(&accounts).Error; err != nil {
		t.Fatal("validation failure closed the current database", err)
	}
	if len(accounts) != 1 || accounts[0].Email != "replacement" {
		t.Fatal("invalid upload changed current customers", accounts)
	}
}
