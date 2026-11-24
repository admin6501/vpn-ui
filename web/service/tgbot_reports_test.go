package service

import (
	"slices"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

func TestTelegramReportsFollowAccountStateAndOwnership(t *testing.T) {
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	db := database.GetDB()
	now := time.Now().UnixMilli()
	for _, record := range []any{
		&model.Account{Email: "Parked", Enable: true, TgID: 981, ExpiryTime: now + 1000},
		&model.Account{Email: "First", Enable: true, TgID: 981, SubID: "shared", TotalGB: 300},
		&model.Account{Email: "Other", Enable: true, TgID: 982, SubID: "shared"},
		&xray.ClientTraffic{Email: "first", Up: 40, Down: 20, Total: 60, ExpiryTime: -86400000},
		&xray.ClientTraffic{Email: "orphan", Enable: true},
		&model.Inbound{Tag: "report-one", Settings: `{"clients":[{"email":"FIRST","subId":"shared","tgId":982},{"email":"other","subId":"shared","tgId":982},{"email":"legacy","tgId":981,"enable":true}]}`},
		&model.Inbound{Tag: "report-two", Settings: `{"clients":[{"email":"First","subId":"shared","tgId":981},{"email":"Legacy","tgId":982,"enable":true}]}`},
	} {
		if err := db.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	bot := &Tgbot{}
	accounts, err := bot.telegramReportAccounts()
	if err != nil || len(accounts) != 4 {
		t.Fatalf("report returned %+v, %v", accounts, err)
	}
	usage, err := bot.telegramAccountTraffic("FIRST")
	// GORM may apply the traffic table's default enabled value on insert.
	if err != nil || usage == nil || usage.Total != 60 || usage.Up+usage.Down != 60 || usage.ExpiryTime != -86400000 {
		t.Fatalf("usage disagrees with the traffic counter: %+v, %v", usage, err)
	}
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", "first").Update("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	accounts, err = bot.telegramReportAccounts()
	if err != nil {
		t.Fatal(err)
	}
	reports := (telegramWarningThreshold{now: now, expiry: 2000}).recipients(accounts)
	if report := reports[981]; report.disabled != 1 || len(report.expiring) != 1 || report.expiring[0].Email != "Parked" {
		t.Fatalf("unexpected warning: %+v", report)
	}
	if bot.canAccessTelegramClient(982, "First") || bot.canAccessTelegramClient(981, "Legacy") || bot.telegramSubscriptionOwnedBy("First", 981) {
		t.Fatal("Telegram exposed an account or a shared subscription to the wrong identity")
	}
	if err := db.Model(&model.Account{}).Where("email = ?", "Other").Update("tg_id", 981).Error; err != nil {
		t.Fatal(err)
	}
	if !bot.telegramSubscriptionOwnedBy("First", 981) {
		t.Fatal("Telegram blocked a shared subscription owned by one identity")
	}
	if _, err := bot.assignTelegramAccountUser("parked", 983); err != nil {
		t.Fatal(err)
	}
	if !bot.canAccessTelegramClient(983, "Parked") || bot.canAccessTelegramClient(981, "Parked") {
		t.Fatal("Telegram assignment for a parked account did not follow the new owner")
	}
	if _, err := bot.editTelegramAccount("First", func(account *model.Account) error {
		account.TotalGB = 500
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	usage, err = bot.telegramAccountTraffic("First")
	if err != nil || usage == nil || usage.Total != 500 || usage.ExpiryTime != -86400000 {
		t.Fatalf("editing the traffic quota changed the expiry: %+v, %v", usage, err)
	}
}

func TestTelegramNumericCallbackPreservesLegacyAccountNames(t *testing.T) {
	action := "reset_exp_in"
	email := "imported account 23"
	want := []string{action, email, "0"}
	if got := telegramNumericCallback(action+" "+email+"\t0", 2); !slices.Equal(got, want) {
		t.Fatalf("decoded %+v, want %+v", got, want)
	}
}
