package controller

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestRepresentativeRoleCRUDAndAssignedDeletion(t *testing.T) {
	newIdorFixture(t)
	root := &model.User{Id: 1, Enable: true, IsSuperAdmin: true}
	r := gin.New()
	r.Use(withUser(root))
	NewResellerController(r.Group("/panel"))
	request := func(method, path string, body url.Values) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(w.Body.String())
		}
		return result
	}
	response := request("GET", "/panel/resellers/roles", nil)
	if response["obj"] == nil {
		t.Fatal("empty role list must be [] not null")
	}
	form := url.Values{"name": {"  Standard  "}, "permissions": {`{"users":{"read":{"scope":1},"create":true}}`}, "limits": {"{}"}, "features": {"{}"}, "access": {"{}"}}
	response = request("POST", "/panel/resellers/roles/add", form)
	if response["success"] != true {
		t.Fatal(response)
	}
	obj := response["obj"].(map[string]any)
	id := int(obj["id"].(float64))
	if obj["name"] != "Standard" {
		t.Fatal("untrimmed role")
	}
	rep := model.User{Username: "role-representative", Enable: true, IsReseller: true, RepresentativeRoleID: id}
	if err := database.GetDB().Create(&rep).Error; err != nil {
		t.Fatal(err)
	}
	path := "/panel/resellers/roles/delete/" + jsonNumber(id)
	response = request("POST", path, nil)
	if response["success"] != false {
		t.Fatal("assigned role deleted")
	}
	form.Set("name", "Updated")
	response = request("POST", "/panel/resellers/roles/update/"+jsonNumber(id), form)
	if response["success"] != true {
		t.Fatal(response)
	}
	database.GetDB().Model(&rep).Update("representative_role_id", 0)
	response = request("POST", path, nil)
	if response["success"] != true {
		t.Fatal(response)
	}
	owner := model.RepresentativeRole{Name: "Owner", IsOwner: true}
	database.GetDB().Create(&owner)
	response = request("POST", "/panel/resellers/roles/delete/"+jsonNumber(owner.ID), nil)
	if response["success"] != false {
		t.Fatal("owner role deleted")
	}
}
func jsonNumber(n int) string { b, _ := json.Marshal(n); return string(b) }
func TestRepresentativeRoleWritesDenyDelegatedAdmin(t *testing.T) {
	newIdorFixture(t)
	user := &model.User{Id: 2, Enable: true, Permissions: model.PermManageResellers}
	r := gin.New()
	r.Use(withUser(user))
	g := r.Group("/panel")
	NewResellerController(g)
	for _, target := range []struct{ method, path string }{{"POST", "/panel/resellers/roles/add"}, {"POST", "/panel/resellers/roles/delete/1"}} {
		req := httptest.NewRequest(target.method, target.path, nil)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if !strings.Contains(w.Body.String(), `"success":false`) {
			t.Fatalf("delegated access to %s: %d %s", target.path, w.Code, w.Body.String())
		}
	}
}
