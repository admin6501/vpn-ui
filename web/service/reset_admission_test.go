package service

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"os"
	"testing"
	"time"
)

func TestResetRestoresAllEligibleMemberships(t *testing.T) {
	for _, mode := range []string{"single", "email", "bulk", "expired", "manual", "blocked"} {
		t.Run(mode, func(t *testing.T) {
			accounts := newAccountsDB(t)
			email := "reset-all@example.com"
			a := seedInboundWithClients(t, model.VLESS, 47111, []map[string]any{{"email": email, "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6", "enable": true, "subId": "keep", "totalGB": float64(100)}})
			b := seedInboundWithClients(t, model.Trojan, 47112, []map[string]any{{"email": email, "password": "keep-password", "enable": true, "subId": "keep", "totalGB": float64(100)}})
			accounts.MigrationAccounts()
			account := noInboundAccount(t, accounts, email)
			if _, err := accounts.SetMembershipEnable(email, b.Id, false); err != nil {
				t.Fatal(err)
			}
			if mode == "expired" {
				database.GetDB().Model(account).Update("expiry_time", time.Now().Add(-time.Hour).UnixMilli())
			}
			if mode == "manual" {
				database.GetDB().Model(account).Update("enable", false)
			}
			if mode == "blocked" {
				database.GetDB().Model(account).Update("representative_blocked", true)
			}
			seedTraffic(t, a.Id, email, 100, 20, 100, 0, false)
			database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", email).Update("all_time", 999)
			// Legacy inbound projection may lag the account; reset must restore it.
			database.GetDB().Model(&model.Inbound{}).Where("id = ?", a.Id).Update("settings", `{"clients":[{"email":"reset-all@example.com","enable":false}]}`)
			svc := &InboundService{}
			var err error
			switch mode {
			case "single":
				_, err = svc.ResetClientTraffic(a.Id, email)
			case "bulk":
				err = svc.ResetAllClientTraffics(a.Id)
			default:
				err = svc.ResetClientTrafficByEmail(email)
			}
			if err != nil {
				t.Fatal(err)
			}
			var traffic xray.ClientTraffic
			database.GetDB().Where("email = ?", email).First(&traffic)
			eligible := mode != "expired" && mode != "manual" && mode != "blocked"
			if traffic.Enable != eligible || traffic.Up != 0 || traffic.Down != 0 || traffic.AllTime != 999 {
				t.Fatalf("incorrect reset: %+v", traffic)
			}
			if clientEntry(t, b.Id, email)["enable"] != false {
				t.Fatal("reset enabled manually disabled membership")
			}
			entry := clientEntry(t, a.Id, email)
			if entry["id"] != account.UUID || entry["subId"] != "keep" {
				t.Fatal("reset lost credentials or token")
			}
			if eligible && entry["enable"] != true {
				t.Fatal("eligible membership not restored")
			}
		})
	}
}

func TestSpeedLimitNativeWireguardAddressIdentity(t *testing.T) {
	path := newLimiterSidecar(t)
	a := seedInboundWithClients(t, model.WireGuard, 47113, []map[string]any{{"email": "wg-speed-a", "enable": true, "addresses": []string{"10.99.0.2/24"}, "speedLimitDown": 128, "speedLimitUp": 64}})
	a.Tag += "-a"
	if err := database.GetDB().Model(a).Update("tag", a.Tag).Error; err != nil {
		t.Fatal(err)
	}
	b := seedInboundWithClients(t, model.WireGuard, 47114, []map[string]any{{"email": "wg-speed-b", "enable": true, "addresses": []string{"10.99.0.2/24"}, "speedLimitDown": 256, "speedLimitUp": 32}})
	WriteSpeedLimits()
	if len(BuildVpnEmailToIPMap()["wg-speed-a"]) != 0 {
		t.Fatal("native WireGuard leaked into ambiguous global tunnel routing")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc speedLimitDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Users) != 2 {
		t.Fatalf("missing WireGuard limits: %+v", doc)
	}
	for _, pair := range []struct {
		email    string
		tag      string
		down, up int64
	}{{"wg-speed-a", a.Tag, 128 * 1024, 64 * 1024}, {"wg-speed-b", b.Tag, 256 * 1024, 32 * 1024}} {
		var found bool
		for _, user := range doc.Users {
			if user.Email != pair.email {
				continue
			}
			found = true
			if user.DownBps != pair.down || user.UpBps != pair.up || len(user.IPs) != 0 || len(user.Bindings) != 1 || user.Bindings[0].Tag != pair.tag || user.Bindings[0].IP != "10.99.0.2" {
				t.Fatalf("wrong scoped peer limit: %+v", user)
			}
		}
		if !found {
			t.Fatal("missing peer", pair.email)
		}
	}
}

