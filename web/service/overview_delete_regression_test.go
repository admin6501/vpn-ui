package service

import (
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"testing"
	"time"
)

func TestOverviewDeleteHomeInboundPreservesAccountUsage(t *testing.T) {
	accounts := newAccountsDB(t)
	email := "delete-home@example.com"
	first, second := seedTwoInboundAccount(t, accounts, email)
	db := database.GetDB()
	expiry := time.Now().Add(-time.Hour).UnixMilli()
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", email).Updates(map[string]any{"inbound_id": first.Id, "down": 12345, "all_time": 23456, "expiry_time": expiry}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Inbound{}).Where("id = ?", first.Id).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	svc := &InboundService{}
	if _, err := svc.DelInbound(first.Id); err != nil {
		t.Fatal(err)
	}
	var traffic xray.ClientTraffic
	if err := db.Where("email = ?", email).First(&traffic).Error; err != nil {
		t.Fatal(err)
	}
	if traffic.Down != 12345 || traffic.AllTime != 23456 || traffic.InboundId != second.Id {
		t.Fatalf("shared usage lost or not rehomed: %+v", traffic)
	}
	stats, err := svc.GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil || stats.Total != 1 || stats.Depleted != 1 {
		t.Fatalf("expired account disappeared after deleting home inbound: %+v %v", stats, err)
	}
}

func TestOverviewExpiredAccountWithMissingTrafficRow(t *testing.T) {
	accounts := newAccountsDB(t)
	email := "already-deleted-home@example.com"
	seedTwoInboundAccount(t, accounts, email)
	db := database.GetDB()
	if err := db.Model(&model.Account{}).Where("email = ?", email).Update("expiry_time", time.Now().Add(-time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("email = ?", email).Delete(&xray.ClientTraffic{}).Error; err != nil {
		t.Fatal(err)
	}
	stats, err := (&InboundService{}).GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil || stats.Depleted != 1 {
		t.Fatalf("account expiry hidden without historical usage: %+v %v", stats, err)
	}
}

func TestOverviewIdleTickClearsOnlineAccount(t *testing.T) {
	accounts := newAccountsDB(t)
	email := "idle-overview@example.com"
	first, _ := seedTwoInboundAccount(t, accounts, email)
	svc := &InboundService{}
	err, _, _, _, _ := svc.AddTraffic(nil, []*xray.ClientTraffic{{InboundId: first.Id, Email: email, Down: mb}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := svc.GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil || before.Online != 1 {
		t.Fatalf("setup not online: %+v %v", before, err)
	}
	svc.ClearOnlineStatus()
	after, err := svc.GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil || after.Online != 0 || len(svc.GetOnlineMemberships()) != 0 {
		t.Fatalf("idle snapshot retained: %+v %v", after, err)
	}
}

func TestOverviewExpirationIncludesExhaustedQuota(t *testing.T) {
	accounts := newAccountsDB(t)
	const email = "quota-expired@example.com"
	first, second := seedTwoInboundAccount(t, accounts, email)
	db := database.GetDB()
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", email).Updates(map[string]any{"total": 10, "down": 10, "expiry_time": time.Now().Add(time.Hour).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	svc := &InboundService{}
	stats, err := svc.GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil || stats.Depleted != 1 || stats.Expired != 1 || len(stats.ExpiredEmails) != 1 || stats.ExpiredEmails[0] != email {
		t.Fatalf("quota-exhausted account missing from expired summary: %+v %v", stats, err)
	}
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", email).Update("expiry_time", time.Now().Add(-time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Inbound{}).Where("id IN ?", []int{first.Id, second.Id}).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	stats, err = svc.GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil || stats.Expired != 1 {
		t.Fatalf("disabled inbounds hid account expiry: %+v %v", stats, err)
	}
}

func TestOverviewExpirationUsesEffectiveTrafficLimits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		total   int64
		up      int64
		down    int64
		expiry  int64
		enabled bool
		want    int
	}{
		{name: "quota_without_deadline", total: 10, up: 4, down: 6, enabled: false, want: 1},
		{name: "quota_with_future_deadline", total: 10, down: 10, expiry: time.Now().Add(time.Hour).UnixMilli(), enabled: false, want: 1},
		{name: "quota_exceeded_before_disable", total: 10, down: 11, enabled: true, want: 1},
		{name: "manually_disabled_with_remaining_quota", total: 10, down: 9, expiry: time.Now().Add(time.Hour).UnixMilli()},
		{name: "unlimited_quota", down: 100, enabled: true},
		{name: "delayed_start", total: 10, expiry: -86400000, enabled: true},
		{name: "frozen_with_remaining_quota", total: 10, down: 1, expiry: -86400000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts := newAccountsDB(t)
			const email = "effective-limits@example.com"
			seedTwoInboundAccount(t, accounts, email)
			if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", email).Updates(map[string]any{
				"total": tc.total, "up": tc.up, "down": tc.down,
				"expiry_time": tc.expiry, "enable": tc.enabled,
			}).Error; err != nil {
				t.Fatal(err)
			}
			stats, err := (&InboundService{}).GetClientStatsFor(&model.User{IsSuperAdmin: true})
			if err != nil {
				t.Fatal(err)
			}
			if stats.Total != 1 || stats.Expired != tc.want || stats.Depleted != tc.want || len(stats.ExpiredEmails) != tc.want || stats.Online != 0 {
				t.Fatalf("expiration summary = %+v, want %d expired account", stats, tc.want)
			}
			if tc.want == 1 && stats.ExpiredEmails[0] != email {
				t.Fatalf("expired email = %q, want %q", stats.ExpiredEmails[0], email)
			}
		})
	}
}

