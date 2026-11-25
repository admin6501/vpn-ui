package service

import (
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
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
