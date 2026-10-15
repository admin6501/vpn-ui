package controller

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestGroupsClientSelectionIsAuthoritativeAndNewInboundsStayUnusable(t *testing.T) {
	env := newBulkMembershipEnv(t)
	a := seedMembershipInbound(t, env.admin.Id, model.VLESS, 44101, `{"email":"existing","id":"11111111-1111-4111-8111-111111111111","enable":true}`)
	syncMembershipAccounts(t)
	if err := database.GetDB().Create(&model.Setting{Key: "accountsMigratedAt", Value: "verified"}).Error; err != nil {
		t.Fatal(err)
	}
	gs := &service.GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	b := seedMembershipInbound(t, env.admin.Id, model.Trojan, 44102, "")
	groups, err := gs.List(env.admin, false)
	if err != nil || len(groups) != 1 {
		t.Fatal(groups, err)
	}
	settings := `{"clients":[{"email":"created-with-group","id":"22222222-2222-4222-8222-222222222222","enable":true}]}`
	form := url.Values{"settings": {settings}, "inboundIds": {strconv.Itoa(b.Id)}}
	response := bulkMembershipPost(t, env.admin, "/panel/api/inbounds/saveAccount", form.Encode())
	if response["success"] != false {
		t.Fatal("ungrouped direct assignment was accepted", response)
	}
	var count int64
	database.GetDB().Model(&model.Account{}).Where("email = ?", "created-with-group").Count(&count)
	if count != 0 {
		t.Fatal("rejected form created an account")
	}
	form["groupIds"] = []string{strconv.Itoa(groups[0].ID)}
	response = bulkMembershipPost(t, env.admin, "/panel/api/inbounds/saveAccount", form.Encode())
	if response["success"] != true {
		t.Fatal(response)
	}
	ids, err := (&service.AccountService{}).InboundIdsForEmail("created-with-group")
	if err != nil || len(ids) != 1 || ids[0] != a.Id {
		t.Fatal("forged inboundIds overrode group's authoritative set", ids, err)
	}
	form.Set("id", strconv.Itoa(b.Id))
	form.Set("settings", `{"clients":[{"email":"invalid-anchor","password":"never-created","enable":true}]}`)
	response = bulkMembershipPost(t, env.admin, "/panel/api/inbounds/addClient", form.Encode())
	if response["success"] != false {
		t.Fatal("ungrouped anchor was accepted", response)
	}
}
func TestGroupsManagementAndAccountIsolation(t *testing.T) {
	env := newBulkMembershipEnv(t)
	a := seedMembershipInbound(t, env.admin.Id, model.VLESS, 44201, `{"email":"private-account","id":"11111111-1111-4111-8111-111111111111","enable":true}`)
	syncMembershipAccounts(t)
	gs := &service.GroupService{}
	group, _, err := gs.Save(env.admin, 0, "Private", []int{a.Id})
	if err != nil {
		t.Fatal(err)
	}
	delegate := model.User{Username: "group-editor", Enable: true, Permissions: model.PermAccessInbounds | model.PermAccessGroups | model.PermCreateGroup | model.PermEditGroup | model.PermDeleteGroup | model.PermEditClient}
	if err := database.GetDB().Create(&delegate).Error; err != nil {
		t.Fatal(err)
	}
	b := seedMembershipInbound(t, delegate.Id, model.Trojan, 44202, "")
	if err := database.GetDB().Create(&model.InboundAccess{UserId: delegate.Id, InboundId: b.Id}).Error; err != nil {
		t.Fatal(err)
	}
	visible, _, err := gs.Save(&delegate, 0, "Delegate", []int{b.Id})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := gs.Save(&delegate, group.ID, "Steal", []int{b.Id}); err == nil {
		t.Fatal("foreign group could be replaced")
	}
	if _, err := gs.Apply(&delegate, "private-account", 0, []int{visible.ID}); err == nil {
		t.Fatal("arbitrary foreign email could be attached to a visible group")
	}
	// The normal addClient path writes the anchor before the account mirror exists.
	// A delegated creator may attach that newly created account to its groups.
	if err := database.GetDB().Model(b).Update("settings", `{"clients":[{"email":"delegated-new","password":"kept-password","enable":true}]}`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Apply(&delegate, "delegated-new", b.Id, []int{visible.ID}); err != nil {
		t.Fatal("delegated newly-created account was refused", err)
	}
	rep := model.User{Username: "group-representative", Enable: true, IsReseller: true}
	if _, _, err := gs.Save(&rep, 0, "Denied", []int{b.Id}); err == nil {
		t.Fatal("representative managed group infrastructure")
	}
	r := gin.New()
	r.Use(withUser(&rep))
	registerGroups(r.Group("/panel"))
	req := httptest.NewRequest("POST", "/panel/groups/save", strings.NewReader(url.Values{"name": {"Denied"}, "inboundIds": {strconv.Itoa(b.Id)}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(w.Body.String())
	}
	if result["success"] != false {
		t.Fatal("route accepted representative group write", result)
	}
}
