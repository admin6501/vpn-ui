package service

import (
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"testing"
)

func TestAccountOwnershipTransferPreservesUsageCredentialsAndVisibility(t *testing.T) {
	svc := newAccountsDB(t)
	db := database.GetDB()
	role := model.RepresentativeRole{Name: "operator", Permissions: `{}`, Limits: `{}`, Access: `{}`}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	root := model.User{Username: "panel-owner", Enable: true, IsSuperAdmin: true}
	first := model.User{Username: "seller-one", Enable: true, IsReseller: true, RepresentativeRoleID: role.ID}
	second := model.User{Username: "seller-two", Enable: true, IsReseller: true, RepresentativeRoleID: role.ID}
	for _, u := range []*model.User{&root, &first, &second} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, u := range []*model.User{&first, &second} {
		p := model.ResellerProfile{UserId: u.Id, ConsumptionBased: true, Unlimited: true}
		if u.Id == first.Id {
			p.UsageBase = gb
		}
		if err := db.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
	}
	in, _ := seedTwoInboundAccount(t, svc, "transfer-customer")
	a, err := svc.GetAccountByEmail("transfer-customer")
	if err != nil {
		t.Fatal(err)
	}
	a.SubID = "unchanged-token"
	if err = db.Save(a).Error; err != nil {
		t.Fatal(err)
	}
	var ct xray.ClientTraffic
	db.Where("email = ?", a.Email).First(&ct)
	ct.AllTime = 6 * gb
	ct.Down = gb
	db.Save(&ct)
	old := model.ResellerClient{Email: a.Email, UserId: first.Id, InboundId: in.Id, AllTimeBase: 2 * gb}
	db.Create(&old)
	if err = svc.TransferOwnership(&first, a.Id, second.Id); err == nil {
		t.Fatal("representative transferred an account")
	}
	if err = svc.TransferOwnership(&root, a.Id, second.Id); err != nil {
		t.Fatal(err)
	}
	p1, err := (&ResellerService{}).ProfileFor(first.Id)
	if err != nil || p1.SpentBytes != 5*gb {
		t.Fatalf("old consumption lost: %+v %v", p1, err)
	}
	p2, err := (&ResellerService{}).ProfileFor(second.Id)
	if err != nil || p2.SpentBytes != 0 {
		t.Fatalf("old consumption charged to new owner: %+v %v", p2, err)
	}
	before, err := svc.visibilityFilter(&first)
	if err != nil {
		t.Fatal(err)
	}
	after, err := svc.visibilityFilter(&second)
	if err != nil {
		t.Fatal(err)
	}
	if before(a, nil) || !after(a, nil) {
		t.Fatal("ownership visibility did not move")
	}
	reloaded, _ := svc.GetAccountByEmail(a.Email)
	if reloaded.SubID != a.SubID || reloaded.UUID != a.UUID || reloaded.Password != a.Password {
		t.Fatal("transfer changed credentials")
	}
	rows, err := svc.ListAccounts(&root, 1, 50, "transfer-customer", "")
	if err != nil || len(rows.Rows) != 1 || rows.Rows[0].OwnerUsername != second.Username || !rows.CanTransferOwnership {
		t.Fatalf("owner label missing: %+v %v", rows, err)
	}
	ct.AllTime = 7 * gb
	db.Save(&ct)
	if err := db.Model(&role).Update("limits", `{"max_users":0}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.TransferOwnership(&root, a.Id, first.Id); err == nil {
		t.Fatal("target user limit bypassed")
	}
	var retained model.ResellerClient
	if err := db.Where("email = ?", a.Email).First(&retained).Error; err != nil || retained.UserId != second.Id {
		t.Fatal("failed transfer lost ownership")
	}
	if err := db.Model(&role).Update("limits", `{}`).Error; err != nil {
		t.Fatal(err)
	}
	if err = svc.TransferOwnership(&root, a.Id, root.Id); err != nil {
		t.Fatal(err)
	}
	p2, _ = (&ResellerService{}).ProfileFor(second.Id)
	if p2.SpentBytes != gb {
		t.Fatal("transfer back refunded consumption")
	}
}
