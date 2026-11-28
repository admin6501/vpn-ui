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

func TestPaymentReminderLifecycle(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	activity := ActivityService{}
	var owner model.User
	if err := db.Where("is_panel_owner").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	setReminders := func(enabled bool) {
		t.Helper()
		if err := activity.SetPaymentReminders(&owner, enabled); err != nil {
			t.Fatal(err)
		}
		if err := db.First(&owner, owner.Id).Error; err != nil || owner.PaymentReminders != enabled {
			t.Fatalf("stored reminder preference: %t, %v", owner.PaymentReminders, err)
		}
	}
	pending := func(kind string) int64 {
		t.Helper()
		rows, err := activity.PendingPayments(&owner, 0)
		if err != nil {
			t.Fatal(err)
		}
		notice, err := activity.Notifications(&owner, "payments")
		if err != nil || notice.Count != int64(len(rows)) {
			t.Fatalf("notice does not match pending rows: %+v, %v", notice, err)
		}
		if kind == "" && len(rows) == 0 {
			return 0
		}
		if kind == "" || len(rows) != 1 || rows[0].Kind != kind || rows[0].Email != "customer" ||
			len(notice.Names) != 1 || notice.Names[0] != "customer" {
			t.Fatalf("expected pending %q, got %+v and %+v", kind, rows, notice)
		}
		return rows[0].ID
	}
	confirm := func(revision int64) {
		t.Helper()
		if err := activity.ConfirmPayment(&owner, revision); err != nil {
			t.Fatal(err)
		}
		pending("")
	}
	if owner.PaymentReminders {
		t.Fatal("reminders should be off by default")
	}
	if err := db.Create(&model.Account{Email: "legacy-import"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := activity.SetChangeLogging(false); err != nil {
		t.Fatal(err)
	}
	entry := map[string]any{"email": "customer", "enable": true, "totalGB": float64(5 * gb), "expiryTime": float64(-7 * dayMillis)}
	account, err := accounts.SaveAccountWithoutInbound(entry, owner.Id)
	if err != nil {
		t.Fatal(err)
	}
	pending("")
	setReminders(true)
	created := pending("created")
	setReminders(false)
	pending("")
	if err := activity.ConfirmPayment(&owner, created); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("disabled reminders could be confirmed: %v", err)
	}
	setReminders(true)
	if pending("created") != created {
		t.Fatal("turning reminders off discarded the pending payment")
	}
	confirm(created)

	// Starting the period at first use and editing metadata do not create a sale.
	expiry := time.Now().UnixMilli() + 7*dayMillis
	if err := db.Model(account).Update("expiry_time", expiry).Error; err != nil {
		t.Fatal(err)
	}
	entry["expiryTime"], entry["comment"] = float64(expiry), "edited comment"
	save := func() {
		t.Helper()
		if _, err := accounts.SaveAccountWithoutInbound(entry, owner.Id); err != nil {
			t.Fatal(err)
		}
	}
	save()
	pending("")
	entry["totalGB"] = float64(6 * gb)
	save()
	quota := pending("updated")
	entry["expiryTime"] = float64(expiry + dayMillis)
	save()
	extension := pending("updated")
	if extension <= quota {
		t.Fatal("the latest extension did not replace the pending revision")
	}
	if err := activity.ConfirmPayment(&owner, quota); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("a stale confirmation approved a newer extension: %v", err)
	}
	confirm(extension)

	if err := accounts.RenewAccount(&owner, account.Email, nil); err != nil {
		t.Fatal(err)
	}
	confirm(pending("renewed"))
	if err := accounts.SetAutoRenew(account.Email, true); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&xray.ClientTraffic{}).Where("email=?", account.Email).Update("expiry_time", 1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.RenewDueAccounts(); err != nil {
		t.Fatal(err)
	}
	pending("autoRenewed")
	if err := accounts.DeleteAccountWithoutInbound(account.Email); err != nil {
		t.Fatal(err)
	}
	pending("")
	var reminders int64
	if err := db.Model(&model.PanelPaymentReminder{}).Where("account_id=?", account.Id).Count(&reminders).Error; err != nil || reminders != 0 {
		t.Fatalf("deleting the client left reminder state: %d, %v", reminders, err)
	}
}

