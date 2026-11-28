package service

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func BenchmarkClientAccountRateSampling(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			records := make([]*xray.ClientTraffic, size)
			for i := range records {
				records[i] = &xray.ClientTraffic{Email: "test-" + strconv.Itoa(i), Up: 1234, Down: 5678}
			}
			b.ReportAllocs()
			for b.Loop() {
				ObserveLocalAccountRates(records, time.Now(), 10*time.Second)
				CurrentAccountRates()
			}
			ObserveLocalAccountRates(nil, time.Now(), 10*time.Second)
		})
	}
}

func TestAccountAutoRenewAndHistory(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		total, expiry           int64
		optIn, enabled, renewed bool
	}{
		{"depleted quota", 10 * gb, 0, true, true, true},
		{"expired without quota", 0, time.Now().Add(-time.Hour).UnixMilli(), true, true, true},
		{"manual disable", 10 * gb, 0, true, false, false},
		{"default off", 10 * gb, 0, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newAccountsDB(t)
			a := renewFixture(t, svc, tc.total, tc.expiry, 7*dayMillis, tc.enabled)
			if tc.optIn {
				if err := svc.SetAutoRenew(a.Email, true); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := svc.RenewDueAccounts(); err != nil {
				t.Fatal(err)
			}
			got, err := svc.GetAccountByEmail(a.Email)
			if err != nil {
				t.Fatal(err)
			}
			if (got.LastRenewedAt > 0) != tc.renewed {
				t.Fatalf("unexpected renewal: %+v", got)
			}
			if got.CreatedAt != a.CreatedAt || got.SubID != a.SubID || got.TotalGB != tc.total {
				t.Fatal("renewal discarded package identity")
			}
			traffic := accountTrafficOf(t, a.Email)
			if tc.renewed && (traffic.Up != 0 || traffic.Down != 0 || !traffic.Enable || traffic.AllTime != 15*gb) {
				t.Fatalf("unexpected traffic state: %+v", traffic)
			}
			if !tc.renewed && traffic.Up+traffic.Down != 10*gb {
				t.Fatal("ineligible client's traffic changed")
			}
			if _, err := svc.RenewDueAccounts(); err != nil {
				t.Fatal(err)
			}
			again, err := svc.GetAccountByEmail(a.Email)
			if err != nil || again.LastRenewedAt != got.LastRenewedAt {
				t.Fatal("same depletion renewed twice")
			}

			if _, err := svc.AccountHistory(&model.User{Id: 2}, a.Email, 0); err == nil {
				t.Fatal("unassigned account's history was exposed")
			}
			history, err := svc.AccountHistory(&model.User{IsSuperAdmin: true}, a.Email, 0)
			if err != nil || len(history.Events) == 0 {
				t.Fatalf("missing history: %+v, %v", history, err)
			}
			if tc.renewed && history.Events[0].Kind != "autoRenewed" {
				t.Fatal(history.Events)
			}
			if history.Events[len(history.Events)-1].Kind != "created" {
				t.Fatal(history.Events)
			}
			for _, event := range history.Events {
				if strings.Contains(event.Changes, a.Password) {
					t.Fatal("credentials leaked in audit history")
				}
			}
		})
	}
}

func TestClientAccountOnlineSortAndRates(t *testing.T) {
	rows := []AccountRow{
		{Id: 1, Email: "z", OnlineSince: 100},
		{Id: 2, Email: "a", OnlineSince: 200},
		{Id: 3, Email: "b", LastOnline: 50},
		{Id: 4, Email: "x", LastOnline: 90},
	}
	live := map[string]bool{"z": true, "a": true}
	sortAccountRows(rows, nil, live, AccountSortOnline)
	if !equal(names(rows), []string{"z", "a", "x", "b"}) {
		t.Fatal(names(rows))
	}
	sortAccountRows(rows, nil, live, AccountSortOffline)
	if !equal(names(rows), []string{"b", "x", "a", "z"}) {
		t.Fatal(names(rows))
	}
	var tracker onlineStarts
	tracker.observe([]string{"Alpha"}, 100)
	tracker.observe([]string{"alpha", "Beta"}, 200)
	if got := tracker.snapshot(); got["alpha"] != 100 || got["beta"] != 200 {
		t.Fatal(got)
	}
	tracker.observe([]string{"beta"}, 300)
	tracker.observe([]string{"alpha", "beta"}, 400)
	if got := tracker.snapshot(); got["alpha"] != 400 {
		t.Fatal(got)
	}

	now := time.Now()
	ObserveLocalAccountRates([]*xray.ClientTraffic{{Email: "Rate-Test", Up: 1000, Down: 2000}, {Email: "rate-test", Up: 2000}}, now, 10*time.Second)
	defer ObserveLocalAccountRates(nil, time.Now(), 10*time.Second)
	got := CurrentAccountRates()["rate-test"]
	if got.Up != 300 || got.Down != 200 {
		t.Fatal(got)
	}
}

