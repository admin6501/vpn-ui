package service

import (
	"errors"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

func TestPanelLoginProtectionAndScope(t *testing.T) {
	newAccountsDB(t)
	db := database.GetDB()
	activity := ActivityService{}
	owner := &model.User{Id: 1, IsPanelOwner: true}
	delegate := &model.User{Id: 2, IsSuperAdmin: true}
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	var events []model.PanelLoginEvent
	for i := range 150 {
		at := now.Add(-72 * time.Hour)
		if i >= 80 {
			at = now.Add(-time.Hour)
		}
		events = append(events, model.PanelLoginEvent{At: at.UnixMilli(), UserID: 1 + i%2, Kind: "login"})
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatal(err)
	}
	first, err := activity.LoginEvents(owner, false, 0)
	if err != nil || len(first) != ActivityPageSize+1 {
		t.Fatalf("owner first page: %d, %v", len(first), err)
	}
	next, err := activity.LoginEvents(owner, false, first[ActivityPageSize-1].Id)
	if err != nil || len(next) != ActivityPageSize+1 || next[0].Id >= first[ActivityPageSize-1].Id {
		t.Fatalf("owner next page: %+v, %v", next, err)
	}
	own, err := activity.LoginEvents(delegate, false, 0)
	if err != nil || len(own) != ActivityPageSize+1 {
		t.Fatalf("delegate first page: %d, %v", len(own), err)
	}
	for _, event := range own {
		if event.UserID != delegate.Id {
			t.Fatal("a super admin who is not the panel owner saw another login")
		}
	}
	if err := activity.PurgeLoginEvents(now, now); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&model.PanelLoginEvent{}).Count(&count).Error; err != nil || count != 70 {
		t.Fatalf("recent records: %d, %v", count, err)
	}
	if err := activity.PurgeLoginEvents(now.Add(72*time.Hour), now.Add(72*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.PanelLoginEvent{}).Count(&count).Error; err != nil || count != 60 {
		t.Fatalf("newest records: %d, %v", count, err)
	}
}

func TestExpiryAndRenewalNoticesFollowOwnership(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	activity := ActivityService{}
	var owner model.User
	if err := db.Where("is_panel_owner = ?", true).First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	rep := model.User{Username: "rep", IsReseller: true, Enable: true}
	admin := model.User{Username: "admin-2", Enable: true}
	if err := db.Create(&rep).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// Representative creation reserves the ledger row before creating the account.
	if err := db.Create(&model.ResellerClient{Email: "Renew-Customer", UserId: rep.Id}).Error; err != nil {
		t.Fatal(err)
	}
	repAccount := renewFixture(t, accounts, 10*gb, 0, 7*dayMillis, true)
	rootAccount, err := accounts.SaveAccountWithoutInbound(map[string]any{
		"email": "root-expired", "enable": true, "expiryTime": float64(now.Add(-time.Hour).UnixMilli()),
	}, owner.Id)
	if err != nil {
		t.Fatal(err)
	}
	check := func(user *model.User, kind, email string) ActivityNotice {
		t.Helper()
		notice, err := activity.Notifications(user, kind)
		if err != nil {
			t.Fatal(err)
		}
		if email == "" {
			if notice.Count != 0 {
				t.Fatalf("%s %s: unwanted notice %+v", user.Username, kind, notice)
			}
		} else if notice.Count != 1 || len(notice.Names) != 1 || notice.Names[0] != email {
			t.Fatalf("%s %s: wanted %s, got %+v", user.Username, kind, email, notice)
		}
		return notice
	}
	scan := func() {
		t.Helper()
		if err := activity.ScanExpired(now); err != nil {
			t.Fatal(err)
		}
	}
	scan()
	badge := check(&rep, "expired", repAccount.Email)
	check(&owner, "expired", rootAccount.Email)
	check(&admin, "expired", "")
	if err := activity.DismissNotice(&rep, "expired", badge.Through); err != nil {
		t.Fatal(err)
	}
	scan()
	check(&rep, "expired", "")
	expired, err := accounts.ListExpiredOwnedAccounts(&owner, 1, 30, "", "")
	if err != nil || len(expired.Rows) != 1 || expired.Rows[0].Email != rootAccount.Email {
		t.Fatalf("owner's expired page: %+v, %v", expired, err)
	}

	// Disable the optional change log and exercise a true renew event through
	// the database trigger. An automatic renewal remains visible to both users.
	if err := activity.SetChangeLogging(false); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(repAccount).Updates(map[string]any{
		"last_renewed_at": now.UnixMilli(), "last_renewal_automatic": true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	ownerBadge := check(&owner, "renewals", repAccount.Email)
	check(&rep, "renewals", repAccount.Email)
	check(&admin, "renewals", "")
	if err := activity.DismissNotice(&owner, "renewals", ownerBadge.Through); err != nil {
		t.Fatal(err)
	}
	check(&owner, "renewals", "")
	check(&rep, "renewals", repAccount.Email)
	rows, err := activity.RenewalEvents(&owner, 0)
	if err != nil || len(rows) != 1 || rows[0].Owner != rep.Username {
		t.Fatalf("renewal owner: %+v, %v", rows, err)
	}

	usage := db.Model(&xray.ClientTraffic{}).Where("email=?", repAccount.Email)
	if err := usage.Updates(map[string]any{"up": 0, "down": 0}).Error; err != nil {
		t.Fatal(err)
	}
	scan()
	if err := usage.Update("up", 10*gb).Error; err != nil {
		t.Fatal(err)
	}
	scan()
	if newest := check(&rep, "expired", repAccount.Email); newest.Through <= badge.Through {
		t.Fatal("a new expiry was hidden by the previous dismissal")
	}
	months, err := activity.ChangeMonths()
	if err != nil || len(months) != 1 {
		t.Fatalf("change months: %v, %v", months, err)
	}
	month, err := time.Parse("2006-01", months[0])
	if err != nil {
		t.Fatal(err)
	}
	changes, err := activity.ChangeEvents(month, rep.Username, 0)
	if err != nil || len(changes) != 1 || changes[0].Kind != "created" {
		t.Fatalf("change ownership or toggle: %+v, %v", changes, err)
	}
	if err := activity.PurgeChangeEvents(month); err != nil {
		t.Fatal(err)
	}
	check(&rep, "renewals", repAccount.Email)
	changes, err = activity.ChangeEvents(month, "", 0)
	if err != nil || len(changes) != 0 {
		t.Fatalf("change log purge: %+v, %v", changes, err)
	}
}

func TestSubscriptionAppSettingsCanRemoveDefaultsAndRejectBadURL(t *testing.T) {
	newAccountsDB(t)
	settings := &SettingService{}
	apps, err := settings.SubscriptionApps()
	if err != nil || len(apps) == 0 {
		t.Fatalf("default apps: %+v, %v", apps, err)
	}
	if err := settings.SetSubscriptionApps([]SubscriptionApp{{"android", "Unsafe", "javascript:alert(1)"}}); err == nil {
		t.Fatal("accepted a non-HTTPS app download URL")
	}
	if err := settings.SetSubscriptionApps([]SubscriptionApp{}); err != nil {
		t.Fatal(err)
	}
	apps, err = settings.SubscriptionApps()
	if err != nil || len(apps) != 0 {
		t.Fatalf("deleted defaults reappeared: %+v, %v", apps, err)
	}
}

func TestPanelClientChangeDetailsRecordQuotaExpiryAndTrafficReset(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	activity := ActivityService{}
	now := time.Now().UTC()
	oldExpiry := now.Add(5 * 24 * time.Hour).UnixMilli()
	period := int64(10 * dayMillis)
	account := renewFixture(t, accounts, 10*gb, oldExpiry, period, true)
	newExpiry := oldExpiry + 10*dayMillis
	legacy := model.PanelClientEvent{AccountID: account.Id, Email: account.Email, At: now.UnixMilli(), Kind: "updated", Changes: "quota"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(account).Updates(map[string]any{"total_gb": 11 * gb, "expiry_time": newExpiry}).Error; err != nil {
		t.Fatal(err)
	}
	if err := (&InboundService{}).ResetClientTrafficByEmail(account.Email); err != nil {
		t.Fatal(err)
	}
	if err := accounts.RenewAccount(&model.User{}, account.Email, nil); err != nil {
		t.Fatal(err)
	}
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	events, err := activity.ChangeEvents(month, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var old, changed, reset, renewed bool
	for _, event := range events {
		switch {
		case event.Id == legacy.Id:
			old = event.Details == nil && event.Changes == "quota"
		case event.Kind == "renewed":
			details := event.Details
			renewed = details != nil && details.UsageReset && details.RenewalPeriod != nil &&
				details.RenewalPeriod.After == period
		case event.Changes == "trafficUsage":
			reset = event.Details != nil && event.Details.UsageReset
		case event.Changes == "quota,expiry":
			details := event.Details
			changed = details != nil && details.Quota != nil && details.Expiry != nil && !details.UsageReset &&
				*details.Quota == (model.ClientValueChange{Before: 10 * gb, After: 11 * gb}) &&
				*details.Expiry == (model.ClientValueChange{Before: oldExpiry, After: newExpiry})
		}
	}
	if !old || !changed || !reset || !renewed {
		t.Fatalf("missing expected audit details: old=%t change=%t reset=%t renewal=%t", old, changed, reset, renewed)
	}
}

func TestFirstEditorBecomesOnlyTheUnattributedLegacyOwner(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	var owner model.User
	if err := db.Where("is_panel_owner = ?", true).First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	editor := model.User{Username: "legacy-account-editor", Enable: true, IsSuperAdmin: true}
	if err := db.Create(&editor).Error; err != nil {
		t.Fatal(err)
	}
	account := model.Account{Email: "legacy-account", OwnerAdminID: owner.Id, TotalGB: gb}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"email": account.Email, "totalGB": float64(2 * gb)}
	if _, err := accounts.SaveAccountWithoutInboundAudit(input, editor.Id, database.AccountAudit{ActorID: editor.Id}); err != nil {
		t.Fatal(err)
	}
	input["comment"] = "a different administrator edited this later"
	if _, err := accounts.SaveAccountWithoutInboundAudit(input, owner.Id, database.AccountAudit{ActorID: owner.Id}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	activity := ActivityService{}
	rows, err := activity.ChangeEvents(month, "", 0, &owner)
	if err != nil || len(rows) != 3 || rows[0].OwnerID != editor.Id || rows[0].Owner != editor.Username ||
		rows[0].Actor != owner.Username || rows[1].Actor != editor.Username || rows[2].Owner != editor.Username {
		t.Fatalf("the first editor should establish ownership while later editors retain their own attribution: %+v, %v", rows, err)
	}
	if visible, err := activity.ChangeEvents(month, "", 0, &editor); err != nil || len(visible) != 3 {
		t.Fatalf("inferred owner cannot see the client history: %+v, %v", visible, err)
	}
	var saved model.Account
	if err := db.First(&saved, account.Id).Error; err != nil || saved.InferredOwnerID != editor.Id {
		t.Fatalf("first editor was not persisted as the fallback owner: %+v, %v", saved, err)
	}
	history, err := accounts.AccountHistory(&editor, account.Email, 0)
	if err != nil || len(history.Events) != 3 || history.Events[0].Actor != owner.Username || history.Events[1].Actor != editor.Username {
		t.Fatalf("individual client history lost the actual editor: %+v, %v", history, err)
	}
	if _, err := accounts.SaveAccountWithoutInboundAudit(map[string]any{"email": "new-client"}, editor.Id,
		database.AccountAudit{ActorID: editor.Id}); err != nil {
		t.Fatal(err)
	}
	notice, err := activity.Notifications(&owner, "changes")
	if err != nil || notice.Count != 3 || len(notice.Names) != 2 || notice.Names[0] != "new-client" {
		t.Fatalf("panel owner did not see change and creation notices: %+v, %v", notice, err)
	}
	if editorNotice, err := activity.Notifications(&editor, "changes"); err != nil || editorNotice.Count != 0 {
		t.Fatalf("global change notice reached a delegated admin: %+v, %v", editorNotice, err)
	}
	if err := activity.DismissNotice(&owner, "changes", notice.Through); err != nil {
		t.Fatal(err)
	}
	if dismissed, err := activity.Notifications(&owner, "changes"); err != nil || dismissed.Count != 0 {
		t.Fatalf("the global change notice stayed unread after dismissal: %+v, %v", dismissed, err)
	}
}

func TestLegacyClientRenewalNoticesUseTheConfiguratorAndPanelOwner(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	var owner model.User
	if err := db.Where("is_panel_owner", true).First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	manager := model.User{Username: "legacy-renewal-manager", Enable: true, Permissions: model.PermAccessInbounds, PaymentReminders: true}
	other := model.User{Username: "shared-inbound-admin", Enable: true, Permissions: model.PermAccessInbounds, PaymentReminders: true}
	inbound := seedInboundWithClients(t, model.VLESS, 43340, []map[string]any{})
	for _, user := range []*model.User{&manager, &other} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.InboundAccess{UserId: user.Id, InboundId: inbound.Id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	owner.PaymentReminders = true
	if err := db.Model(&owner).Update("payment_reminders", true).Error; err != nil {
		t.Fatal(err)
	}
	account := model.Account{Email: "legacy-renewal-notice", OwnerAdminID: owner.Id, TotalGB: 5 * gb, Enable: true}
	if err := database.AccountAuditTransaction(db, func(tx *gorm.DB) error {
		return tx.Create(&account).Error
	}, database.AccountAudit{ActorID: manager.Id}); err != nil {
		t.Fatal(err)
	}
	var path string
	if err := db.Raw("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&path).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(path); err != nil {
		t.Fatal(err)
	}
	db = database.GetDB()
	if err := db.First(&account, account.Id).Error; err != nil || account.OwnerAdminID != manager.Id || !account.OwnerConfirmed {
		t.Fatalf("recover canonical owner: %+v %v", account, err)
	}
	if err := db.Create(&model.AccountInbound{AccountId: account.Id, InboundId: inbound.Id}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&xray.ClientTraffic{Email: account.Email, InboundId: inbound.Id, Total: account.TotalGB, Up: account.TotalGB}).Error; err != nil {
		t.Fatal(err)
	}
	activity := ActivityService{}
	if err := activity.SetChangeLogging(false); err != nil {
		t.Fatal(err)
	}
	if err := accounts.SetAutoRenew(account.Email, true, database.AccountAudit{ActorID: other.Id}); err != nil {
		t.Fatal(err)
	}
	if touched, err := accounts.RenewDueAccounts(); err != nil || len(touched) != 1 {
		t.Fatalf("renew the legacy client's depleted package: %v %v", touched, err)
	}
	for _, recipient := range []*model.User{&manager, &owner, &other} {
		want := int64(1)
		if recipient.Id == manager.Id {
			want = 0
		}
		notice, err := activity.Notifications(recipient, "renewals")
		if err != nil || notice.Count != want {
			t.Fatalf("renewal notice to %s: %+v %v", recipient.Username, notice, err)
		}
		if recipient.Id == owner.Id {
			if err := activity.DismissNotice(&owner, "renewals", notice.Through); err != nil {
				t.Fatal(err)
			}
		}
	}
	if notice, err := activity.Notifications(&other, "renewals"); err != nil || notice.Count != 1 {
		t.Fatalf("panel owner's dismissal hid the configurator's notice: %+v %v", notice, err)
	}
	logs, err := activity.RenewalEvents(&owner, 0)
	if err != nil || len(logs) != 1 || logs[0].Owner != manager.Username || logs[0].Actor != other.Username {
		t.Fatalf("automatic renewal should record its owner and configurator: %+v %v", logs, err)
	}
	rows, err := activity.PendingPayments(&other, 0)
	if err != nil || len(rows) != 1 || rows[0].Owner != manager.Username || rows[0].Actor != other.Username ||
		rows[0].Kind != "autoRenewed" {
		t.Fatalf("pending payment for automatic renewal configurator: %+v %v", rows, err)
	}
	for _, unrelated := range []*model.User{&manager, &owner} {
		if notice, err := activity.PaymentNotice(unrelated); err != nil || notice.Count != 0 {
			t.Fatalf("payment notice to %s: %+v %v", unrelated.Username, notice, err)
		}
		if err := activity.ConfirmPayment(unrelated, rows[0].ID); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("%s was allowed to confirm an unrelated payment: %v", unrelated.Username, err)
		}
	}
	if err := activity.ConfirmPayment(&other, rows[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestClientActivityDetailsTrackInboundAssignments(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	var owner model.User
	if err := db.Where("is_panel_owner", true).First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	first := seedInboundWithClients(t, model.VLESS, 43341, []map[string]any{})
	second := seedInboundWithClients(t, model.Trojan, 43342, []map[string]any{})
	for inbound, name := range map[*model.Inbound]string{first: "Primary Inbound", second: "Replacement Inbound"} {
		if err := db.Model(inbound).Update("remark", name).Error; err != nil {
			t.Fatal(err)
		}
	}
	create := database.AccountAudit{ActorID: owner.Id, Operation: "create-with-inbound"}
	account, err := accounts.SaveAccountWithoutInboundAudit(map[string]any{"email": "membership-audit", "enable": true}, owner.Id, create)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.ApplyMembershipsFrom(account.Email, 0, []int{first.Id}, nil, true, create); err != nil {
		t.Fatal(err)
	}
	replace := database.AccountAudit{ActorID: owner.Id, Operation: "replace-inbound"}
	if _, err := accounts.ApplyMembershipsFrom(account.Email, 0, []int{second.Id}, []int{first.Id}, true, replace); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.SetMembershipEnable(account.Email, second.Id, false, database.AccountAudit{ActorID: owner.Id}); err != nil {
		t.Fatal(err)
	}
	history, err := accounts.AccountHistory(&owner, account.Email, 0)
	if err != nil || len(history.Events) != 3 {
		t.Fatalf("history must group each operation: %+v %v", history, err)
	}
	enable, swap, initial := history.Events[0].Details.Inbounds, history.Events[1].Details.Inbounds, history.Events[2].Details.Inbounds
	if len(initial) != 1 || initial[0].Name != "Primary Inbound" || initial[0].Protocol != string(model.VLESS) ||
		initial[0].Before != nil || initial[0].After == nil || !*initial[0].After ||
		len(swap) != 2 || swap[0].ID != first.Id || swap[0].After != nil || swap[1].ID != second.Id || swap[1].Before != nil ||
		len(enable) != 1 || enable[0].Name != "Replacement Inbound" || enable[0].After == nil || *enable[0].After {
		t.Fatalf("unexpected inbound changes: initial=%+v swap=%+v enable=%+v", initial, swap, enable)
	}
}

func TestClientActivityDeletionCapturesOneEventAndTraffic(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	var owner model.User
	if err := db.Where("is_panel_owner = ?", true).First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	editor := model.User{Username: "client-deleter", Enable: true, IsSuperAdmin: true}
	if err := db.Create(&editor).Error; err != nil {
		t.Fatal(err)
	}
	first := seedInboundWithClients(t, model.VLESS, 43331, []map[string]any{})
	second := seedInboundWithClients(t, model.Trojan, 43332, []map[string]any{})
	groups := &GroupService{}
	group, _, err := groups.Save(&owner, 0, "Audit deletion", []int{first.Id, second.Id})
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(10 * 24 * time.Hour).UnixMilli()
	entry := map[string]any{"email": "deleted-audit-client", "totalGB": float64(10 * gb), "enable": true,
		"expiryTime": float64(expiry), "renewalDuration": float64(10 * dayMillis)}
	account, err := accounts.SaveAccountWithoutInboundAudit(entry, owner.Id, database.AccountAudit{ActorID: owner.Id})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := groups.Apply(&owner, account.Email, 0, []int{group.ID}); err != nil {
		t.Fatal(err)
	}
	entry["totalGB"] = float64(12 * gb)
	if _, err := accounts.SaveAccountWithoutInboundAudit(entry, owner.Id, database.AccountAudit{ActorID: owner.Id}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", account.Email).
		Updates(map[string]any{"up": 2 * gb, "down": 3 * gb, "all_time": 8 * gb}).Error; err != nil {
		t.Fatal(err)
	}
	var panelCursor, historyCursor int64
	if err := db.Model(&model.PanelClientEvent{}).Select("MAX(id)").Scan(&panelCursor).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.AccountEvent{}).Select("MAX(id)").Scan(&historyCursor).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := (TrashService{}).DeleteClient(&editor, account.Email); err != nil {
		t.Fatal(err)
	}
	var rows []model.PanelClientEvent
	if err := db.Where("account_id = ? AND id > ?", account.Id, panelCursor).Find(&rows).Error; err != nil ||
		len(rows) != 1 || rows[0].Kind != "deleted" || rows[0].Actor != editor.Username || rows[0].Changes != "" {
		t.Fatalf("expected one deletion event: %+v, %v", rows, err)
	}
	if rows[0].Details == nil || rows[0].Details.Deleted == nil {
		t.Fatal("missing deletion snapshot")
	}
	details := rows[0].Details.Deleted
	if details.Quota != 12*gb || details.Expiry != expiry || details.Period != 10*dayMillis || !details.Enabled ||
		details.InitialQuota == nil || *details.InitialQuota != 10*gb ||
		details.Upload == nil || *details.Upload != 2*gb || details.Download == nil || *details.Download != 3*gb ||
		details.Lifetime == nil || *details.Lifetime != 8*gb {
		t.Fatalf("deletion snapshot: %+v", details)
	}
	var history []model.AccountEvent
	if err := db.Where("account_id = ? AND id > ?", account.Id, historyCursor).Find(&history).Error; err != nil ||
		len(history) != 1 || history[0].Kind != "deleted" {
		t.Fatalf("account history recorded association cleanup: %+v, %v", history, err)
	}
}

func TestClientActivityGroupsARequestAndKeepsAccountHistory(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	activity := ActivityService{}
	var owner model.User
	if err := db.Where("is_panel_owner = ?", true).First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	admin := &model.User{Username: "audit-admin", Enable: true, IsSuperAdmin: true}
	other := &model.User{Username: "audit-other", Enable: true, IsSuperAdmin: true}
	if err := db.Create(admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatal(err)
	}
	inbound := seedInboundWithClients(t, model.VLESS, 43321, []map[string]any{})
	accounts.MigrationAccounts()
	groups := &GroupService{}
	if err := groups.Migrate(); err != nil {
		t.Fatal(err)
	}
	group, _, err := groups.Save(&owner, 0, "Audit test", []int{inbound.Id})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	email := "grouped-audit"
	expiry := now.Add(10 * 24 * time.Hour).UnixMilli()
	client := map[string]any{"email": email, "enable": true, "totalGB": float64(2 * gb),
		"expiryTime": float64(expiry), "renewalDuration": float64(10 * dayMillis)}
	if _, err := accounts.SaveAccountWithoutInboundAudit(client, admin.Id, database.AccountAudit{Operation: "create-request"}); err != nil {
		t.Fatal(err)
	}
	if _, err := groups.Apply(&owner, email, 0, []int{group.ID}, database.AccountAudit{Operation: "create-request"}); err != nil {
		t.Fatal(err)
	}
	client["totalGB"] = float64(3 * gb)
	client["expiryTime"] = float64(expiry + 5*dayMillis)
	if _, err := accounts.SaveAccountWithoutInboundAudit(client, admin.Id, database.AccountAudit{Operation: "edit-request"}); err != nil {
		t.Fatal(err)
	}
	if _, err := groups.Apply(&owner, email, 0, []int{group.ID}, database.AccountAudit{Operation: "edit-request"}); err != nil {
		t.Fatal(err)
	}
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	rows, err := activity.ChangeEvents(month, "", 0, admin)
	if err != nil || len(rows) != 2 || rows[0].Kind != "updated" || rows[1].Kind != "created" {
		t.Fatalf("expected exactly one row per edit and create: %+v, %v", rows, err)
	}
	if detail := rows[0].Details; detail == nil || detail.Quota == nil || detail.Expiry == nil ||
		*detail.Quota != (model.ClientValueChange{Before: 2 * gb, After: 3 * gb}) ||
		*detail.Expiry != (model.ClientValueChange{Before: expiry, After: expiry + 5*dayMillis}) ||
		rows[0].Changes != "quota,expiry" {
		t.Fatalf("edit details or unchanged group: %+v / %s", detail, rows[0].Changes)
	}
	if detail := rows[1].Details; detail == nil || detail.Created == nil ||
		detail.Created.Quota != 2*gb || detail.Created.Expiry != expiry || detail.Created.Period != 10*dayMillis {
		t.Fatalf("original package values missing: %+v", detail)
	}
	previous, err := activity.ChangeEvents(month, "", rows[0].Id, admin)
	if err != nil || len(previous) != 1 || previous[0].Id != rows[1].Id {
		t.Fatalf("operation pagination: %+v, %v", previous, err)
	}
	if err := activity.HideClientChange(other, rows[1].Id); err != nil {
		t.Fatal(err)
	}
	if rows, err := activity.ChangeEvents(month, "", 0, other); err != nil || len(rows) != 0 {
		t.Fatalf("another admin saw hidden or unowned entries: %+v, %v", rows, err)
	}
	if rows, err := activity.ChangeEvents(month, "", 0, admin); err != nil || len(rows) != 2 {
		t.Fatalf("another admin affected my history: %+v, %v", rows, err)
	}
	if err := activity.HideClientChange(admin, rows[1].Id); err != nil {
		t.Fatal(err)
	}
	if rows, err := activity.ChangeEvents(month, "", 0, admin); err != nil || len(rows) != 1 || rows[0].Kind != "updated" {
		t.Fatalf("whole operation was not hidden: %+v, %v", rows, err)
	}
	if rows, err := activity.ChangeEvents(month, "", 0, &owner); err != nil || len(rows) != 2 {
		t.Fatalf("owner lost access to hidden record: %+v, %v", rows, err)
	}
	checkHistory := func() {
		t.Helper()
		history, err := accounts.AccountHistory(&owner, email, 0)
		if err != nil || len(history.Events) != 2 || history.Events[1].Kind != "created" ||
			history.Events[1].Details == nil || history.Events[1].Details.Created == nil {
			t.Fatalf("grouped account history: %+v, %v", history, err)
		}
	}
	checkHistory()

	nextYear := now.AddDate(1, 0, 0)
	if err := activity.PurgeExpiredClientChanges(nextYear); err != nil {
		t.Fatal(err)
	}
	if count, err := activity.ChangeRetention(); err != nil || count != 0 {
		t.Fatalf("automatic purging must be off by default: %d, %v", count, err)
	}
	if rows, err := activity.ChangeEvents(month, "", 0, &owner); err != nil || len(rows) != 2 {
		t.Fatalf("disabled retention removed records: %+v, %v", rows, err)
	}
	if err := db.Model(&model.PanelClientEvent{}).Where("account_id = ? AND audit_operation = ?", rows[1].AccountID, "create-request").
		Update("at", now.Add(-61*24*time.Hour).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if err := activity.SetChangeRetention(60); err != nil {
		t.Fatal(err)
	}
	if err := activity.PurgeExpiredClientChanges(now); err != nil {
		t.Fatal(err)
	}
	var left int64
	if err := db.Model(&model.PanelClientEvent{}).Where("account_id = ? AND audit_operation = ?", rows[1].AccountID, "create-request").
		Count(&left).Error; err != nil || left != 0 {
		t.Fatalf("aged operation remains after automatic purge: %d, %v", left, err)
	}
	var hidden int64
	if err := db.Model(&model.PanelClientHidden{}).Count(&hidden).Error; err != nil || hidden != 0 {
		t.Fatalf("aged hide markers remain: %d, %v", hidden, err)
	}
	if rows, err := activity.ChangeEvents(month, "", 0, &owner); err != nil || len(rows) != 1 {
		t.Fatalf("retention affected a recent operation: %+v, %v", rows, err)
	}
	checkHistory()
}
