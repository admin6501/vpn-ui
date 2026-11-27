package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

func TestTrashClientPrivacyAndRestoreWithNewAllowance(t *testing.T) {
	newInboundDB(t)
	db := database.GetDB()
	var owner model.User
	if err := db.Where("is_panel_owner = ?", true).First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	admin := model.User{Username: "client-owner", GroupAccess: true,
		Permissions: model.PermAccessInbounds | model.PermCreateClient | model.PermDeleteClient}
	other := model.User{Username: "other-admin", IsSuperAdmin: true}
	group := model.ClientGroup{Name: "customer-group"}
	first := model.Inbound{Tag: "first", Port: 3001, Protocol: model.VLESS, Settings: `{"clients":[]}`}
	second := model.Inbound{Tag: "second", Port: 3002, Protocol: model.Trojan, Settings: `{"clients":[]}`}
	for _, row := range []any{&admin, &other, &group, &first, &second} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	expiry := time.Now().AddDate(0, 0, 20).UnixMilli()
	account := model.Account{OwnerAdminID: admin.Id, Email: "trashed-client", SubID: "trash-sub", UUID: "test-uuid",
		Password: "test-password", ProtocolCredentials: `{"trojan":"test-password"}`, TotalGB: 1<<54 + 1,
		ExpiryTime: expiry, Comment: "client note", Enable: true, AutoRenew: true, RenewalDuration: 86400000}
	if err := db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{
		&model.AccountInbound{AccountId: account.Id, InboundId: first.Id, Extra: `{"totalGB":18014398509481985,"custom":"config"}`, Up: 60},
		&model.AccountInbound{AccountId: account.Id, InboundId: second.Id, Extra: `{"expiryTime":123,"custom":"second config"}`},
		&model.AccountGroup{AccountID: account.Id, GroupID: group.ID},
		&model.GroupInbound{GroupID: group.ID, InboundID: first.Id},
		&model.GroupInbound{GroupID: group.ID, InboundID: second.Id},
		&model.AdminGroupAccess{UserID: admin.Id, GroupID: group.ID},
		&xray.ClientTraffic{InboundId: first.Id, Email: account.Email, Enable: true, Total: account.TotalGB, ExpiryTime: expiry, Up: 60},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (&AccountService{}).ProjectAccount(db, account.Id); err != nil {
		t.Fatal(err)
	}
	trash := TrashService{}
	if ids, err := trash.DeleteClient(&owner, account.Email); err != nil || len(ids) != 2 {
		t.Fatalf("delete account: %v %v", ids, err)
	}
	rows, err := trash.List(&admin, TrashClient, 0)
	if err != nil || len(rows) != 1 || rows[0].Name != account.Email || rows[0].Payload != "" {
		t.Fatalf("owner metadata: %+v %v", rows, err)
	}
	quota, days := int64(5<<30), int64(30)
	plan := &TrashClientPlan{QuotaBytes: &quota, ValidityDays: &days}
	for _, forbidden := range []*model.User{&owner, &other} {
		if rows, err := trash.List(forbidden, TrashClient, 0); err != nil || len(rows) > 0 {
			t.Fatalf("private client trash exposed: %+v %v", rows, err)
		}
		if _, err := trash.Restore(forbidden, TrashClient, rows[0].Id, plan); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("cross-owner restore: %v", err)
		}
		if err := trash.Purge(forbidden, TrashClient, rows[0].Id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := trash.Restore(&admin, TrashClient, rows[0].Id, nil); !errors.Is(err, ErrTrashClientPlan) {
		t.Fatalf("restore without plan: %v", err)
	}
	start := time.Now().UnixMilli()
	if ids, err := trash.Restore(&admin, TrashClient, rows[0].Id, plan); err != nil || len(ids) != 2 {
		t.Fatalf("restore account: %v %v", ids, err)
	}
	var restored model.Account
	if err := db.First(&restored, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	if restored.OwnerAdminID != admin.Id || restored.UUID != account.UUID || restored.Password != account.Password ||
		restored.ProtocolCredentials != account.ProtocolCredentials || restored.Comment != account.Comment || !restored.AutoRenew ||
		!restored.Enable || restored.TotalGB != quota || restored.RenewalDuration != days*bulkMsPerDay ||
		restored.ExpiryTime < start+restored.RenewalDuration || restored.ExpiryTime > time.Now().UnixMilli()+restored.RenewalDuration {
		t.Fatalf("restore preserved identity/config and assigned the selected allowance: %+v", restored)
	}
	var traffic xray.ClientTraffic
	if err := db.Where("email = ?", account.Email).First(&traffic).Error; err != nil ||
		traffic.Total != quota || traffic.ExpiryTime != restored.ExpiryTime || traffic.Up != 0 {
		t.Fatalf("restored traffic row: %+v %v", traffic, err)
	}
	var members []model.AccountInbound
	if err := db.Where("account_id = ?", account.Id).Order("inbound_id").Find(&members).Error; err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 || members[0].Extra != `{"custom":"config"}` || members[0].Up != 0 {
		t.Fatalf("restored memberships: %+v", members)
	}
	if rows, err := trash.List(&admin, TrashClient, 0); err != nil || len(rows) != 0 {
		t.Fatalf("restored row remained in trash: %+v %v", rows, err)
	}
}

func TestTrashClientRestoreHonorsRepresentativePlan(t *testing.T) {
	fx := newResellerFixture(t, "resellers-client")
	seedResellerProfile(t, fx)
	db := database.GetDB()
	if err := (&AccountService{}).SyncInboundAccounts(db, fx.inbound.Id, fx.admin.Id); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.ResellerProfile{}).Where("user_id = ?", fx.reseller.Id).
		Updates(map[string]any{"allowance_bytes": 3 * oneGB, "spent_bytes": oneGB, "days_per_gb": 3}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.ResellerClient{}).Where("user_id = ?", fx.reseller.Id).
		Update("charged_bytes", oneGB).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&xray.ClientTraffic{}).Where("email = ?", "resellers-client").
		Update("all_time", 256).Error; err != nil {
		t.Fatal(err)
	}
	trash := TrashService{}
	if _, err := trash.DeleteClient(fx.reseller, "resellers-client"); err != nil {
		t.Fatal(err)
	}
	rows, err := trash.List(fx.reseller, TrashClient, 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("representative trash: %v %v", rows, err)
	}
	wantDays, tooMuch := int64(10), int64(5*oneGB)
	plan := TrashClientPlan{QuotaBytes: &tooMuch, ValidityDays: &wantDays}
	if _, err := trash.Restore(fx.reseller, TrashClient, rows[0].Id, &plan); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("restore exceeding balance: %v", err)
	}
	var profile model.ResellerProfile
	if err := db.Where("user_id = ?", fx.reseller.Id).First(&profile).Error; err != nil || profile.SpentBytes != 256 {
		t.Fatalf("failed restore changed balance: %+v %v", profile, err)
	}
	newQuota := int64(2 * oneGB)
	plan.QuotaBytes = &newQuota
	if _, err := trash.Restore(fx.reseller, TrashClient, rows[0].Id, &plan); err != nil {
		t.Fatal(err)
	}
	var restored model.Account
	if err := db.Where("LOWER(email) = ?", "resellers-client").First(&restored).Error; err != nil ||
		restored.Enable || restored.TotalGB != newQuota || restored.RenewalDuration != 6*bulkMsPerDay {
		t.Fatalf("representative restore terms: %+v %v", restored, err)
	}
	if err := db.Where("user_id = ?", fx.reseller.Id).First(&profile).Error; err != nil || profile.SpentBytes != newQuota+256 {
		t.Fatalf("restore did not charge the representative: %+v %v", profile, err)
	}
	var ledger model.ResellerClient
	if err := db.Where("user_id = ?", fx.reseller.Id).First(&ledger).Error; err != nil || ledger.ChargedBytes != newQuota {
		t.Fatalf("restore did not record the charge: %+v %v", ledger, err)
	}
}

