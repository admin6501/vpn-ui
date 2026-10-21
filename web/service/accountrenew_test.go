package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func renewFixture(t *testing.T, svc *AccountService, quota, expiry, period int64, enabled bool) *model.Account {
	t.Helper()
	account, err := svc.SaveAccountWithoutInbound(map[string]any{
		"email": "Renew-Customer", "enable": enabled, "totalGB": float64(quota),
		"expiryTime": float64(expiry), "renewalDuration": float64(period),
		"uuid": "3fa85f64-5717-4562-b3fc-2c963f66afa6", "password": "keep-password",
		"vpnUsername": "keep-login", "subId": "keep-subscription", "limitIp": float64(2),
		"speedLimitDown": float64(512), "userLimitOverride": float64(1), "comment": "keep-comment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", account.Email).
		Updates(map[string]any{"up": 4 * gb, "down": 6 * gb, "all_time": 15 * gb, "enable": false}).Error; err != nil {
		t.Fatal(err)
	}
	return account
}

func TestAccountRenewAllPackageKinds(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		quota, expiry, period int64
		enabled               bool
	}{
		{"quota-and-time", 10 * gb, testNow - dayMillis, 30 * dayMillis, false},
		{"quota-only", 10 * gb, 0, 0, false},
		{"time-only", 0, testNow - dayMillis, 30 * dayMillis, false},
		{"fully-unlimited", 0, 0, 0, false},
		{"not-yet-exhausted", 10 * gb, time.Now().UnixMilli() + 8*dayMillis, 30 * dayMillis, true},
		{"first-use", 10 * gb, -30 * dayMillis, 0, true},
		{"frozen", 10 * gb, -5 * dayMillis, 30 * dayMillis, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newAccountsDB(t)
			before := renewFixture(t, svc, tc.quota, tc.expiry, tc.period, tc.enabled)
			started := time.Now().UnixMilli()
			if err := svc.RenewAccount(&model.User{IsSuperAdmin: true}, "renew-customer", nil); err != nil {
				t.Fatal(err)
			}
			after, err := svc.GetAccountByEmail(before.Email)
			if err != nil {
				t.Fatal(err)
			}
			traffic := accountTrafficOf(t, before.Email)
			if !after.Enable || !traffic.Enable || traffic.Up != 0 || traffic.Down != 0 || traffic.AllTime != 15*gb || traffic.Total != tc.quota || after.TotalGB != tc.quota {
				t.Fatalf("wrong renewed state: account=%+v traffic=%+v", after, traffic)
			}
			if tc.expiry == 0 {
				if after.ExpiryTime != 0 || after.RenewalDuration != 0 {
					t.Fatal("unlimited duration was changed")
				}
			} else if after.ExpiryTime < started+30*dayMillis || after.ExpiryTime > time.Now().UnixMilli()+30*dayMillis || traffic.ExpiryTime != after.ExpiryTime {
				t.Fatalf("deadline did not restart the full 30-day period: %d", after.ExpiryTime)
			}
			if after.UUID != before.UUID || after.Password != before.Password || after.VpnUsername != before.VpnUsername || after.SubID != before.SubID || after.LimitIP != 2 || *after.SpeedLimitDown != 512 || *after.UserLimitOverride != 1 || after.Comment != before.Comment || after.CreatedAt != before.CreatedAt {
				t.Fatal("renewal changed identity, connection details or limits")
			}
		})
	}
}

func TestAccountRenewLegacyDurationIsAskedOnce(t *testing.T) {
	svc := newAccountsDB(t)
	before := renewFixture(t, svc, 10*gb, testNow-dayMillis, 0, false)
	user := &model.User{IsSuperAdmin: true}
	if err := svc.RenewAccount(user, before.Email, nil); !errors.Is(err, ErrRenewalDurationUnknown) {
		t.Fatalf("got %v", err)
	}
	if got := accountTrafficOf(t, before.Email); got.Up != 4*gb || got.Down != 6*gb {
		t.Fatal("rejected renewal cleared usage")
	}
	period := 30 * dayMillis
	if err := svc.RenewAccount(user, before.Email, &period); err != nil {
		t.Fatal(err)
	}
	if err := svc.RenewAccount(user, before.Email, nil); err != nil {
		t.Fatal("saved duration was lost:", err)
	}
	wrong := int64(60 * dayMillis)
	if err := svc.RenewAccount(user, before.Email, &wrong); err == nil {
		t.Fatal("stale or forged period replaced the current package")
	}
	got, _ := svc.GetAccountByEmail(before.Email)
	if got.RenewalDuration != period {
		t.Fatal("original duration not retained")
	}
}

