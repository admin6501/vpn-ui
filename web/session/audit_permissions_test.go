package session

import (
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"path/filepath"
	"testing"
)

func TestSecurityAuditCapturedSessionUsesLiveAdministratorRole(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "role.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	role := model.AdminRole{Name: "overview-audit", Permissions: model.PermAccessOverview | model.PermAccessInbounds}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := model.User{Username: "role-admin-audit", Password: "original-binding", Enable: true, AdminRoleID: role.ID}
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	current := CurrentUserSession(&snapshot)
	if current == nil || !current.Can(model.PermAccessOverview) {
		t.Fatal("valid role was not loaded")
	}
	if err := db.Model(&role).UpdateColumn("permissions", model.PermAccessInbounds).Error; err != nil {
		t.Fatal(err)
	}
	current = CurrentUserSession(&snapshot)
	if current == nil || current.Can(model.PermAccessOverview) || !current.Can(model.PermAccessInbounds) {
		t.Fatal("role revocation did not preserve only allowed client access")
	}
	if err := db.Model(&model.User{}).Where("id=?", snapshot.Id).UpdateColumn("password", "new-binding").Error; err != nil {
		t.Fatal(err)
	}
	if CurrentUserSession(&snapshot) != nil {
		t.Fatal("password change retained captured authorization")
	}
}
