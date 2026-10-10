package service

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"testing"
	"time"
)

func TestPasarGuardQuotaUsesConsumptionNotAllocatedTraffic(t *testing.T) {
	p := model.ResellerProfile{ConsumptionBased: true, AllowanceBytes: 10 * gb, SpentBytes: 2 * gb}
	q, err := Quote(QuoteInput{Profile: p, Create: true, NewTotal: 100 * gb})
	if err != nil || q.DeltaSpent != 0 || q.NewCharged != 0 {
		t.Fatalf("allocated traffic was charged: %+v, %v", q, err)
	}
	p.SpentBytes = 10 * gb
	if _, err := Quote(QuoteInput{Profile: p, Create: true, NewTotal: gb}); err == nil {
		t.Fatal("exhausted representative was allowed to write")
	}
	p.Unlimited = true
	p.RoleLimits = `{"data_limit_max":1073741824}`
	if _, err := Quote(QuoteInput{Profile: p, Create: true, NewTotal: 2 * gb}); err == nil {
		t.Fatal("role quota maximum bypassed")
	}
}

func TestPasarGuardDeletedUsageRemainsCharged(t *testing.T) {
	newInboundDB(t)
	db := database.GetDB()
	u := model.User{Username: "usage-representative", Enable: true, IsReseller: true}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	profile := model.ResellerProfile{UserId: u.Id, ConsumptionBased: true, AllowanceBytes: 10 * gb, UsageBase: 2 * gb}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	ct := xray.ClientTraffic{Email: "customer", AllTime: 6 * gb, Down: 3 * gb}
	if err := db.Create(&ct).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ResellerClient{Email: ct.Email, UserId: u.Id, AllTimeBase: 3 * gb}).Error; err != nil {
		t.Fatal(err)
	}
	s := ResellerService{}
	before, err := s.ProfileFor(u.Id)
	if err != nil || before.SpentBytes != 5*gb {
		t.Fatalf("usage before deletion: %+v %v", before, err)
	}
	if err := db.Delete(&ct).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.RefundDeleted(ct.Email, ct.AllTime, true); err != nil {
		t.Fatal(err)
	}
	after, err := s.ProfileFor(u.Id)
	if err != nil || after.SpentBytes != 5*gb {
		t.Fatalf("deleting a customer refunded consumed bytes: %+v %v", after, err)
	}
}

func TestPasarGuardProjectionPreservesDifferentProtocolCredentials(t *testing.T) {
	a := model.Account{UUID: "shared", Security: "auto", ProtocolCredentials: `{"vmess":{"id":"vmess-original"},"vless":{"id":"vless-original"}}`}
	for protocol, want := range map[model.Protocol]string{model.VMESS: "vmess-original", model.VLESS: "vless-original"} {
		entry := map[string]any{}
		applyAccountCredential(entry, &a, &model.Inbound{Protocol: protocol})
		if entry["id"] != want {
			t.Fatalf("%s credential changed", protocol)
		}
	}
	overwriteAccountCredential(&a, map[string]any{"id": "rotated-vless"}, model.VLESS)
	var ps map[string]map[string]string
	if json.Unmarshal([]byte(a.ProtocolCredentials), &ps) != nil || ps["vless"]["id"] != "rotated-vless" || ps["vmess"]["id"] != "vmess-original" {
		t.Fatal("credential rotation damaged another protocol")
	}
}

func TestPasarGuardUsageResetDoesNotRenewExpiryOrLifetimeUsage(t *testing.T) {
	newInboundDB(t)
	db := database.GetDB()
	expiry := time.Now().Add(48 * time.Hour).UnixMilli()
	a := model.Account{Email: "monthly-customer", Enable: false, ExpiryTime: expiry, ImportedResetDays: 30, ImportedLastResetAt: time.Now().Add(-31 * 24 * time.Hour).UnixMilli()}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	ct := xray.ClientTraffic{Email: a.Email, Enable: false, ExpiryTime: expiry, Down: 5 * gb, AllTime: 12 * gb}
	if err := db.Create(&ct).Error; err != nil {
		t.Fatal(err)
	}
	changed, err := resetImportedUsage(db)
	if err != nil || !changed {
		t.Fatal("usage not reset", err)
	}
	if err := db.First(&ct, ct.Id).Error; err != nil {
		t.Fatal(err)
	}
	if ct.Down != 0 || ct.AllTime != 12*gb || ct.ExpiryTime != expiry || !ct.Enable {
		t.Fatal("reset renewed expiry or lost lifetime usage")
	}
}
