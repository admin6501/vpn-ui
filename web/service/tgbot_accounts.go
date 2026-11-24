package service

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/global"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
	"gorm.io/gorm"
)

func telegramGroupOperator() *model.User {
	return &model.User{Enable: true, IsSuperAdmin: true}
}

func (t *Tgbot) telegramGroupKeyboard(selected []int, prefix, suffix string) (*telego.InlineKeyboardMarkup, error) {
	groups, err := (&GroupService{}).List(telegramGroupOperator(), false)
	if err != nil {
		return nil, err
	}
	var rows [][]telego.InlineKeyboardButton
	for _, group := range groups {
		if len(group.InboundIDs) == 0 && prefix == "add_client_group" {
			continue
		}
		label := group.Name
		if slices.Contains(selected, group.ID) {
			label = "✓ " + label
		}
		rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(label).
			WithCallbackData(t.encodeQuery(fmt.Sprintf("%s %d%s", prefix, group.ID, suffix)))))
	}
	if len(rows) == 0 {
		return nil, errors.New(t.I18nBot("pages.groups.noGroups"))
	}
	if prefix == "add_client_group" && len(selected) != 0 {
		rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(t.I18nBot("confirm")).
			WithCallbackData("add_client_groups_done")))
	}
	return tu.InlineKeyboard(rows...), nil
}

func (t *Tgbot) draftInbound() (*model.Inbound, error) {
	if len(t.draft.groupIDs) > 0 {
		return &model.Inbound{Remark: t.I18nBot("pages.groups.title"), Protocol: model.VLESS}, nil
	}
	return t.inboundService.GetInbound(t.draft.inboundID)
}

func (t *Tgbot) createGroupedTelegramAccount() ([]int, error) {
	client := t.draft.Client
	client.Email = strings.TrimSpace(client.Email)
	if err := ValidateClientEmail(client.Email); err != nil {
		return nil, err
	}
	if err := ValidateClientSubID(client.SubID); err != nil {
		return nil, err
	}
	if err := validateClientLimits([]model.Client{client}); err != nil {
		return nil, err
	}
	var changed []int
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		key := accountKey(client.Email)
		for _, table := range []string{"accounts", "client_traffics"} {
			var n int64
			if err := tx.Table(table).Where("LOWER(TRIM(email)) = ?", key).Count(&n).Error; err != nil {
				return err
			}
			if n != 0 {
				return fmt.Errorf("the email %q is already in use", client.Email)
			}
		}
		account := model.Account{
			Email: client.Email, SubID: client.SubID, UUID: client.ID, Security: client.Security,
			Enable: client.Enable, TotalGB: client.TotalGB, ExpiryTime: client.ExpiryTime,
			LimitIP: client.LimitIP, TgID: client.TgID, Comment: client.Comment,
		}
		if err := captureRenewalDuration(&account, nil, nil); err != nil {
			return err
		}
		if err := tx.Create(&account).Error; err != nil {
			return err
		}
		for _, id := range t.draft.groupIDs {
			var group model.ClientGroup
			if err := tx.Where("disabled = ?", false).First(&group, id).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.AccountGroup{AccountID: account.Id, GroupID: id}).Error; err != nil {
				return err
			}
		}
		ids, err := groupUnion(tx, account.Id)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return errors.New(t.I18nBot("pages.groups.noGroups"))
		}
		if err := tx.Create(&xray.ClientTraffic{
			Email: account.Email, Enable: account.Enable, Total: account.TotalGB, ExpiryTime: account.ExpiryTime,
		}).Error; err != nil {
			return err
		}
		changed, err = projectGroups(tx, account.Id)
		return err
	})
	return changed, err
}

func (t *Tgbot) refreshTelegramMemberships(ids []int) {
	if panel, ok := global.GetWebServer().(interface{ ReconcileTelegramMemberships([]int) }); ok {
		panel.ReconcileTelegramMemberships(ids)
	}
	t.notifyPanelClientChange(len(ids) != 0)
}

func (t *Tgbot) telegramAccountTraffic(email string) (*xray.ClientTraffic, error) {
	account, err := (&AccountService{}).GetAccountByEmail(email)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return t.inboundService.GetClientTrafficByEmail(email)
	}
	var usage xray.ClientTraffic
	err = database.GetDB().Where("LOWER(TRIM(email)) = ?", accountKey(email)).First(&usage).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	usage.Email, usage.SubId, usage.UUID = account.Email, account.SubID, account.UUID
	usage.Total, usage.ExpiryTime = account.TotalGB, account.ExpiryTime
	usage.Enable = account.Enable && !account.RepresentativeBlocked && (usage.Id == 0 || usage.Enable)
	return &usage, nil
}

func (t *Tgbot) telegramEmails() ([]string, error) {
	var accounts, legacy []string
	db := database.GetDB()
	if err := db.Model(&model.Account{}).Order("email").Pluck("email", &accounts).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&xray.ClientTraffic{}).Order("email").Pluck("email", &legacy).Error; err != nil {
		return nil, err
	}
	emails := make([]string, 0, len(accounts))
	seen := make(map[string]bool)
	for _, email := range append(accounts, legacy...) {
		key := accountKey(email)
		if key != "" && !seen[key] {
			seen[key] = true
			emails = append(emails, email)
		}
	}
	slices.Sort(emails)
	return emails, nil
}

