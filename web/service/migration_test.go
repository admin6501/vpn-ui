package service

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"

	"gorm.io/gorm"
)

func upsertSetting(t *testing.T, db *gorm.DB, key, value string) {
	t.Helper()
	s := &model.Setting{}
	err := db.Where("key = ?", key).First(s).Error
	switch {
	case database.IsNotFound(err):
		if e := db.Create(&model.Setting{Key: key, Value: value}).Error; e != nil {
			t.Fatalf("create setting %s: %v", key, e)
		}
	case err != nil:
		t.Fatalf("read setting %s: %v", key, err)
	default:
		s.Value = value
		if e := db.Save(s).Error; e != nil {
			t.Fatalf("save setting %s: %v", key, e)
		}
	}
}

func getSetting(t *testing.T, db *gorm.DB, key string) string {
	t.Helper()
	s := &model.Setting{}
	err := db.Where("key = ?", key).First(s).Error
	if database.IsNotFound(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("read setting %s: %v", key, err)
	}
	return s.Value
}

// A stock 3x-ui import must bring the operator's data across (superset schema +
// InitDB's adopt migrations) while keeping THIS panel reachable: its port and
// session secret must survive, but a data setting like the subscription title must
// come from the backup.
func TestImportForeignDBBringsDataAndPreservesPanelSettings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("VPNUI_DB_FOLDER", tmp) // config.GetDBPath() -> tmp/vpn-ui.db; no legacy paths

	// A foreign backup: one inbound + client, plus settings that differ from this
	// panel (webPort/secret) and one that is pure data (subTitle).
	foreignPath := filepath.Join(tmp, "foreign.db")
	if err := database.InitDB(foreignPath); err != nil {
		t.Fatalf("init foreign: %v", err)
	}
	fdb := database.GetDB()
	fin := &model.Inbound{
		Tag: "imported-tag", Port: 55055, Protocol: model.VMESS, Enable: true,
		Settings: `{"clients":[{"id":"x","email":"imported-client","enable":true}]}`,
	}
	if err := fdb.Create(fin).Error; err != nil {
		t.Fatalf("seed foreign inbound: %v", err)
	}
	if err := fdb.Create(&xray.ClientTraffic{InboundId: fin.Id, Email: "imported-client", Enable: true}).Error; err != nil {
		t.Fatalf("seed foreign client: %v", err)
	}
	upsertSetting(t, fdb, "webPort", "2053")
	upsertSetting(t, fdb, "secret", "foreign-secret-should-not-survive")
	upsertSetting(t, fdb, "subTitle", "from-3xui")
	if err := database.Checkpoint(); err != nil { // flush WAL so the file copy is complete
		t.Fatalf("checkpoint foreign: %v", err)
	}
	if err := database.CloseDB(); err != nil {
		t.Fatalf("close foreign: %v", err)
	}

	// THIS panel, with distinctive reachability settings that must be kept.
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatalf("init current: %v", err)
	}
	upsertSetting(t, database.GetDB(), "webPort", "8443")
	upsertSetting(t, database.GetDB(), "secret", "current-secret-keep-me")

	src, err := os.Open(foreignPath)
	if err != nil {
		t.Fatalf("open foreign: %v", err)
	}
	defer src.Close()

	var ss ServerService
	report, err := ss.ImportForeignDB(src, false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	db := database.GetDB()
	var inb int64
	db.Model(&model.Inbound{}).Where("tag = ?", "imported-tag").Count(&inb)
	if inb != 1 {
		t.Fatalf("imported inbound did not come across (found %d)", inb)
	}
	if got := getSetting(t, db, "webPort"); got != "8443" {
		t.Fatalf("panel webPort not preserved: got %q want 8443", got)
	}
	if got := getSetting(t, db, "secret"); got != "current-secret-keep-me" {
		t.Fatalf("panel secret not preserved: got %q", got)
	}
	if got := getSetting(t, db, "subTitle"); got != "from-3xui" {
		t.Fatalf("data setting subTitle did not come across: got %q", got)
	}
	if report == nil || report.Inbounds < 1 {
		t.Fatalf("report missing imported inbound: %+v", report)
	}
	found := false
	for _, k := range report.PreservedSettings {
		if k == "webPort" {
			found = true
		}
	}
	if !found {
		t.Fatalf("report should list webPort as preserved: %+v", report.PreservedSettings)
	}
}

