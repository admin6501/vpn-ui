package service

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestGroupsImportedNamesAndRepresentativeGrantsUpgradeV1WithoutChangingCustomers(t *testing.T) {
	accounts := newAccountsDB(t)
	a := seedInboundWithClients(t, model.VLESS, 44701, []map[string]any{{"email": "only-a", "id": "11111111-1111-4111-8111-111111111111", "enable": true}})
	b := seedInboundWithClients(t, model.Trojan, 44702, []map[string]any{{"email": "only-b", "password": "unchanged", "enable": true}})
	accounts.MigrationAccounts()
	db := database.GetDB()
	root := groupRoot(t)
	role := model.RepresentativeRole{Name: "Original role", Access: `{"allowed_group_ids":[17],"allowed_template_ids":[9]}`}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	rep := model.User{Username: "imported-groups", IsReseller: true, RepresentativeRoleID: role.ID, Enable: true}
	if err := db.Create(&rep).Error; err != nil {
		t.Fatal(err)
	}
	for _, ib := range []int{a.Id, b.Id} {
		if err := db.Create(&model.InboundAccess{UserId: rep.Id, InboundId: ib}).Error; err != nil {
			t.Fatal(err)
		}
	}
	rawRole, _ := json.Marshal(map[string]string{"id": fmt.Sprint(role.ID), "access": role.Access})
	rows := []model.PasarGuardResource{
		{Kind: "groups", SourceID: 17, Data: `{"id":"17","name":"گروه اصلی پاسارگاد","is_disabled":"f"}`},
		{Kind: "groups", SourceID: 18, Data: `{"id":"18","name":"Disabled original","is_disabled":"t"}`},
		{Kind: "inbounds", SourceID: 21, Data: fmt.Sprintf(`{"id":"21","tag":%q}`, a.Tag)},
		{Kind: "inbounds", SourceID: 22, Data: fmt.Sprintf(`{"id":"22","tag":%q}`, b.Tag)},
		{Kind: "inbounds_groups_association", SourceID: 1, Data: `{"group_id":"17","inbound_id":"21"}`},
		{Kind: "inbounds_groups_association", SourceID: 2, Data: `{"group_id":"17","inbound_id":"22"}`},
		{Kind: "inbounds_groups_association", SourceID: 3, Data: `{"group_id":"18","inbound_id":"22"}`},
		{Kind: "admin_roles", SourceID: role.ID, Data: string(rawRole)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	gs := &GroupService{}
	// Simulate a panel which has already installed the previous release.
	if err := gs.migrateV1(); err != nil {
		t.Fatal(err)
	}
	// A role tightened after V1 must not regain its original source allowlist.
	restricted := model.RepresentativeRole{Name: "Manually restricted", Access: `{"allowed_group_ids":[]}`}
	if err := db.Create(&restricted).Error; err != nil {
		t.Fatal(err)
	}
	rawRestricted, _ := json.Marshal(map[string]string{"access": `{"allowed_group_ids":[17]}`})
	if err := db.Create(&model.PasarGuardResource{Kind: "admin_roles", SourceID: restricted.ID, Data: string(rawRestricted)}).Error; err != nil {
		t.Fatal(err)
	}
	blocked := model.User{Username: "restricted-after-v1", IsReseller: true, RepresentativeRoleID: restricted.ID, Enable: true}
	if err := db.Create(&blocked).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{a.Id, b.Id} {
		if err := db.Create(&model.InboundAccess{UserId: blocked.Id, InboundId: id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	beforeAccounts := accountsInDB(t)
	beforeMemberships := membershipsInDB(t)
	beforeGroups := map[int][]int{}
	for _, acc := range beforeAccounts {
		beforeGroups[acc.Id] = groupIDsFor(t, acc.Id)
	}
	if err := db.Exec(`CREATE TRIGGER reject_group_grants BEFORE INSERT ON representative_group_accesses BEGIN SELECT RAISE(ABORT,'migration failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := gs.Migrate(); err == nil {
		t.Fatal("expected migration failure")
	}
	if len(mustGroups(t)) != 2 {
		t.Fatal("partial original groups committed")
	}
	if err := db.First(&rep, rep.Id).Error; err != nil {
		t.Fatal(err)
	}
	if rep.GroupAccess {
		t.Fatal("partial access migration committed")
	}
	db.Exec(`DROP TRIGGER reject_group_grants`)
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeAccounts, accountsInDB(t)) || !reflect.DeepEqual(beforeMemberships, membershipsInDB(t)) {
		t.Fatal("customer data changed")
	}
	for id, ids := range beforeGroups {
		if !reflect.DeepEqual(ids, groupIDsFor(t, id)) {
			t.Fatal("existing customer's groups changed")
		}
	}
	groups := mustGroups(t)
	if len(groups) != 4 {
		t.Fatal(groups)
	}
	var original, disabled GroupView
	for _, g := range groups {
		if g.PasarGuardID != nil {
			switch *g.PasarGuardID {
			case 17:
				original = g
			case 18:
				disabled = g
			}
		}
	}
	if original.Name != "گروه اصلی پاسارگاد" || !reflect.DeepEqual(original.InboundIDs, []int{a.Id, b.Id}) || !disabled.Disabled {
		t.Fatal("source group definition lost", groups)
	}
	if err := db.First(&rep, rep.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !rep.GroupAccess {
		t.Fatal("new grant authority not enabled")
	}
	ids, err := representativeGroupIDs(db, rep.Id)
	if err != nil || !reflect.DeepEqual(ids, []int{original.ID}) {
		t.Fatal("original group not restored to representative", ids, err)
	}
	if err := db.First(&blocked, blocked.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&restricted, restricted.ID).Error; err != nil {
		t.Fatal(err)
	}
	if restricted.Access != `{"allowed_group_ids":[]}` {
		t.Fatal("manual role restriction overwritten", restricted.Access)
	}
	if choices, err := gs.List(&blocked, false); err != nil || len(choices) != 0 {
		t.Fatal("manual role restriction ignored", choices, err)
	}
	access := &AdminService{}
	if ok, err := access.CanAccessAllInbounds([]int{a.Id, b.Id}, rep.Id); err != nil || !ok {
		t.Fatal("upgrade changed scope", ok, err)
	}
	if err := db.First(&role, role.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := enforceRoleGroupAccess(&role, []int{original.ID}); err != nil {
		t.Fatal("original role group not translated", err)
	}
	if _, _, err := gs.Save(root, original.ID, original.Name, []int{a.Id}); err != nil {
		t.Fatal(err)
	}
	if ok, err := access.CanAccessInbound(b.Id, rep.Id); err != nil || ok {
		t.Fatal("stale individual inbound grant still authorizes access", ok, err)
	}
	if ok, err := access.CanAccessClientEmail("only-b", rep.Id); err != nil || ok {
		t.Fatal("email scope uses stale grant", ok, err)
	}
	broadcast, err := access.ClientEmailAccess()
	if err != nil || broadcast["only-b"][rep.Id] {
		t.Fatal("broadcast scope uses stale grant", broadcast, err)
	}
	if _, err := gs.Resolve(&rep, []int{groups[0].ID}); err == nil {
		t.Fatal("shared inbound bypassed explicit group grant")
	}
	if !reflect.DeepEqual(beforeMemberships, membershipsInDB(t)) {
		t.Fatal("editing representative group rewrote unrelated customers")
	}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	if len(mustGroups(t)) != 4 {
		t.Fatal("restart duplicated groups")
	}
	if ok, _ := access.CanAccessInbound(b.Id, rep.Id); ok {
		t.Fatal("restart restored revoked inbound")
	}
}

func TestGroupsRepresentativeGrantValidationAndDynamicAccess(t *testing.T) {
	accounts := newAccountsDB(t)
	a := seedInboundWithClients(t, model.VLESS, 44801, []map[string]any{})
	b := seedInboundWithClients(t, model.Trojan, 44802, []map[string]any{})
	accounts.MigrationAccounts()
	gs := &GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	root := groupRoot(t)
	groups := mustGroups(t)
	db := database.GetDB()
	role := model.RepresentativeRole{Name: "Group seller", Access: fmt.Sprintf(`{"allowed_group_ids":[%d]}`, groups[0].ID)}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	service := &ResellerService{}
	selected := []int{groups[0].ID}
	spec := ResellerSpec{Username: "group-only-seller", Password: "test-password", Enable: true, ConsumptionBased: true, RepresentativeRoleID: role.ID, Unlimited: true, GroupIDs: &selected}
	rep, err := service.AddReseller(root, spec)
	if err != nil {
		t.Fatal(err)
	}
	access := &AdminService{}
	if ok, err := access.CanAccessInbound(a.Id, rep.Id); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, _ := access.CanAccessInbound(b.Id, rep.Id); ok {
		t.Fatal("unassigned group visible")
	}
	// A group expanding grants the representative access without an inbound save.
	if _, _, err := gs.Save(root, groups[0].ID, "Expanded", []int{a.Id, b.Id}); err != nil {
		t.Fatal(err)
	}
	if ok, err := access.CanAccessAllInbounds([]int{a.Id, b.Id, b.Id}, rep.Id); err != nil || !ok {
		t.Fatal("dynamic group not authoritative", ok, err)
	}
	spec.Password = ""
	spec.Nickname = "saved"
	spec.GroupIDs = nil
	if err := service.UpdateReseller(root, rep.Id, spec); err != nil {
		t.Fatal("metadata save dropped groups", err)
	}
	invalid := []int{groups[1].ID}
	spec.GroupIDs = &invalid
	if err := service.UpdateReseller(root, rep.Id, spec); err == nil {
		t.Fatal("role group allowlist bypassed with a shared inbound")
	}
	if err := gs.Delete(root, groups[0].ID); err == nil {
		t.Fatal("assigned representative group deleted")
	}
	// Disabling groups closes inbound, email and list scope, while retaining grants.
	if _, _, err := gs.Save(root, groups[0].ID, "Expanded", []int{a.Id, b.Id}, true); err != nil {
		t.Fatal(err)
	}
	if ok, _ := access.CanAccessInbound(a.Id, rep.Id); ok {
		t.Fatal("disabled group still authorizes")
	}
	if choices, err := gs.List(rep, false); err != nil || len(choices) != 0 {
		t.Fatal(choices, err)
	}
	// Explicit empty differs from omitted: it revokes every group permanently.
	empty := []int{}
	spec.GroupIDs = &empty
	if err := service.UpdateReseller(root, rep.Id, spec); err != nil {
		t.Fatal(err)
	}
	if _, _, err := gs.Save(root, groups[0].ID, "Expanded", []int{a.Id, b.Id}, false); err != nil {
		t.Fatal(err)
	}
	if ok, _ := access.CanAccessInbound(a.Id, rep.Id); ok {
		t.Fatal("empty assignment was treated as omitted")
	}
}

func TestGroupsUpgradeAddsEnabledDefaultToOldGroupRows(t *testing.T) {
	accounts := newAccountsDB(t)
	a := seedInboundWithClients(t, model.VLESS, 44901, []map[string]any{})
	accounts.MigrationAccounts()
	gs := &GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	// The previous release's client_groups table had neither this column nor a
	// default. Check the actual SQLite ALTER path, not just newly inserted rows.
	if err := db.Migrator().DropColumn(&model.ClientGroup{}, "Disabled"); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.ClientGroup{}); err != nil {
		t.Fatal(err)
	}
	groups := mustGroups(t)
	if len(groups) != 1 || groups[0].Disabled {
		t.Fatal(groups)
	}
	var n int64
	if err := db.Model(&model.ClientGroup{}).Where("disabled = 0").Count(&n).Error; err != nil || n != 1 {
		t.Fatal("old row does not have enabled SQL default", n, err)
	}
	if ids, err := gs.Resolve(groupRoot(t), []int{groups[0].ID}); err != nil || !reflect.DeepEqual(ids, []int{a.Id}) {
		t.Fatal(ids, err)
	}
}

func TestGroupsOwnershipTransferUsesGroupGrants(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	root := groupRoot(t)
	role := model.RepresentativeRole{Name: "Transfer role", Permissions: `{}`, Limits: `{}`, Access: `{}`}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	rep := model.User{Username: "transfer-group-seller", Enable: true, IsReseller: true, RepresentativeRoleID: role.ID}
	if err := db.Create(&rep).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ResellerProfile{UserId: rep.Id, ConsumptionBased: true, Unlimited: true}).Error; err != nil {
		t.Fatal(err)
	}
	first, _ := seedTwoInboundAccount(t, accounts, "group-transfer")
	accounts.MigrationAccounts()
	gs := &GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	account, err := accounts.GetAccountByEmail("group-transfer")
	if err != nil {
		t.Fatal(err)
	}
	if err := accounts.TransferOwnership(root, account.Id, rep.Id); err != nil {
		t.Fatal(err)
	}
	assigned, err := representativeGroupIDs(db, rep.Id)
	if err != nil || !reflect.DeepEqual(assigned, groupIDsFor(t, account.Id)) {
		t.Fatal("transfer did not grant customer groups", assigned, err)
	}
	access := &AdminService{}
	if ok, err := access.CanAccessInbound(first.Id, rep.Id); err != nil || !ok {
		t.Fatal("transferred account is inaccessible", ok, err)
	}
}
