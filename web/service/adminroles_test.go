package service

import (
	"errors"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"testing"
)

func TestAdminRolesDynamicPermissionsAssignmentAndDeletion(t *testing.T) {
	s := newAdminDB(t)
	db := database.GetDB()
	var owner model.User
	db.Where("is_panel_owner = ?", true).First(&owner)
	if err := s.SaveAdminRole(&owner, 0, "Node operator", model.PermManageNodes); err != nil {
		t.Fatal(err)
	}
	var role model.AdminRole
	db.First(&role)
	u, err := s.AddAdmin(AdminSpec{Username: "operator", Password: "role-test-password", Enable: true, AdminRoleID: &role.ID, Permissions: model.PermPanelSettings})
	if err != nil {
		t.Fatal(err)
	}
	u.AdminRole = &role
	if !u.Can(model.PermManageNodes) || u.Can(model.PermPanelSettings) || u.CanManageAdmins() {
		t.Fatal("role inheritance or privilege boundaries failed")
	}
	if err := s.DeleteAdminRole(&owner, role.ID); !errors.Is(err, ErrAdminRoleAssigned) {
		t.Fatal(err)
	}
	if err := s.SaveAdminRole(&owner, role.ID, role.Name, model.PermAccessInbounds); err != nil {
		t.Fatal(err)
	}
	db.First(&role, role.ID)
	u.AdminRole = &role
	if u.Can(model.PermManageNodes) || !u.Can(model.PermAccessInbounds) {
		t.Fatal("permission changes did not revoke node access")
	}
	u.AdminRole = nil
	if u.Can(model.PermManageNodes) || u.Can(model.PermPanelSettings) {
		t.Fatal("missing role falls back to stale permissions")
	}
	zero := 0
	if err := s.UpdateAdmin(u.Id, AdminSpec{Username: u.Username, Enable: true, Permissions: model.PermAccessInbounds, AdminRoleID: &zero}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAdminRole(&owner, role.ID); err != nil {
		t.Fatal(err)
	}
}
func TestAdminOwnerCanDemoteWithoutLosingOwnershipOrManagement(t *testing.T) {
	s := newAdminDB(t)
	db := database.GetDB()
	var owner model.User
	db.Where("is_panel_owner = ?", true).First(&owner)
	if err := s.UpdateAdmin(owner.Id, AdminSpec{Username: owner.Username, Enable: true, Permissions: model.PermManageNodes, IsSuperAdmin: false}); err != nil {
		t.Fatal(err)
	}
	db.First(&owner, owner.Id)
	if !owner.IsPanelOwner || owner.IsSuperAdmin || !owner.CanManageAdmins() || !owner.Can(model.PermManageNodes) || owner.Can(model.PermPanelSettings) {
		t.Fatalf("owner demotion invalid: %+v", owner)
	}
	database.MigrateSuperAdminForTest()
	db.First(&owner, owner.Id)
	if owner.IsSuperAdmin {
		t.Fatal("restart migration silently promoted the owner again")
	}
	if err := s.UpdateAdmin(owner.Id, AdminSpec{Username: owner.Username, Enable: false}); !errors.Is(err, ErrProtectedAdmin) {
		t.Fatal("owner can be disabled", err)
	}
	if err := s.DeleteAdmin(owner.Id); !errors.Is(err, ErrProtectedAdmin) {
		t.Fatal("owner can be deleted", err)
	}
	other, err := s.AddAdmin(AdminSpec{Username: "other-root", Password: "test-password", Enable: true, IsSuperAdmin: true})
	if err != nil {
		t.Fatal(err)
	}
	if !other.IsSuperAdmin || other.IsPanelOwner {
		t.Fatal("super-admin creation changed ownership")
	}
	if err := s.GuardAdminTarget(other, owner.Id); !errors.Is(err, ErrProtectedAdmin) {
		t.Fatal("other super admin can edit owner", err)
	}
	if err := s.SaveAdminRole(&owner, 0, "Owner role", model.PermAccessInbounds); err != nil {
		t.Fatal(err)
	}
	var role model.AdminRole
	db.First(&role)
	if err := s.UpdateAdmin(owner.Id, AdminSpec{Username: owner.Username, Enable: true, AdminRoleID: &role.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAdminRole(other, role.ID, role.Name, 0); !errors.Is(err, ErrProtectedAdmin) {
		t.Fatal("owner role changed indirectly", err)
	}
	if err := s.DeleteAdminRole(other, role.ID); !errors.Is(err, ErrProtectedAdmin) {
		t.Fatal("owner role deleted indirectly", err)
	}
}
