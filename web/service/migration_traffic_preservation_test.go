package service

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestMigrationPreservesCentralTrafficWithoutHomeInbound(t *testing.T) {
	for _, protocol := range []model.Protocol{model.VLESS, model.Shadowsocks} {
		for _, home := range []int{0, 9999} {
			t.Run(fmt.Sprintf("%s/home=%d", protocol, home), func(t *testing.T) {
				svc := newInboundDB(t)
				in := seedTaggedInbound(t, protocol, 48301, []map[string]any{testClient("central@example.com")})
				a := model.Account{Email: "central@example.com", Enable: true, TotalGB: 30 * gb}
				db := database.GetDB()
				if err := db.Create(&a).Error; err != nil {
					t.Fatal(err)
				}
				membership := model.AccountInbound{AccountId: a.Id, InboundId: in.Id, Up: gb, Down: 2 * gb, AllTime: 5 * gb}
				if err := db.Create(&membership).Error; err != nil {
					t.Fatal(err)
				}
				seedTrafficRow(t, xray.ClientTraffic{Email: a.Email, InboundId: home, Enable: false, Up: 2 * gb, Down: 3 * gb, AllTime: 12 * gb, Total: 30 * gb, ExpiryTime: 1900000000000, LastOnline: 1700000000000})
				before := *trafficRow(t, a.Email)
				for pass := 0; pass < 2; pass++ {
					svc.MigrationRequirements()
					svc.MigrationRemoveOrphanedTraffics()
					after := trafficRow(t, a.Email)
					if after == nil || !reflect.DeepEqual(before, *after) {
						t.Fatalf("pass %d deleted or changed central usage, quota, expiry or admission: before=%+v after=%+v", pass, before, after)
					}
					var got model.AccountInbound
					if err := db.Where("account_id = ? AND inbound_id = ?", a.Id, in.Id).First(&got).Error; err != nil || !reflect.DeepEqual(membership, got) {
						t.Fatalf("migration changed membership usage: %+v, %v", got, err)
					}
				}
			})
		}
	}
}

func TestXrayOutboundEditPreservesCentralAndMembershipTraffic(t *testing.T) {
	newInboundDB(t)
	db := database.GetDB()
	in := seedTaggedInbound(t, model.VLESS, 48302, []map[string]any{testClient("outbound-edit@example.com")})
	a := model.Account{Email: "outbound-edit@example.com", Enable: true, TotalGB: 30 * gb}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	membership := model.AccountInbound{AccountId: a.Id, InboundId: in.Id, Up: gb, Down: 2 * gb, AllTime: 7 * gb}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	seedTrafficRow(t, xray.ClientTraffic{Email: a.Email, InboundId: 0, Enable: true, Up: 2 * gb, Down: 3 * gb, AllTime: 12 * gb, Total: 30 * gb})
	before := *trafficRow(t, a.Email)
	settings := &XraySettingService{}
	initial, err := settings.GetXrayConfigTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var template map[string]any
	if err := json.Unmarshal([]byte(initial), &template); err != nil {
		t.Fatal(err)
	}
	retained, _ := template["outbounds"].([]any)
	template["outbounds"] = append(retained, map[string]any{"tag": "remove-me", "protocol": "freedom"})
	withOutbound, _ := json.Marshal(template)
	if err := settings.SaveXraySetting(string(withOutbound)); err != nil {
		t.Fatal(err)
	}
	template["outbounds"] = retained
	withoutOutbound, _ := json.Marshal(template)
	if err := settings.SaveXraySetting(string(withoutOutbound)); err != nil {
		t.Fatal(err)
	}
	// The restart builds this candidate before replacing the core process.
	if _, err := (&XrayService{}).GetXrayConfig(); err != nil {
		t.Fatal(err)
	}
	after := trafficRow(t, a.Email)
	if after == nil || !reflect.DeepEqual(before, *after) {
		t.Fatalf("removing an outbound changed central traffic: %+v", after)
	}
	var got model.AccountInbound
	if err := db.Where("account_id = ? AND inbound_id = ?", a.Id, in.Id).First(&got).Error; err != nil || !reflect.DeepEqual(membership, got) {
		t.Fatalf("removing an outbound changed membership traffic: %+v, %v", got, err)
	}
}

func TestMigrationCleanupPreservesParkedCanonicalTraffic(t *testing.T) {
	svc := newInboundDB(t)
	db := database.GetDB()
	a := model.Account{Email: " Parked@Example.com ", Enable: true, TotalGB: 30 * gb}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	seedTrafficRow(t, xray.ClientTraffic{Email: "parked@example.com", InboundId: 0, Up: gb, Down: 2 * gb, AllTime: 9 * gb, Total: 30 * gb, Enable: true})
	seedTrafficRow(t, xray.ClientTraffic{Email: "unclaimed@example.com", InboundId: 0, Up: gb})
	before := *trafficRow(t, "parked@example.com")
	for pass := 0; pass < 2; pass++ {
		svc.MigrationRequirements()
		svc.MigrationRemoveOrphanedTraffics()
		got := trafficRow(t, "parked@example.com")
		if got == nil || !reflect.DeepEqual(before, *got) {
			t.Fatalf("parked account lost its accounting: %+v", got)
		}
		if trafficRow(t, "unclaimed@example.com") != nil {
			t.Fatal("legacy unclaimed row survived cleanup")
		}
	}
}
