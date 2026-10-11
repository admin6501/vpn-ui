package service

import (
	"encoding/json"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"testing"
)

func TestIndependentAccountEnableWithDisabledVlessAndWireguard(t *testing.T) {
	svc := newAccountsDB(t)
	const email = "independent@example.com"
	vless := seedInboundWithClients(t, model.VLESS, 47361, []map[string]any{
		{"id": "3fa85f64-5717-4562-b3fc-2c963f66afa6", "email": email, "enable": true, "subId": "keep-token", "totalGB": float64(10 << 30)},
	})
	wg := seedInboundWithClients(t, model.WireGuard, 47362, []map[string]any{
		{"id": email, "email": email, "enable": true, "subId": "keep-token", "totalGB": float64(10 << 30), "privKey": "CBi/NbmtWR2Uk2jA0KAc4ZM+Q3WkKdBAm0jNs2K5XW4=", "pubKey": "dVBgJcQqfo6rb1K+OKSqvr0IuOX21iRz/lFsfcZAdEg="},
	})
	svc.MigrationAccounts()
	seedTraffic(t, vless.Id, email, 123, 456, 10<<30, 0, true)
	if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", vless.Id).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetMembershipEnable(email, vless.Id, false); err != nil {
		t.Fatal(err)
	}
	account := noInboundAccount(t, svc, email)
	if !account.Enable || account.ImportedResetDisabled {
		t.Fatalf("disabled VLESS contaminated shared account: %+v", account)
	}
	if clientEntry(t, wg.Id, email)["enable"] != true {
		t.Fatal("WireGuard disabled alongside VLESS")
	}
	before := *account
	ReconcileAllWireguardXrayKeys()
	config, err := (&XrayService{}).GetXrayConfig()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, inbound := range config.InboundConfigs {
		if inbound.Tag == vless.Tag {
			t.Fatal("disabled VLESS remains in runtime config")
		}
		if inbound.Tag == wg.Tag {
			var settings map[string]any
			if err := json.Unmarshal(inbound.Settings, &settings); err != nil {
				t.Fatal(err)
			}
			peers, _ := settings["peers"].([]any)
			if len(peers) != 1 {
				t.Fatalf("WireGuard peer disappeared: %s", inbound.Settings)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("WireGuard missing from runtime config")
	}
	inboundService := &InboundService{}
	enabled, err := inboundService.checkIsEnabledByEmail(email)
	if err != nil || !enabled {
		t.Fatalf("account read from disabled first membership: %v %v", enabled, err)
	}
	for _, want := range []bool{false, true} {
		got, restart, err := inboundService.ToggleClientEnableByEmail(email)
		if err != nil || got != want || !restart {
			t.Fatalf("toggle: %v %v %v", got, restart, err)
		}
		if clientEntry(t, vless.Id, email)["enable"] != false {
			t.Fatal("global toggle re-enabled disabled VLESS membership")
		}
		if clientEntry(t, wg.Id, email)["enable"] != want {
			t.Fatal("WireGuard did not follow explicit global toggle")
		}
	}
	account = noInboundAccount(t, svc, email)
	if account.UUID != before.UUID || account.SubID != before.SubID || account.TotalGB != before.TotalGB {
		t.Fatal("credentials, subscription or quota changed")
	}
	var traffic xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", email).First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if !traffic.Enable || traffic.Up != 123 || traffic.Down != 456 || traffic.Total != 10<<30 {
		t.Fatalf("shared quota/usage changed: %+v", traffic)
	}
}

func TestIndependentAccountEnableWithoutInbound(t *testing.T) {
	svc := newAccountsDB(t)
	if _, err := svc.SaveAccountWithoutInbound(map[string]any{"email": "standalone", "enable": true, "totalGB": float64(10 << 30), "subId": "preserve"}); err != nil {
		t.Fatal(err)
	}
	s := &InboundService{}
	for _, want := range []bool{false, true} {
		got, _, err := s.ToggleClientEnableByEmail("standalone")
		if err != nil || got != want {
			t.Fatalf("standalone toggle: %v %v", got, err)
		}
	}
}

func TestIndependentAccountRejectsInvalidLimitsBeforeWriting(t *testing.T) {
	svc := newAccountsDB(t)
	for _, limit := range []any{-1, "invalid"} {
		if _, err := svc.SaveAccountWithoutInbound(map[string]any{"email": "bad-limit", "enable": true, "speedLimitDown": limit}); err == nil {
			t.Fatal("invalid speed limit accepted")
		}
		if account, err := svc.GetAccountByEmail("bad-limit"); err != nil || account != nil {
			t.Fatal("invalid speed created a partial account")
		}
	}
}
