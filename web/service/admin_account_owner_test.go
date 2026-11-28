package service

import (
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"testing"
)

func TestAdminDeletionRequiresTransferringOwnedClients(t *testing.T) {
	svc := newAccountsDB(t)
	db := database.GetDB()
	owner := model.User{Username: "owner-admin", Enable: true, IsSuperAdmin: true, IsPanelOwner: true}
	other := model.User{Username: "other-admin", Enable: true, IsSuperAdmin: true}
	for _, u := range []*model.User{&owner, &other} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	account, err := svc.SaveAccountWithoutInbound(map[string]any{"email": "owned-client", "enable": false}, other.Id)
	if err != nil {
		t.Fatal(err)
	}
	if err := (&AdminService{}).DeleteAdmin(other.Id); err == nil {
		t.Fatal("deleted an admin with owned clients")
	}
	if err := svc.TransferOwnership(&owner, account.Id, owner.Id); err != nil {
		t.Fatal(err)
	}
	if err := (&AdminService{}).DeleteAdmin(other.Id); err != nil {
		t.Fatalf("delete after transfer: %v", err)
	}
}

func TestLegacyUnattributedClientCanTransferToSecondSuperAdmin(t *testing.T) {
	svc := newAccountsDB(t)
	db := database.GetDB()
	if err := db.Model(&model.User{}).Where("is_panel_owner = ?", true).Update("is_panel_owner", false).Error; err != nil {
		t.Fatal(err)
	}
	owner := model.User{Username: "main-admin", Enable: true, IsSuperAdmin: true, IsPanelOwner: true}
	second := model.User{Username: "legacy-super", Enable: true, IsSuperAdmin: true}
	for _, u := range []*model.User{&owner, &second} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	a, err := svc.SaveAccountWithoutInbound(map[string]any{"email": "old-unknown", "enable": false, "subId": "old-sub"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := svc.ListAccounts(&owner, 1, 50, "old-unknown", "")
	if err != nil || len(rows.Rows) != 1 || rows.Rows[0].OwnerID != owner.Id || rows.Rows[0].OwnerUsername != "" {
		t.Fatalf("legacy fallback: %+v %v", rows, err)
	}
	if err := svc.TransferOwnership(&owner, a.Id, second.Id); err != nil {
		t.Fatal(err)
	}
	rows, err = svc.ListAccounts(&owner, 1, 50, "old-unknown", "")
	if err != nil || len(rows.Rows) != 1 || rows.Rows[0].OwnerID != second.Id {
		t.Fatalf("new owner: %+v %v", rows, err)
	}
	a, err = svc.GetAccountByEmail("old-unknown")
	if err != nil || a.Enable || a.SubID != "old-sub" {
		t.Fatalf("client changed: %+v %v", a, err)
	}
}

func TestUnattributedClientCanBeAssignedToOrdinaryAdmin(t *testing.T) {
	svc := newAccountsDB(t)
	db := database.GetDB()
	var owner model.User
	if err := db.Where("is_panel_owner").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	admin := model.User{Username: "original-admin", Enable: true, Permissions: model.PermAccessInbounds}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	inbound := seedInboundWithClients(t, model.VLESS, 43350, []map[string]any{})
	account, err := svc.SaveAccountWithoutInbound(map[string]any{"email": "unknown-owner", "enable": true}, owner.Id)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(account).Update("owner_confirmed", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyMembershipsFrom(account.Email, 0, []int{inbound.Id}, nil, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.TransferOwnership(&owner, account.Id, admin.Id); err == nil {
		t.Fatal("assigned an operator without access to the account")
	}
	if err := db.Create(&model.InboundAccess{UserId: admin.Id, InboundId: inbound.Id}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.TransferOwnership(&owner, account.Id, admin.Id); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.ListAccounts(&owner, 1, 50, account.Email, "")
	if err != nil || len(rows.Rows) != 1 || rows.Rows[0].OwnerID != admin.Id || rows.Rows[0].OwnerUsername != admin.Username {
		t.Fatalf("corrected owner in client list: %+v %v", rows, err)
	}
	var mismatched int64
	if err := db.Model(&model.PanelClientEvent{}).Where("account_id=? AND owner_id<>?", account.Id, admin.Id).
		Count(&mismatched).Error; err != nil || mismatched != 0 {
		t.Fatalf("legacy activity still attributed to the default owner: %d %v", mismatched, err)
	}
}