func TestDismissedPaymentNoticeKeepsUnconfirmedPayments(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	activity := ActivityService{}
	var owner model.User
	if err := db.Where("is_panel_owner").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	owner.PaymentReminders = true
	oldPayment := map[string]any{"email": "older-payment", "totalGB": float64(gb)}
	for _, entry := range []map[string]any{oldPayment, {"email": "another-pending-payment"}} {
		if _, err := accounts.SaveAccountWithoutInbound(entry, owner.Id); err != nil {
			t.Fatal(err)
		}
	}
	initial, err := activity.Notifications(&owner, "payments")
	if err != nil || initial.Count != 2 || initial.Through == 0 {
		t.Fatalf("initial payment notice: %+v, %v", initial, err)
	}
	if err := activity.DismissNotice(&owner, "payments", initial.Through); err != nil {
		t.Fatal(err)
	}
	if dismissed, err := activity.Notifications(&owner, "payments"); err != nil || dismissed.Count != 0 {
		t.Fatalf("dismissed payments still counted: %+v, %v", dismissed, err)
	}
	if rows, err := activity.PendingPayments(&owner, 0); err != nil || len(rows) != 2 {
		t.Fatalf("dismissing the notice modified unconfirmed payments: %+v, %v", rows, err)
	}
	oldPayment["totalGB"] = float64(2 * gb)
	if _, err := accounts.SaveAccountWithoutInbound(oldPayment, owner.Id); err != nil {
		t.Fatal(err)
	}
	if err := activity.DismissNotice(&owner, "payments", initial.Through); err != nil {
		t.Fatal(err)
	}
	if fresh, err := activity.Notifications(&owner, "payments"); err != nil || fresh.Count != 1 ||
		fresh.Through <= initial.Through || len(fresh.Names) != 1 || fresh.Names[0] != "older-payment" {
		t.Fatalf("only the new payment revision should be counted: %+v, %v", fresh, err)
	}
	if rows, err := activity.PendingPayments(&owner, 0); err != nil || len(rows) != 2 {
		t.Fatalf("expected both unpaid clients: %+v, %v", rows, err)
	}
}

func TestOnlyTheBillableChangeActorCanSeeAndConfirmPayment(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	activity := ActivityService{}
	var owner model.User
	if err := db.Where("is_panel_owner").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	delegated := model.User{Username: "delegated", Enable: true, Permissions: model.PermAccessInbounds}
	rep := model.User{Username: "representative", Enable: true, IsReseller: true}
	for _, user := range []*model.User{&delegated, &rep} {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range []*model.User{&owner, &delegated, &rep} {
		if err := activity.SetPaymentReminders(user, true); err != nil {
			t.Fatal(err)
		}
		if err := db.First(user, user.Id).Error; err != nil {
			t.Fatal(err)
		}
	}
	ledger := model.ResellerClient{Email: "Group-Customer", UserId: rep.Id}
	if err := db.Create(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.SaveAccountWithoutInbound(map[string]any{"email": ledger.Email, "enable": true}, delegated.Id); err != nil {
		t.Fatal(err)
	}
	pending := func(user *model.User, count int) []PaymentReminder {
		t.Helper()
		rows, err := activity.PendingPayments(user, 0)
		if err != nil || len(rows) != count {
			t.Fatalf("%s saw %d payments, want %d: %v", user.Username, len(rows), count, err)
		}
		return rows
	}
	created := pending(&delegated, 1)[0]
	revision := created.ID
	if created.Owner != rep.Username || created.Actor != delegated.Username {
		t.Fatalf("payment did not preserve the client owner and the operator: %+v", created)
	}
	for _, user := range []*model.User{&owner, &rep} {
		pending(user, 0)
		if err := activity.ConfirmPayment(user, revision); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("%s confirmed another user's payment: %v", user.Username, err)
		}
	}
	if err := db.Delete(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if pending(&delegated, 1)[0].ID != revision {
		t.Fatal("an ownership change moved the outstanding payment to another actor")
	}
	if err := activity.ConfirmPayment(&delegated, revision); err != nil {
		t.Fatal(err)
	}
	pending(&delegated, 0)
}