func TestAccountAutoRenewDoesNotBypassManualBilling(t *testing.T) {
	svc := newAccountsDB(t)
	a := renewFixture(t, svc, 10*gb, 0, 0, true)
	owner := &model.User{Username: "auto-renew-reseller", Password: "unused", Enable: true, IsReseller: true}
	db := database.GetDB()
	if err := db.Create(owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ResellerClient{Email: a.Email, UserId: owner.Id, ChargedBytes: 10 * gb}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.SetAutoRenew(a.Email, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RenewDueAccounts(); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetAccountByEmail(a.Email)
	if err != nil || got.LastRenewedAt != 0 || accountTrafficOf(t, a.Email).Up != 4*gb {
		t.Fatalf("unfunded renewal modified state: %+v, %v", got, err)
	}
}

func TestAutomaticRenewalLimitCountsQuotaAndTimeOnce(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	account := renewFixture(t, accounts, 10*gb, time.Now().Add(time.Hour).UnixMilli(), dayMillis, true)
	limit := 2
	if err := accounts.ConfigureAutoRenew(account.Email, true, &limit); err != nil {
		t.Fatal(err)
	}
	traffic := db.Model(&xray.ClientTraffic{}).Where("email=?", account.Email)
	for i, depleted := range []map[string]any{
		{"up": 10 * gb, "down": 0},
		{"expiry_time": 1},
		{"expiry_time": 1, "up": 10 * gb},
	} {
		if err := traffic.Updates(depleted).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := accounts.RenewDueAccounts(); err != nil {
			t.Fatal(err)
		}
		var current model.Account
		if err := db.First(&current, account.Id).Error; err != nil {
			t.Fatal(err)
		}
		if current.AutoRenewCount != min(i+1, 2) {
			t.Fatalf("renewal round %d used %d renewals", i, current.AutoRenewCount)
		}
		var stored xray.ClientTraffic
		if err := db.Where("email=?", account.Email).First(&stored).Error; err != nil {
			t.Fatal(err)
		}
		if renewalDepleted(&stored, time.Now().UnixMilli()) != (i == 2) {
			t.Fatalf("unexpected depletion after round %d: %+v", i, stored)
		}
	}
}

func TestLowQuotaNoticeUsesClientOwnerAndPositiveBalance(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	var owner model.User
	if err := db.Where("is_panel_owner").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	admin := model.User{Username: "low-quota-admin", Enable: true, Permissions: model.PermAccessInbounds}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		email       string
		owner       int
		total, used int64
	}{
		{"only-low", admin.Id, 2 * gb, 2*gb - 1234},
		{"full-gigabyte", admin.Id, 2 * gb, gb},
		{"used-up", admin.Id, gb, gb},
		{"unlimited", admin.Id, 0, 0},
		{"owners-low", owner.Id, gb, 1234},
	} {
		if _, err := accounts.SaveAccountWithoutInbound(map[string]any{"email": entry.email, "enable": true, "totalGB": float64(entry.total)}, entry.owner); err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&xray.ClientTraffic{}).Where("email=?", entry.email).Update("up", entry.used).Error; err != nil {
			t.Fatal(err)
		}
	}
	activity := ActivityService{}
	for user, want := range map[*model.User]string{&admin: "only-low", &owner: "owners-low"} {
		notice, err := activity.LowQuotaNotice(user)
		if err != nil || notice.Count != 1 || len(notice.Names) != 1 || notice.Names[0] != want {
			t.Fatalf("unexpected low quota notice for %s: %+v, %v", user.Username, notice, err)
		}
		rows, err := activity.LowQuotaClients(user, 0)
		if err != nil || len(rows) != 1 || rows[0].Email != want || rows[0].Remaining <= 0 {
			t.Fatalf("unexpected low quota rows for %s: %+v, %v", user.Username, rows, err)
		}
	}
	if err := activity.SetLowQuotaNotices(&admin, false); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&admin, admin.Id).Error; err != nil {
		t.Fatal(err)
	}
	if notice, err := activity.LowQuotaNotice(&admin); err != nil || notice.Count != 0 {
		t.Fatalf("disabled low quota notice: %+v, %v", notice, err)
	}
}
