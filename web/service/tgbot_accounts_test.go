package service

import (
	"slices"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/config"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestTelegramGroupedAccountCreationAndParkedManagement(t *testing.T) {
	t.Setenv("VPNUI_DB_FOLDER", t.TempDir())
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	db := database.GetDB()
	var groupIDs, inboundIDs []int
	for i, protocol := range []model.Protocol{model.VLESS, model.Trojan} {
		inbound := model.Inbound{Tag: string(protocol), Protocol: protocol, Port: 34443 + i, Settings: `{"clients":[]}`}
		group := model.ClientGroup{Name: string(protocol)}
		for _, row := range []any{&inbound, &group} {
			if err := db.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Create(&model.GroupInbound{GroupID: group.ID, InboundID: inbound.Id}).Error; err != nil {
			t.Fatal(err)
		}
		groupIDs = append(groupIDs, group.ID)
		inboundIDs = append(inboundIDs, inbound.Id)
	}
	const email, telegramID = "two-groups", 157896
	bot := &Tgbot{draft: &telegramClientDraft{groupIDs: groupIDs, Client: model.Client{
		ID: "12345678-1234-4234-9234-123456789abc", Email: email, SubID: "grouped-telegram", TgID: telegramID, Enable: true,
	}}}
	if _, err := bot.SubmitAddClient(); err != nil {
		t.Fatal(err)
	}
	accounts := &AccountService{}
	got, err := accounts.InboundIdsForEmail(email)
	if err != nil || !slices.Equal(got, inboundIDs) {
		t.Fatalf("expected both group-derived memberships, got %v, err=%v", got, err)
	}
	account, err := accounts.GetAccountByEmail(email)
	if err != nil || account == nil || account.Password == "" {
		t.Fatalf("mixed group credentials were not generated: %v", err)
	}
	if _, err := (&GroupService{}).Apply(telegramGroupOperator(), email, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.editTelegramAccount(email, func(a *model.Account) error {
		a.TotalGB = 3 << 30
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	usage, err := bot.telegramAccountTraffic(email)
	if err != nil || usage == nil || usage.Total != 3<<30 {
		t.Fatalf("parked account quota was lost: %v", err)
	}
	rows, err := bot.telegramUserAccounts(telegramID)
	if err != nil || len(rows) != 1 || rows[0].Email != email {
		t.Fatalf("parked account not discoverable by its Telegram owner: %v", err)
	}
	if bot.canAccessTelegramClient(telegramID+1, email) {
		t.Fatal("another Telegram user was allowed to access the account")
	}
	bot.draft.groupIDs = []int{groupIDs[0], 9999}
	bot.draft.Email = "incomplete"
	if _, err := bot.SubmitAddClient(); err == nil {
		t.Fatal("missing group accepted")
	}
	if account, err := accounts.GetAccountByEmail("incomplete"); err != nil || account != nil {
		t.Fatalf("failed group assignment left an account: %v", err)
	}
}