func TestOverviewOnlineEvidenceSurvivesMissingSharedTrafficRow(t *testing.T) {
	accounts := newAccountsDB(t)
	const email = "live-without-traffic-row@example.com"
	first, _ := seedTwoInboundAccount(t, accounts, email)
	db := database.GetDB()
	if err := db.Where("email = ?", email).Delete(&xray.ClientTraffic{}).Error; err != nil {
		t.Fatal(err)
	}
	svc := &InboundService{}
	err, _, _, _, _ := svc.AddTraffic(nil, []*xray.ClientTraffic{{InboundId: first.Id, Email: email, Down: mb}})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := svc.GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil || stats.Online != 1 {
		t.Fatalf("actual traffic shown as offline because billing row missing: %+v %v", stats, err)
	}
	var n int64
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", email).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("must not invent historical usage", err)
	}
	svc.ClearOnlineStatus()
	stats, err = svc.GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil || stats.Online != 0 {
		t.Fatalf("idle state retained: %+v %v", stats, err)
	}
}

func TestOverviewRetainedAccountsMatchClientsWithoutInbounds(t *testing.T) {
	accounts := newAccountsDB(t)
	const email = "retained-expired@example.com"
	first, second := seedTwoInboundAccount(t, accounts, email)
	db := database.GetDB()
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", email).Update("expiry_time", time.Now().Add(-time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.ApplyMemberships(email, nil, nil, true); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&model.Inbound{}, []int{first.Id, second.Id}).Error; err != nil {
		t.Fatal(err)
	}
	root := &model.User{IsSuperAdmin: true}
	list, err := accounts.ListAccounts(root, 1, 100, "", "")
	if err != nil {
		t.Fatal(err)
	}
	stats, err := (&InboundService{}).GetClientStatsFor(root)
	if err != nil || stats.Total != list.Total || stats.Total != 1 || stats.Expired != 1 || stats.Online != 0 {
		t.Fatalf("retained accounts disagree with Clients: %+v, list=%+v, error=%v", stats, list, err)
	}
}

func TestOverviewCountsSharedExpiryAndQuotaOnce(t *testing.T) {
	accounts := newAccountsDB(t)
	const email = "shared-expired@example.com"
	first, second := seedTwoInboundAccount(t, accounts, email)
	db := database.GetDB()
	svc := &InboundService{}
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", email).Updates(map[string]any{"expiry_time": time.Now().Add(-time.Hour).UnixMilli(), "total": 10, "down": 10}).Error; err != nil {
		t.Fatal(err)
	}
	stats, err := svc.GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil || stats.Total != 1 || stats.Expired != 1 || stats.Depleted != 1 {
		t.Fatalf("shared identity counted twice: %+v %v", stats, err)
	}
	if err := db.Model(&model.Inbound{}).Where("id IN ?", []int{first.Id, second.Id}).Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	stats, err = svc.GetClientStatsFor(&model.User{IsSuperAdmin: true})
	if err != nil || stats.Total != 1 || stats.Expired != 1 {
		t.Fatalf("disabled memberships changed account counters: %+v %v", stats, err)
	}
}

func TestOverviewRetainedAccountsStayScopedToOwner(t *testing.T) {
	accounts := newAccountsDB(t)
	const email = "owned-retained@example.com"
	seedTwoInboundAccount(t, accounts, email)
	db := database.GetDB()
	if err := db.Create(&model.ResellerClient{Email: email, UserId: 42}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.ApplyMemberships(email, nil, nil, true); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", email).Update("expiry_time", time.Now().Add(-time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	svc := &InboundService{}
	for _, test := range []struct {
		user model.User
		want int
	}{
		{model.User{Id: 42, IsReseller: true}, 1},
		{model.User{Id: 43, IsReseller: true}, 0},
		{model.User{Id: 43}, 0},
	} {
		stats, err := svc.GetClientStatsFor(&test.user)
		if err != nil || stats.Total != test.want || stats.Expired != test.want || len(stats.ExpiredEmails) != test.want {
			t.Fatalf("owner scope for %+v: %+v %v", test.user, stats, err)
		}
	}
}
