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