func TestAccountRenewPreservesMembershipAccessAndNodeCursors(t *testing.T) {
	svc := newAccountsDB(t)
	account := renewFixture(t, svc, 10*gb, testNow-dayMillis, 30*dayMillis, false)
	var memberships []model.AccountInbound
	for i, protocol := range []model.Protocol{model.VLESS, model.Shadowsocks, model.AWG, model.GRE} {
		inbound := &model.Inbound{UserId: 1, Protocol: protocol, Tag: "renew-" + string(protocol), Port: 49000 + i, Enable: true, Settings: `{"clients":[]}`}
		if err := database.GetDB().Create(inbound).Error; err != nil {
			t.Fatal(err)
		}
		slot, enabled := 7+i, i != 1
		entry := map[string]any{"email": account.Email, "id": account.UUID, "password": account.Password, "slot": slot,
			"devices": []any{map[string]any{"privateKey": "keep-private", "publicKey": "keep-public"}},
			"peers":   []any{map[string]any{"peerIp": "198.51.100.9"}}}
		encoded, _ := json.Marshal(entry)
		membership := model.AccountInbound{AccountId: account.Id, InboundId: inbound.Id, Slot: &slot, Enable: &enabled, Extra: string(encoded), Up: gb, Down: 2 * gb, AllTime: 5 * gb}
		if err := database.GetDB().Create(&membership).Error; err != nil {
			t.Fatal(err)
		}
		memberships = append(memberships, membership)
	}
	cursor := model.NodeCounter{NodeID: 3, Kind: "account", AccountID: account.Id, InboundID: memberships[0].InboundId, Up: 50 * gb, Down: 80 * gb}
	if err := database.GetDB().Create(&cursor).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.RenewAccount(&model.User{IsSuperAdmin: true}, account.Email, nil); err != nil {
		t.Fatal(err)
	}
	var after []model.AccountInbound
	if err := database.GetDB().Where("account_id = ?", account.Id).Order("inbound_id").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	if len(after) != len(memberships) {
		t.Fatal("membership set changed")
	}
	for i, got := range after {
		want := memberships[i]
		if got.Up != 0 || got.Down != 0 || got.AllTime != want.AllTime || !reflect.DeepEqual(got.Slot, want.Slot) || !reflect.DeepEqual(got.Enable, want.Enable) || got.Extra != want.Extra {
			t.Fatal("membership access, device keys, peers or accounting changed:", got)
		}
		var inbound model.Inbound
		database.GetDB().First(&inbound, got.InboundId)
		var settings struct {
			Clients []map[string]any `json:"clients"`
		}
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			t.Fatal(err)
		}
		if len(settings.Clients) != 1 || settings.Clients[0]["enable"] != *want.Enable || settings.Clients[0]["subId"] != account.SubID || settings.Clients[0]["slot"] != float64(*want.Slot) {
			t.Fatal("wrong projected access:", settings.Clients)
		}
	}
	var gotCursor model.NodeCounter
	if err := database.GetDB().First(&gotCursor).Error; err != nil {
		t.Fatal(err)
	}
	if gotCursor != cursor {
		t.Fatal("cumulative node cursor was reset, risking double billing")
	}
}

func TestAccountRenewLedgerAndRollback(t *testing.T) {
	for _, failProjection := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[failProjection], func(t *testing.T) {
			svc := newAccountsDB(t)
			account := renewFixture(t, svc, 10*gb, 0, 0, false)
			profile := model.ResellerProfile{UserId: 42, AllowanceBytes: 30 * gb, SpentBytes: 10 * gb}
			owner := model.ResellerClient{Email: account.Email, UserId: 42, ChargedBytes: 10 * gb, AllTimeBase: 2 * gb}
			if err := database.GetDB().Create(&profile).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.GetDB().Create(&owner).Error; err != nil {
				t.Fatal(err)
			}
			if failProjection {
				inbound := &model.Inbound{UserId: 1, Protocol: model.VLESS, Tag: "broken", Port: 49090, Settings: "invalid JSON"}
				database.GetDB().Create(inbound)
				database.GetDB().Create(&model.AccountInbound{AccountId: account.Id, InboundId: inbound.Id})
			}
			err := svc.RenewAccount(&model.User{Id: 42, IsReseller: true}, account.Email, nil)
			if (err != nil) != failProjection {
				t.Fatalf("err=%v", err)
			}
			database.GetDB().First(&profile, profile.Id)
			database.GetDB().First(&owner, owner.Id)
			wantSpent := int64(20 * gb)
			if failProjection {
				wantSpent = 10 * gb
			}
			if profile.SpentBytes != wantSpent || owner.ChargedBytes != wantSpent || owner.AllTimeBase != 2*gb {
				t.Fatal("wrong or partially committed charge")
			}
			got := accountTrafficOf(t, account.Email)
			if failProjection && (got.Up != 4*gb || got.Down != 6*gb) {
				t.Fatal("failed renewal reset usage")
			}
			if !failProjection {
				// A second renewal of zero usage cannot bill the same bytes twice.
				if err := svc.RenewAccount(&model.User{Id: 42, IsReseller: true}, account.Email, nil); err != nil {
					t.Fatal(err)
				}
				database.GetDB().First(&profile, profile.Id)
				if profile.SpentBytes != wantSpent {
					t.Fatal("duplicate reset charge")
				}
			}
		})
	}
}