// This fixture uses the upstream 3x-ui table layout and deliberately has no
// clients in inbound.settings. A VPN-UI-created fixture misses this regression.
func TestImportForeignDBProjectsStockClientMemberships(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPNUI_DB_FOLDER", dir)
	t.Setenv("VPNUI_OFFLINE", "1")
	t.Setenv("VPNUI_NODE_AGENT", "1")
	srcPath := filepath.Join(dir, "stock.db")
	stock, err := sql.Open("sqlite3", srcPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = stock.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT, password TEXT);
		INSERT INTO users VALUES (1, 'stock-admin', 'stock-password');
		CREATE TABLE settings (id INTEGER PRIMARY KEY, key TEXT, value TEXT);
		INSERT INTO settings VALUES (1, 'webPort', '2053'), (2, 'nodeCorePaused:xray', 'true');
		CREATE TABLE inbounds (
			id INTEGER PRIMARY KEY, user_id INTEGER, port INTEGER, protocol TEXT,
			tag TEXT UNIQUE, settings TEXT, stream_settings TEXT, sniffing TEXT, enable NUMERIC
		);
		INSERT INTO inbounds VALUES
			(1, 1, 5443, 'vless', 'in-5443-tcp', '{"decryption":"none"}', '{}', '{}', 1),
			(2, 1, 5444, 'vless', 'in-5444-tcp', '{"clients":[],"decryption":"none"}', '{}', '{}', 1);
		CREATE TABLE clients (
			id INTEGER PRIMARY KEY, email TEXT UNIQUE, uuid TEXT, sub_id TEXT, flow TEXT,
			total_gb INTEGER, expiry_time INTEGER, enable NUMERIC, tg_id INTEGER, comment TEXT
		);
		INSERT INTO clients VALUES
			(7, 'stock-client', '12345678-1234-4234-9234-123456789abc', 'stock-sub', '',
			 10737418240, 1900000000000, 1, 123456789, 'stock comment');
		CREATE TABLE client_inbounds (client_id INTEGER, inbound_id INTEGER, flow_override TEXT);
		INSERT INTO client_inbounds VALUES (7, 1, 'xtls-rprx-vision'), (7, 2, '');
		CREATE TABLE client_traffics (
			id INTEGER PRIMARY KEY, inbound_id INTEGER, email TEXT UNIQUE,
			up INTEGER, down INTEGER, total INTEGER, expiry_time INTEGER, enable NUMERIC
		);
		INSERT INTO client_traffics VALUES (1, 1, 'stock-client', 335, 445, 10737418240, 1900000000000, 1);
	`)
	if err != nil {
		_ = stock.Close()
		t.Fatal(err)
	}
	if err := stock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	upsertSetting(t, database.GetDB(), "webPort", "8443")
	src, err := os.Open(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	report, err := (&ServerService{}).ImportForeignDB(src, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Inbounds != 2 || report.Clients != 1 || getSetting(t, database.GetDB(), "webPort") != "8443" {
		t.Fatalf("unexpected import report or settings: %+v", report)
	}
	for id, wantFlow := range map[int]string{1: "xtls-rprx-vision", 2: ""} {
		svc := &InboundService{}
		inbound, err := svc.GetInbound(id)
		if err != nil {
			t.Fatal(err)
		}
		clients, err := svc.GetClients(inbound)
		if err != nil || len(clients) != 1 {
			t.Fatalf("inbound %d: projected clients = %v, err = %v", id, clients, err)
		}
		client := clients[0]
		if client.Email != "stock-client" || client.ID != "12345678-1234-4234-9234-123456789abc" ||
			client.Flow != wantFlow || client.SubID != "stock-sub" || client.TgID != 123456789 || client.TotalGB != 10737418240 {
			t.Fatalf("inbound %d: unexpected projected client %+v", id, client)
		}
	}
	stats, err := (&InboundService{}).GetClientTrafficByEmail("stock-client")
	if err != nil || stats == nil || stats.Up != 335 || stats.Down != 445 {
		t.Fatalf("traffic was not preserved: %+v, %v", stats, err)
	}
	accounts := &AccountService{}
	account, err := accounts.GetAccountByEmail("stock-client")
	if err != nil || account == nil {
		t.Fatalf("imported client missing from accounts: %+v, %v", account, err)
	}

	// A broken membership is rejected before the live DB is touched.
	stock, err = sql.Open("sqlite3", srcPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = stock.Exec("INSERT INTO client_inbounds VALUES (999, 1, '')")
	_ = stock.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&ServerService{}).ImportForeignDB(src, false); err == nil {
		t.Fatal("expected malformed backup to be rejected")
	}
	if getSetting(t, database.GetDB(), "webPort") != "8443" {
		t.Fatal("the original database was changed on rejection")
	}
}

func TestImportForeignDBRollsBackWhenSchemaMigrationFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPNUI_DB_FOLDER", dir)
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	upsertSetting(t, database.GetDB(), "webPort", "8443")
	path := filepath.Join(dir, "broken.db")
	broken, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = broken.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT, password TEXT);
		CREATE TABLE settings (id INTEGER PRIMARY KEY, key TEXT, value TEXT);
		CREATE TABLE inbounds (id INTEGER PRIMARY KEY);
		CREATE VIEW accounts AS SELECT id FROM users;
	`)
	_ = broken.Close()
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if _, err := (&ServerService{}).ImportForeignDB(src, false); err == nil {
		t.Fatal("expected the incompatible schema migration to fail")
	}
	if getSetting(t, database.GetDB(), "webPort") != "8443" {
		t.Fatal("the prior database was not restored and opened")
	}
}