func (t *Tgbot) telegramAccountList(chatID int64, action string) {
	emails, err := t.telegramEmails()
	if err != nil {
		t.SendMsgToTgbot(chatID, t.I18nBot("tgbot.answers.errorOperation"))
		return
	}
	var rows [][]telego.InlineKeyboardButton
	for _, email := range emails {
		rows = append(rows, tu.InlineKeyboardRow(tu.InlineKeyboardButton(email).WithCallbackData(t.encodeQuery(action+" "+email))))
		if len(rows) == 90 {
			t.SendMsgToTgbot(chatID, t.I18nBot("tgbot.commands.pleaseChoose"), tu.InlineKeyboard(rows...))
			rows = nil
		}
	}
	if len(rows) != 0 {
		t.SendMsgToTgbot(chatID, t.I18nBot("tgbot.commands.pleaseChoose"), tu.InlineKeyboard(rows...))
	} else if len(emails) == 0 {
		t.SendMsgToTgbot(chatID, t.I18nBot("tgbot.noResult"))
	}
}

func (t *Tgbot) manageTelegramGroups(chatID int64, messageID int, email, toggle string) error {
	account, err := (&AccountService{}).GetAccountByEmail(email)
	if err != nil {
		return err
	}
	if account == nil {
		return errors.New(t.I18nBot("tgbot.noResult"))
	}
	var selected []int
	if err := database.GetDB().Model(&model.AccountGroup{}).Where("account_id = ?", account.Id).Pluck("group_id", &selected).Error; err != nil {
		return err
	}
	if toggle != "" {
		id, err := strconv.Atoi(toggle)
		if err != nil {
			return err
		}
		var touched []int
		err = database.GetDB().Transaction(func(tx *gorm.DB) error {
			var group model.ClientGroup
			if err := tx.Where("disabled = ?", false).First(&group, id).Error; err != nil {
				return err
			}
			var row model.AccountGroup
			err := tx.Where("account_id = ? AND group_id = ?", account.Id, group.ID).First(&row).Error
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				err = tx.Create(&model.AccountGroup{AccountID: account.Id, GroupID: group.ID}).Error
			case err == nil:
				err = tx.Delete(&row).Error
			}
			if err != nil {
				return err
			}
			touched, err = projectGroups(tx, account.Id)
			if err != nil {
				return err
			}
			return tx.Model(&model.AccountGroup{}).Where("account_id = ?", account.Id).Pluck("group_id", &selected).Error
		})
		if err != nil {
			return err
		}
		t.refreshTelegramMemberships(touched)
	}
	keyboard, err := t.telegramGroupKeyboard(selected, "client_group_toggle", " "+email)
	if err != nil {
		return err
	}
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, tu.InlineKeyboardRow(
		tu.InlineKeyboardButton(t.I18nBot("close")).WithCallbackData(t.encodeQuery("client_refresh "+email)),
	))
	t.editMessageTgBot(chatID, messageID, t.I18nBot("pages.groups.title"), keyboard)
	return nil
}

func (t *Tgbot) editTelegramAccount(email string, edit func(*model.Account) error, legacy func() (bool, error)) (bool, error) {
	var found bool
	var touched []int
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		accounts := &AccountService{}
		account, err := accounts.GetAccountByEmailTx(tx, email)
		if err != nil || account == nil {
			return err
		}
		found = true
		previous := *account
		if err := edit(account); err != nil {
			return err
		}
		if err := validateClientLimits([]model.Client{{TotalGB: account.TotalGB, ExpiryTime: account.ExpiryTime, LimitIP: account.LimitIP}}); err != nil {
			return err
		}
		if account.ExpiryTime != previous.ExpiryTime {
			if err := captureRenewalDuration(account, &previous, map[string]any{"expiryTime": float64(account.ExpiryTime)}); err != nil {
				return err
			}
		}
		if err := tx.Save(account).Error; err != nil {
			return err
		}
		if account.TotalGB != previous.TotalGB || account.ExpiryTime != previous.ExpiryTime {
			if err := tx.Model(&xray.ClientTraffic{}).Where("LOWER(TRIM(email)) = ?", accountKey(email)).
				Updates(map[string]any{"total": account.TotalGB, "expiry_time": account.ExpiryTime}).Error; err != nil {
				return err
			}
		}
		touched, err = accounts.ProjectAccount(tx, account.Id)
		return err
	})
	if err != nil {
		return false, err
	}
	if !found {
		return legacy()
	}
	t.refreshTelegramMemberships(touched)
	return len(touched) != 0, nil
}

func (t *Tgbot) assignTelegramAccountUser(trafficID int, telegramID int64) (bool, error) {
	var traffic xray.ClientTraffic
	if err := database.GetDB().First(&traffic, trafficID).Error; err != nil {
		return false, err
	}
	return t.editTelegramAccount(traffic.Email, func(a *model.Account) error {
		a.TgID = telegramID
		return nil
	}, func() (bool, error) { return t.inboundService.SetClientTelegramUserID(trafficID, telegramID) })
}

func (t *Tgbot) telegramUserAccounts(telegramID int64) ([]*xray.ClientTraffic, error) {
	if telegramID <= 0 {
		return nil, nil
	}
	var emails []string
	if err := database.GetDB().Model(&model.Account{}).Where("tg_id = ?", telegramID).Order("email").Pluck("email", &emails).Error; err != nil {
		return nil, err
	}
	legacy, err := t.inboundService.GetClientTrafficTgBot(telegramID)
	if err != nil {
		return nil, err
	}
	for _, row := range legacy {
		emails = append(emails, row.Email)
	}
	seen := make(map[string]bool)
	var rows []*xray.ClientTraffic
	for _, email := range emails {
		key := accountKey(email)
		if seen[key] || !t.canAccessTelegramClient(telegramID, email) {
			continue
		}
		seen[key] = true
		row, err := t.telegramAccountTraffic(email)
		if err != nil {
			return nil, err
		}
		if row != nil {
			rows = append(rows, row)
		}
	}
	return rows, nil
}