func TestAccountRenewPeriodSurvivesOrdinaryEditsAndFreeze(t *testing.T) {
	old := &model.Account{Enable: true, ExpiryTime: testNow + 7*dayMillis, RenewalDuration: 30 * dayMillis}
	next := &model.Account{Enable: false, ExpiryTime: -7 * dayMillis}
	if err := captureRenewalDuration(next, old, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if next.RenewalDuration != 30*dayMillis {
		t.Fatal("freeze replaced full period with remaining time")
	}
	old.RenewalDuration = 0
	if err := captureRenewalDuration(next, old, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if next.RenewalDuration != 0 {
		t.Fatal("legacy frozen remainder was invented as original duration")
	}
}

func TestAccountRenewRetainsLegacyFirstUsePeriodBeforeClockStarts(t *testing.T) {
	svc := newAccountsDB(t)
	account := renewFixture(t, svc, 10*gb, -30*dayMillis, 0, true)
	// Simulate an account from before renewal metadata existed.
	if err := database.GetDB().Model(account).Update("renewal_duration", 0).Error; err != nil {
		t.Fatal(err)
	}
	inbound := seedInboundWithClients(t, model.VLESS, 49200, []map[string]any{
		{"email": account.Email, "id": account.UUID, "enable": true, "expiryTime": float64(-30 * dayMillis)},
	})
	if err := database.GetDB().Create(&model.AccountInbound{AccountId: account.Id, InboundId: inbound.Id}).Error; err != nil {
		t.Fatal(err)
	}
	traffics := []*xray.ClientTraffic{{Email: account.Email, InboundId: inbound.Id, Enable: true, ExpiryTime: -30 * dayMillis}}
	if _, err := (&InboundService{}).adjustTraffics(database.GetDB(), traffics); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncInboundAccounts(database.GetDB(), inbound.Id); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.GetAccountByEmail(account.Email)
	if got.ExpiryTime <= 0 || got.RenewalDuration != 30*dayMillis {
		t.Fatal("first use lost the full legacy period:", got)
	}
}

func TestAccountRenewRepresentativeMeasuredBalanceAndExpiryLimits(t *testing.T) {
	for _, tc := range []struct {
		name, limits string
		allowance    int64
		blocked      bool
	}{
		{"enough-balance", `{}`, 100 * gb, false},
		{"measured-balance-exhausted", `{}`, 10 * gb, true},
		{"expiry-limit", `{"expire_max":864000}`, 100 * gb, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newAccountsDB(t)
			account := renewFixture(t, svc, 10*gb, testNow-dayMillis, 30*dayMillis, false)
			role := model.RepresentativeRole{Name: "renew-role", Limits: tc.limits}
			if err := database.GetDB().Create(&role).Error; err != nil {
				t.Fatal(err)
			}
			user := model.User{Username: "renew-representative", Enable: true, IsReseller: true, RepresentativeRoleID: role.ID}
			if err := database.GetDB().Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			profile := model.ResellerProfile{UserId: user.Id, ConsumptionBased: true, AllowanceBytes: tc.allowance}
			if err := database.GetDB().Create(&profile).Error; err != nil {
				t.Fatal(err)
			}
			owner := model.ResellerClient{Email: account.Email, UserId: user.Id, AllTimeBase: 2 * gb}
			if err := database.GetDB().Create(&owner).Error; err != nil {
				t.Fatal(err)
			}
			err := svc.RenewAccount(&user, account.Email, nil)
			if (err != nil) != tc.blocked {
				t.Fatalf("wrong representative outcome: %v", err)
			}
			got := accountTrafficOf(t, account.Email)
			if got.AllTime != 15*gb {
				t.Fatal("renewal erased measured representative consumption")
			}
			if tc.blocked && (got.Up != 4*gb || got.Down != 6*gb) {
				t.Fatal("rejected renewal cleared usage")
			}
		})
	}
}

func TestAccountRenewDerivedTimeCannotBypassPackagePrice(t *testing.T) {
	for _, tc := range []struct {
		name              string
		allowance, period int64
		blocked           bool
	}{
		{"full-package-price", 100 * gb, 30 * dayMillis, false},
		{"insufficient-full-package-balance", 15 * gb, 30 * dayMillis, true},
		{"excess-duration", 100 * gb, 31 * dayMillis, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newAccountsDB(t)
			account := renewFixture(t, svc, 10*gb, testNow-dayMillis, tc.period, false)
			database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", account.Email).Updates(map[string]any{"up": gb, "down": 0})
			profile := model.ResellerProfile{UserId: 42, AllowanceBytes: tc.allowance, SpentBytes: 10 * gb, DaysPerGB: 3}
			owner := model.ResellerClient{Email: account.Email, UserId: 42, ChargedBytes: 10 * gb}
			if err := database.GetDB().Create(&profile).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.GetDB().Create(&owner).Error; err != nil {
				t.Fatal(err)
			}
			err := svc.RenewAccount(&model.User{Id: 42, IsReseller: true}, account.Email, nil)
			if (err != nil) != tc.blocked {
				t.Fatalf("wrong derived-duration renewal: %v", err)
			}
			database.GetDB().First(&profile, profile.Id)
			want := int64(20 * gb)
			if tc.blocked {
				want = 10 * gb
			}
			if profile.SpentBytes != want {
				t.Fatal("full timed package was renewed without buying it")
			}
		})
	}
}
