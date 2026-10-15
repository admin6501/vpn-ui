package service

import (
	"reflect"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestGroupsAdministratorUpgradePreservesScopeAndIndependentPermissions(t *testing.T) {
	accounts := newAccountsDB(t)
	db := database.GetDB()
	a := seedInboundWithClients(t, model.VLESS, 45101, []map[string]any{{"email": "existing-group-account", "id": "11111111-1111-4111-8111-111111111111", "enable": true}})
	b := seedInboundWithClients(t, model.Trojan, 45102, []map[string]any{})
	accounts.MigrationAccounts()
	gs := &GroupService{}
	// Simulate the previous release, including a manually expanded group.
	if err := gs.migrateV1(); err != nil {
		t.Fatal(err)
	}
	if err := gs.migrateImportedGroupsAndGrants(); err != nil {
		t.Fatal(err)
	}
	groups := mustGroups(t)
	if _, _, err := gs.Save(groupRoot(t), groups[0].ID, "Previously expanded", []int{a.Id, b.Id}); err != nil {
		t.Fatal(err)
	}
	role := model.AdminRole{Name: "Legacy inbound editor", Permissions: model.PermEditInbound | model.PermAccessInbounds}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	admin := model.User{Username: "legacy-group-admin", Enable: true, AdminRoleID: role.ID}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.InboundAccess{UserId: admin.Id, InboundId: a.Id}).Error; err != nil {
		t.Fatal(err)
	}
	before := snapshotUntouchedTables(t)
	beforeAccounts := accountsInDB(t)
	beforeMembers := membershipsInDB(t)
	if err := db.Exec(`CREATE TRIGGER reject_admin_group BEFORE INSERT ON admin_group_accesses BEGIN SELECT RAISE(ABORT,'migration failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := gs.Migrate(); err == nil {
		t.Fatal("expected failed migration")
	}
	if AdminGroupsMigrated(db) {
		t.Fatal("failed migration marker committed")
	}
	db.First(&role, role.ID)
	if role.Permissions.Has(model.PermAccessGroups) {
		t.Fatal("partial permission migration committed")
	}
	db.Exec(`DROP TRIGGER reject_admin_group`)
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	assertAdditiveOnly(t, before)
	if !reflect.DeepEqual(beforeAccounts, accountsInDB(t)) || !reflect.DeepEqual(beforeMembers, membershipsInDB(t)) {
		t.Fatal("administrator migration rewrote customers")
	}
	db.First(&admin, admin.Id)
	db.First(&role, role.ID)
	admin.AdminRole = &role
	for _, perm := range []model.Permission{model.PermAccessGroups, model.PermCreateGroup, model.PermEditGroup, model.PermDeleteGroup} {
		if !admin.Can(perm) {
			t.Fatal("old group capability lost", perm)
		}
	}
	access := &AdminService{}
	ids, err := access.AccessibleInboundIds(admin.Id)
	if err != nil || !reflect.DeepEqual(ids, []int{a.Id}) {
		t.Fatal("migration widened or lost inbound scope", ids, err)
	}
	assigned, err := adminGroupIDs(db, admin.Id)
	if err != nil || len(assigned) != 1 {
		t.Fatal(assigned, err)
	}
	if assigned[0] == groups[0].ID {
		t.Fatal("partial old grant was widened to the expanded group")
	}
	if _, _, err := gs.Save(groupRoot(t), assigned[0], "Admin access", []int{a.Id, b.Id}); err != nil {
		t.Fatal(err)
	}
	if ok, err := access.CanAccessAllInbounds([]int{a.Id, b.Id}, admin.Id); err != nil || !ok {
		t.Fatal("group edit did not update admin scope", ok, err)
	}
	if _, _, err := gs.Save(groupRoot(t), assigned[0], "Admin access", []int{a.Id}); err != nil {
		t.Fatal(err)
	}
	// Even a stale legacy grant cannot restore removed access.
	if err := db.Create(&model.InboundAccess{UserId: admin.Id, InboundId: b.Id}).Error; err != nil {
		t.Fatal(err)
	}
	if ok, _ := access.CanAccessInbound(b.Id, admin.Id); ok {
		t.Fatal("stale direct grant bypassed group scope")
	}
	if err := db.Model(&role).Update("permissions", model.PermEditInbound).Error; err != nil {
		t.Fatal(err)
	}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	db.First(&role, role.ID)
	admin.AdminRole = &role
	if admin.Can(model.PermAccessGroups) {
		t.Fatal("restart restored explicitly revoked group permissions")
	}
	if _, _, err := gs.Save(&admin, assigned[0], "denied", []int{a.Id}); err == nil {
		t.Fatal("inbound edit permission still grants group editing")
	}
}

func TestGroupsAdministratorCreatePendingInboundAndExplicitRevocation(t *testing.T) {
	accounts := newAccountsDB(t)
	accounts.MigrationAccounts()
	gs := &GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	svc := &AdminService{}
	empty := []int{}
	admin, err := svc.AddAdmin(AdminSpec{Username: "group-infra-admin", Password: "test-password", Enable: true, Permissions: groupManagementPermissions | model.PermAccessInbounds | model.PermCreateInbound, GroupIDs: &empty})
	if err != nil {
		t.Fatal(err)
	}
	ib := seedInboundWithClients(t, model.VLESS, 45201, []map[string]any{})
	if err := db.Model(ib).Update("user_id", admin.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.GrantInbound(admin.Id, ib.Id); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.CanAccessInbound(ib.Id, admin.Id); !ok {
		t.Fatal("creator cannot configure their ungrouped inbound")
	}
	var n int64
	db.Model(&model.GroupInbound{}).Where("inbound_id = ?", ib.Id).Count(&n)
	if n != 0 {
		t.Fatal("new inbound was grouped automatically")
	}
	g, _, err := gs.Save(admin, 0, "Manually assigned", []int{ib.Id})
	if err != nil {
		t.Fatal(err)
	}
	selected, _ := adminGroupIDs(db, admin.Id)
	if !reflect.DeepEqual(selected, []int{g.ID}) {
		t.Fatal("creator lost the new group", selected)
	}
	db.Model(&model.InboundAccess{}).Where("user_id = ? AND pending_group = ?", admin.Id, true).Count(&n)
	if n != 0 {
		t.Fatal("temporary grant survived its first group assignment")
	}
	if _, _, err := gs.Save(admin, g.ID, g.Name, []int{ib.Id}, true); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.CanAccessInbound(ib.Id, admin.Id); !ok {
		t.Fatal("administrator lost infrastructure control of disabled group")
	}
	if err := svc.UpdateAdmin(admin.Id, AdminSpec{Username: admin.Username, Enable: true, Permissions: admin.Permissions, GroupIDs: &empty}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.CanAccessInbound(ib.Id, admin.Id); ok {
		t.Fatal("explicit empty retained group access")
	}
	if _, _, err := gs.Save(groupRoot(t), g.ID, g.Name, nil); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.CanAccessInbound(ib.Id, admin.Id); ok {
		t.Fatal("removing group membership resurrected creator access")
	}
}

func TestGroupsAdministratorGrantFailureRollsBackCredentialsAndPermissions(t *testing.T) {
	accounts := newAccountsDB(t)
	ib := seedInboundWithClients(t, model.VLESS, 45301, []map[string]any{})
	accounts.MigrationAccounts()
	gs := &GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	groups := mustGroups(t)
	ids := []int{groups[0].ID}
	svc := &AdminService{}
	admin, err := svc.AddAdmin(AdminSpec{Username: "atomic-group-admin", Password: "initial-password", Enable: true, Permissions: model.PermAccessInbounds, GroupIDs: &ids})
	if err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	if err := db.Exec(`CREATE TRIGGER reject_admin_grant BEFORE INSERT ON admin_group_accesses BEGIN SELECT RAISE(ABORT,'grant failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateAdmin(admin.Id, AdminSpec{Username: "changed-name", Password: "changed-password", Enable: true, Permissions: model.PermPanelSettings, GroupIDs: &ids}); err == nil {
		t.Fatal("expected failure")
	}
	var stored model.User
	db.First(&stored, admin.Id)
	if stored.Username != admin.Username || stored.Password != admin.Password || stored.Permissions != admin.Permissions {
		t.Fatal("failed grant partially saved administrator")
	}
	if ok, _ := svc.CanAccessInbound(ib.Id, admin.Id); !ok {
		t.Fatal("failed grant lost previous access")
	}
	if _, err := svc.AddAdmin(AdminSpec{Username: "must-not-exist", Password: "password", Enable: true, GroupIDs: &ids}); err == nil {
		t.Fatal("expected create grant failure")
	}
	var n int64
	db.Model(&model.User{}).Where("username = ?", "must-not-exist").Count(&n)
	if n != 0 {
		t.Fatal("failed grant left a live administrator")
	}
	db.Exec(`DROP TRIGGER reject_admin_grant`)
	forged := []int{999999}
	if err := svc.UpdateAdmin(admin.Id, AdminSpec{Username: admin.Username, Enable: true, GroupIDs: &forged}); err == nil {
		t.Fatal("unknown group accepted")
	}
	if err := svc.UpdateAdmin(admin.Id, AdminSpec{Username: admin.Username, Enable: true, InboundIds: []int{ib.Id}}); err == nil {
		t.Fatal("individual inbound assignment still accepted")
	}
	if err := svc.UpdateAdmin(admin.Id, AdminSpec{Username: admin.Username, Enable: true, Nickname: "metadata"}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.CanAccessInbound(ib.Id, admin.Id); !ok {
		t.Fatal("omitted groups revoked historical access")
	}
}
