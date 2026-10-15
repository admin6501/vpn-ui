package controller

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
)

func TestGroupsAdministratorRoleControlsEachGroupOperation(t *testing.T) {
	env := newBulkMembershipEnv(t)
	db := database.GetDB()
	ib := seedMembershipInbound(t, env.admin.Id, model.VLESS, 45401, "")
	syncMembershipAccounts(t)
	if err := db.Create(&model.Setting{Key: "accountsMigratedAt", Value: "verified"}).Error; err != nil {
		t.Fatal(err)
	}
	gs := &service.GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	groups, err := gs.List(env.admin, false)
	if err != nil {
		t.Fatal(err)
	}
	role := model.AdminRole{Name: "Independent group role", Permissions: model.PermAccessGroups}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	ids := []int{groups[0].ID}
	admin, err := (&service.AdminService{}).AddAdmin(service.AdminSpec{Username: "group-permission-operator", Password: "password", Enable: true, AdminRoleID: &role.ID, GroupIDs: &ids})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		var current model.User
		db.First(&current, admin.Id)
		db.First(&role, role.ID)
		current.AdminRole = &role
		withUser(&current)(c)
	})
	registerGroups(r.Group("/panel"))
	call := func(method, path string, values url.Values) map[string]any {
		req := httptest.NewRequest(method, "/panel/groups"+path, strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err, w.Body.String())
		}
		return result
	}
	setPermissions := func(p model.Permission) {
		if err := db.Model(&role).Update("permissions", p).Error; err != nil {
			t.Fatal(err)
		}
	}
	save := url.Values{"id": {strconv.Itoa(groups[0].ID)}, "name": {"Editable group"}, "inboundIds": {strconv.Itoa(ib.Id)}}
	if got := call("GET", "/list", nil); got["success"] != true {
		t.Fatal("view groups denied", got)
	}
	for _, path := range []string{"/save", "/delete/" + strconv.Itoa(groups[0].ID)} {
		if got := call("POST", path, save); got["success"] != false {
			t.Fatal("view granted mutation", path, got)
		}
	}
	setPermissions(model.PermAccessGroups | model.PermCreateGroup)
	create := url.Values{"name": {"Created by scoped operator"}, "inboundIds": {strconv.Itoa(ib.Id)}}
	got := call("POST", "/save", create)
	if got["success"] != true {
		t.Fatal("create grant denied", got)
	}
	created := int(got["obj"].(map[string]any)["id"].(float64))
	if got := call("POST", "/save", save); got["success"] != false {
		t.Fatal("create granted editing", got)
	}
	setPermissions(model.PermAccessGroups | model.PermEditGroup)
	if got := call("POST", "/save", save); got["success"] != true {
		t.Fatal("edit grant denied", got)
	}
	if got := call("POST", "/save", create); got["success"] != false {
		t.Fatal("edit granted creation", got)
	}
	setPermissions(model.PermAccessGroups | model.PermDeleteGroup)
	if got := call("POST", "/delete/"+strconv.Itoa(created), nil); got["success"] != true {
		t.Fatal("delete grant denied own unused group", got)
	}
	setPermissions(model.PermAccessGroups | model.PermCreateGroup | model.PermDeleteGroup)
	got = call("POST", "/save", url.Values{"name": {"Empty delegated group"}})
	if got["success"] != true {
		t.Fatal("cannot create an empty group for later inbound assignment", got)
	}
	emptyID := int(got["obj"].(map[string]any)["id"].(float64))
	got = call("GET", "/list", nil)
	visible := false
	for _, row := range got["obj"].([]any) {
		if int(row.(map[string]any)["id"].(float64)) == emptyID {
			visible = true
		}
	}
	if !visible {
		t.Fatal("creator cannot manage their empty group", got)
	}
	other := model.User{Username: "another-group-operator", Enable: true, GroupAccess: true}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AdminGroupAccess{UserID: other.Id, GroupID: emptyID}).Error; err != nil {
		t.Fatal(err)
	}
	if got := call("POST", "/delete/"+strconv.Itoa(emptyID), nil); got["success"] != false {
		t.Fatal("deleted group still assigned to another administrator", got)
	}
	if err := db.Where("user_id = ?", other.Id).Delete(&model.AdminGroupAccess{}).Error; err != nil {
		t.Fatal(err)
	}
	if got := call("POST", "/delete/"+strconv.Itoa(emptyID), nil); got["success"] != true {
		t.Fatal("cannot delete own empty unused group", got)
	}
	setPermissions(model.PermEditInbound)
	if got := call("GET", "/list", nil); got["success"] != false {
		t.Fatal("inbound edit still grants group access", got)
	}
	if got := call("POST", "/save", save); got["success"] != false {
		t.Fatal("inbound edit still grants group editing", got)
	}
}