func TestNodeSpeedThresholdUsesCentralPeriodAfterReset(t *testing.T) {
	svc, node, _ := nodeFixture(t)
	in, account := seedNodeAccount(t, node, 47121, "node-reset-speed")
	database.GetDB().Model(in).Updates(map[string]any{"speed_limit_enable": true, "speed_limit_down": 128, "speed_limit_after": 500})
	database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", account.Email).Updates(map[string]any{"up": 800, "enable": false})
	before, err := svc.Snapshot(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Accounts[0].Usage == nil || *before.Accounts[0].Usage != 800 {
		t.Fatal("snapshot omitted central period usage")
	}
	if err := (&InboundService{}).ResetClientTrafficByEmail(account.Email); err != nil {
		t.Fatal(err)
	}
	after, err := svc.Snapshot(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Accounts[0].Usage == nil || *after.Accounts[0].Usage != 0 || !after.Accounts[0].Allowed {
		t.Fatal("reset not propagated to node")
	}
	// Separate worker database: its cumulative report meter must survive reset.
	newAccountsDB(t)
	t.Setenv("VPNUI_BIN_FOLDER", t.TempDir())
	if _, err := ApplyNodeSnapshot(before, map[string]bool{"xray": true}); err != nil {
		t.Fatal(err)
	}
	database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", account.Email).Updates(map[string]any{"up": 900, "all_time": 900})
	if _, err := ApplyNodeSnapshot(after, map[string]bool{"xray": true}); err != nil {
		t.Fatal(err)
	}
	var traffic xray.ClientTraffic
	database.GetDB().Where("email = ?", account.Email).First(&traffic)
	if traffic.Up != 900 || traffic.AllTime != 900 || !traffic.Enable {
		t.Fatalf("reset corrupted worker meter/admission: %+v", traffic)
	}
	if usage := loadSpeedLimitUsage()[account.Email]; usage != 0 {
		t.Fatalf("threshold reads cumulative worker usage: %d", usage)
	}
	users := computeSpeedLimits(loadSpeedLimitPolicies(), loadSpeedLimitUsage(), BuildVpnEmailToIPMap())
	if len(users) != 1 || users[0].DownBps != 0 || users[0].UpBps != 0 {
		t.Fatalf("reset kept speed threshold armed: %+v", users)
	}
}

func TestSpeedLimitBucketActivationAndThresholdContinuity(t *testing.T) {
	old := []byte(`{"users":[]}`)
	prepared := []byte(`{"users":[{"email":"live","downBps":0,"upBps":0,"ips":[]}]}`)
	capped := []byte(`{"users":[{"email":"live","downBps":65536,"upBps":32768,"ips":[]}]}`)
	if !speedBucketIntroduced(old, prepared) {
		t.Fatal("first activation leaves unlimited open dispatch unwrapped")
	}
	if speedBucketIntroduced(prepared, capped) || speedBucketIntroduced(capped, prepared) {
		t.Fatal("threshold/reset must not restart existing bucket")
	}
}
