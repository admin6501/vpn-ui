package controller

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func blockedCustomerFixture(t *testing.T) (*bulkMembershipEnv, *model.Account, *model.ResellerProfile, []int) {
	t.Helper()
	env := newBulkMembershipEnv(t)
	db := database.GetDB()
	const email = "blocked-customer"
	inbound := seedMembershipInbound(t, env.admin.Id, model.VLESS, 48781, `{"id":"11111111-2222-3333-4444-555555555555","email":"blocked-customer","enable":true,"subId":"keep-subscription","totalGB":10737418240}`)
	syncMembershipAccounts(t)
	account, err := accountService.GetAccountByEmail(email)
	if err != nil || account == nil {
		t.Fatal("missing account", err)
	}
	role := model.RepresentativeRole{Name: "Consumption role", DisconnectUsersWhenLimited: true}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	rep := model.User{Username: "consumption-representative", Enable: true, IsReseller: true, RepresentativeRoleID: role.ID}
	if err := db.Create(&rep).Error; err != nil {
		t.Fatal(err)
	}
	profile := model.ResellerProfile{UserId: rep.Id, ConsumptionBased: true, AllowanceBytes: 3 << 30}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ResellerClient{UserId: rep.Id, Email: email, InboundId: inbound.Id}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&xray.ClientTraffic{Email: email, InboundId: inbound.Id, Enable: true, Up: 1 << 30, Down: 2 << 30, AllTime: 3 << 30, Total: 10 << 30}).Error; err != nil {
		t.Fatal(err)
	}
	if err, _, _, _, _ := (&service.InboundService{}).AddTraffic(nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.First(account, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !account.Enable || !account.RepresentativeBlocked {
		t.Fatal("fixture must independently block an enabled customer")
	}
	return env, account, &profile, []int{inbound.Id}
}

func assertBlockedCustomerState(t *testing.T, account *model.Account, enabled, blocked bool) {
	t.Helper()
	if err := database.GetDB().First(account, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	if account.Enable != enabled || account.RepresentativeBlocked != blocked {
		t.Fatalf("account enable/block = %v/%v; want %v/%v", account.Enable, account.RepresentativeBlocked, enabled, blocked)
	}
	ct := accountTraffic(t, account.Email)
	if ct.Enable != (enabled && !blocked) || ct.Up != 1<<30 || ct.Down != 2<<30 || ct.AllTime != 3<<30 || ct.Total != 10<<30 {
		t.Fatal("effective traffic state or usage changed incorrectly")
	}
	var inbounds []model.Inbound
	if err := database.GetDB().Find(&inbounds).Error; err != nil {
		t.Fatal(err)
	}
	for _, inbound := range inbounds {
		var settings struct {
			Clients []model.Client `json:"clients"`
		}
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			t.Fatal(err)
		}
		for _, client := range settings.Clients {
			if client.Email == account.Email && client.Enable != (enabled && !blocked) {
				t.Fatal("runtime membership does not match effective state")
			}
		}
	}
}

func TestSecurityBlockedCustomerManualDisableSurvivesReplenishment(t *testing.T) {
	env, account, profile, ids := blockedCustomerFixture(t)
	msg := bulkMembershipPost(t, env.admin, "/panel/api/inbounds/saveAccount", clientBody(t, 0, map[string]any{"email": account.Email, "enable": false, "totalGB": float64(account.TotalGB), "subId": account.SubID}, ids, false))
	mustSucceed(t, msg, "disabling a representative-blocked client from the Clients page")
	assertBlockedCustomerState(t, account, false, true)
	if !account.ImportedResetDisabled {
		t.Fatal("manual disable must suppress imported automatic resets")
	}
	rows, err := accountService.ListAccounts(env.admin, 1, 50, account.Email, "")
	if err != nil || len(rows.Rows) != 1 || rows.Rows[0].Enable {
		t.Fatal("Clients page refresh would turn the switch back on", err)
	}
	if err := database.GetDB().Model(profile).Update("allowance_bytes", 100<<30).Error; err != nil {
		t.Fatal(err)
	}
	if err, _, _, _, _ := (&service.InboundService{}).AddTraffic(nil, nil); err != nil {
		t.Fatal(err)
	}
	assertBlockedCustomerState(t, account, false, false)
	if err := database.GetDB().Model(account).Updates(map[string]any{"imported_reset_days": 1, "imported_last_reset_at": time.Now().Add(-48 * time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if err, _, _, _, _ := (&service.InboundService{}).AddTraffic(nil, nil); err != nil {
		t.Fatal(err)
	}
	assertBlockedCustomerState(t, account, false, false)
}

func TestSecurityBlockedCustomerManualEnableKeepsEnforcement(t *testing.T) {
	env, account, _, ids := blockedCustomerFixture(t)
	for _, enabled := range []bool{false, true, false} {
		msg := bulkMembershipPost(t, env.admin, "/panel/api/inbounds/saveAccount", clientBody(t, 0, map[string]any{"email": account.Email, "enable": enabled, "totalGB": float64(account.TotalGB), "subId": account.SubID}, ids, false))
		mustSucceed(t, msg, "saving account enable intent")
		assertBlockedCustomerState(t, account, enabled, true)
	}
}

func TestSecurityBlockedCustomerRepairsStaleEnabledTraffic(t *testing.T) {
	_, account, _, _ := blockedCustomerFixture(t)
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", account.Email).Update("enable", true).Error; err != nil {
		t.Fatal(err)
	}
	err, changed, _, _, _ := (&service.InboundService{}).AddTraffic(nil, nil)
	if err != nil || !changed {
		t.Fatal("unchanged representative block must repair stale effective traffic enable", err)
	}
	assertBlockedCustomerState(t, account, true, true)
	if err, changed, _, _, _ := (&service.InboundService{}).AddTraffic(nil, nil); err != nil || changed {
		t.Fatal("repair must become a no-op on the next tick", err)
	}
}

func TestSecurityBlockedCustomerPartialSavePreservesEnableIntent(t *testing.T) {
	_, account, _, _ := blockedCustomerFixture(t)
	for _, enabled := range []bool{true, false} {
		if _, err := accountService.SaveAccountWithoutInbound(map[string]any{"email": account.Email, "enable": enabled, "totalGB": float64(account.TotalGB)}); err != nil {
			t.Fatal(err)
		}
		if _, err := accountService.SaveAccountWithoutInbound(map[string]any{"email": account.Email, "totalGB": float64(account.TotalGB)}); err != nil {
			t.Fatal(err)
		}
		assertBlockedCustomerState(t, account, enabled, true)
	}
}
