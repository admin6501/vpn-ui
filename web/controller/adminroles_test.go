package controller

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityOtherSuperCannotEditProtectedOwnerOrAssignedRole(t *testing.T) {
	f := newIdorFixture(t)
	db := database.GetDB()
	var owner model.User
	db.Where("is_panel_owner = ?", true).First(&owner)
	f.ali.IsSuperAdmin = true
	db.Save(f.ali)
	svc := &service.AdminService{}
	if err := svc.SaveAdminRole(&owner, 0, "Protected role", model.PermManageNodes); err != nil {
		t.Fatal(err)
	}
	var role model.AdminRole
	db.First(&role)
	if err := svc.UpdateAdmin(owner.Id, service.AdminSpec{Username: owner.Username, Enable: true, AdminRoleID: &role.ID}); err != nil {
		t.Fatal(err)
	}
	var before model.User
	db.First(&before, owner.Id)
	r := gin.New()
	r.Use(withUser(f.ali))
	NewAdminController(r.Group("/panel"))
	for _, tc := range []struct{ path, body string }{
		{fmt.Sprintf("/panel/admins/update/%d", owner.Id), `{"username":"hijacked","password":"replacement","enable":true,"isSuperAdmin":true,"isPanelOwner":false}`},
		{fmt.Sprintf("/panel/admins/del/%d", owner.Id), `{}`},
		{fmt.Sprintf("/panel/admins/roles/update/%d", role.ID), `{"name":"Overridden","permissions":["accessPanelSettings"]}`},
		{fmt.Sprintf("/panel/admins/roles/delete/%d", role.ID), `{}`},
	} {
		req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if !strings.Contains(w.Body.String(), `"success":false`) {
			t.Fatal(tc.path, w.Body.String())
		}
	}
	var after model.User
	db.First(&after, owner.Id)
	if after.Username != before.Username || after.Password != before.Password || after.AdminRoleID != before.AdminRoleID || !after.IsPanelOwner || after.IsSuperAdmin {
		t.Fatal("protected owner changed")
	}
	if err := svc.GuardAdminTarget(&after, after.Id); err != nil {
		t.Fatal("owner cannot manage itself", err)
	}
}
func TestSecurityNonOwnerCannotReplaceDatabaseAuthenticationOrExecutable(t *testing.T) {
	newIdorFixture(t)
	actor := &model.User{Id: 2, Enable: true, IsSuperAdmin: true}
	r := gin.New()
	r.Use(withUser(actor))
	(&ServerController{}).initRouter(r.Group("/panel/api/server"))
	(&SettingController{}).initRouter(r.Group("/panel"))
	(&CoreController{}).initRouter(r.Group("/panel"))
	for _, tc := range []struct{ method, path string }{
		{"GET", "/panel/api/server/getDb"}, {"POST", "/panel/api/server/importDB"},
		{"POST", "/panel/api/server/uploadPanelBinary"}, {"POST", "/panel/api/server/applyPanelBinary"},
		{"POST", "/panel/setting/update"}, {"POST", "/panel/setting/all"},
		{"POST", "/panel/setting/service"}, {"POST", "/panel/core/config/openvpn"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		body := w.Body.String()
		if !strings.Contains(body, `"success":false`) || !strings.Contains(body, "ownerOnly") {
			t.Fatal(tc.path, body)
		}
	}
}