func TestTrashRepresentativeBundleSurvivesNativeBackup(t *testing.T) {
	fx := newResellerFixture(t, "resellers-client")
	seedResellerProfile(t, fx)
	db := database.GetDB()
	var owner model.User
	if err := db.Where("is_panel_owner = ?", true).First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := (&AccountService{}).SyncInboundAccounts(db, fx.inbound.Id, fx.admin.Id); err != nil {
		t.Fatal(err)
	}
	account, err := (&AccountService{}).GetAccountByEmailTx(db, "resellers-client")
	if err != nil || account == nil {
		t.Fatalf("seed account: %v", err)
	}
	quota := int64(1<<54 + 1)
	expiry := time.Now().AddDate(0, 0, 20).UnixMilli()
	if err := db.Model(account).Updates(map[string]any{"total_gb": quota, "expiry_time": expiry, "enable": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", fx.reseller.Id).
		Updates(map[string]any{"session_version": 7, "two_factor_token": "test-secret", "payment_reminders": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.ResellerProfile{}).Where("user_id = ?", fx.reseller.Id).
		Updates(map[string]any{"spent_bytes": 987, "usage_base": 234}).Error; err != nil {
		t.Fatal(err)
	}
	if err := (ActivityService{}).SetRenewalNotices(&owner, false); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.PanelPaymentReminder{AccountID: account.Id, Kind: "created", Revision: 5}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := (&ResellerService{}).DeleteReseller(&owner, fx.reseller.Id, DeleteModeCascade); err != nil {
		t.Fatal(err)
	}
	trash := TrashService{}
	rows, err := trash.List(&owner, TrashManager, 0)
	if err != nil || len(rows) != 1 || rows[0].ClientCount != 1 {
		t.Fatalf("manager archive: %+v %v", rows, err)
	}
	if visible, err := trash.List(fx.admin, TrashManager, 0); err != nil || len(visible) != 0 {
		t.Fatalf("non-owner manager scope: %+v %v", visible, err)
	}
	data, err := (&ServerService{}).GetDb()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "restored.db")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := database.ValidateSQLiteDB(path); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(path); err != nil {
		t.Fatal(err)
	}
	db = database.GetDB()
	if err := db.First(&owner, owner.Id).Error; err != nil || !owner.RenewalNoticesDisabled {
		t.Fatalf("renewal notification preference from backup: %v %+v", err, owner)
	}
	conflict := model.Account{Email: account.Email, Enable: false}
	if err := db.Create(&conflict).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := trash.Restore(&owner, TrashManager, rows[0].Id, nil); !errors.Is(err, ErrTrashConflict) {
		t.Fatalf("restore should refuse an existing client identity: %v", err)
	}
	if resellerExists(t, fx.reseller.Id) {
		t.Fatal("failed restore partially recreated the representative")
	}
	if err := db.Delete(&conflict).Error; err != nil {
		t.Fatal(err)
	}
	if ids, err := trash.Restore(&owner, TrashManager, rows[0].Id, nil); err != nil || len(ids) != 1 {
		t.Fatalf("bundle restore: %v %v", ids, err)
	}
	var user model.User
	if err := db.First(&user, fx.reseller.Id).Error; err != nil || user.SessionVersion != 8 || user.TwoFactorToken != "test-secret" || !user.PaymentReminders {
		t.Fatalf("manager settings or session revocation: %+v %v", user, err)
	}
	var profile model.ResellerProfile
	if err := db.Where("user_id = ?", user.Id).First(&profile).Error; err != nil || profile.SpentBytes != 987 || profile.UsageBase != 234 {
		t.Fatalf("restored profile: %+v %v", profile, err)
	}
	var restored model.Account
	if err := db.First(&restored, account.Id).Error; err != nil || restored.TotalGB != quota || restored.ExpiryTime != expiry || !restored.Enable {
		t.Fatalf("restored allowance: %+v %v", restored, err)
	}
	var payment model.PanelPaymentReminder
	if err := db.First(&payment, "account_id = ?", account.Id).Error; err != nil || payment.Revision != 5 {
		t.Fatalf("restored pending payment: %+v %v", payment, err)
	}
}
