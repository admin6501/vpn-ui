package controller

import (
	"encoding/json"
	"fmt"
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

func TestGroupsRepresentativeAPIUsesGroupsAndSavesAtomically(t *testing.T) {
	f := newResellerFixture(t)
	db := database.GetDB()
	role := model.RepresentativeRole{Name: "Group role", Access: `{}`}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.User{}).Where("id = ?", f.sara.Id).Update("representative_role_id", role.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.ResellerProfile{}).Where("user_id = ?", f.sara.Id).Update("consumption_based", true).Error; err != nil {
		t.Fatal(err)
	}
	syncMembershipAccounts(t)
	if err := db.Create(&model.Setting{Key: "accountsMigratedAt", Value: "verified"}).Error; err != nil {
		t.Fatal(err)
	}
	gs := &service.GroupService{}
	if err := gs.Migrate(); err != nil {
		t.Fatal(err)
	}
	groups, err := gs.List(f.ali, false)
	if err != nil || len(groups) != 1 {
		t.Fatal(groups, err)
	}
	root := *f.ali
	root.IsSuperAdmin = true
	all, err := gs.List(&root, false)
	if err != nil {
		t.Fatal(err)
	}
	foreign := 0
	for _, g := range all {
		if g.ID != groups[0].ID {
			foreign = g.ID
		}
	}
	actor := f.ali
	r := gin.New()
	r.Use(func(c *gin.Context) { withUser(actor)(c) })
	NewResellerController(r.Group("/panel"))
	form := url.Values{"username": {f.sara.Username}, "enable": {"true"}, "nickname": {"group-metadata"}, "representativeRoleId": {strconv.Itoa(role.ID)}, "groupIds": {strconv.Itoa(groups[0].ID)}, "twoFactorRequired": {"true"}}
	post := func() map[string]any {
		req := httptest.NewRequest("POST", fmt.Sprintf("/panel/resellers/update/%d", f.sara.Id), strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	if got := post(); got["success"] != true {
		t.Fatal("delegated group metadata update failed", got)
	}
	// Failed grant writes must roll back metadata and the prior group grant.
	if err := db.Exec(`CREATE TRIGGER reject_representative_group BEFORE INSERT ON representative_group_accesses BEGIN SELECT RAISE(ABORT,'group failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	form.Set("nickname", "must rollback")
	if got := post(); got["success"] != false {
		t.Fatal("expected failure", got)
	}
	var stored model.User
	if err := db.First(&stored, f.sara.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Nickname != "group-metadata" || !stored.TwoFactorRequired || !stored.GroupAccess {
		t.Fatal("partial update committed")
	}
	db.Exec(`DROP TRIGGER reject_representative_group`)
	for _, bad := range []string{strconv.Itoa(foreign), "garbage", "0", "-1"} {
		form.Set("groupIds", bad)
		if got := post(); got["success"] != false {
			t.Fatal("invalid/out-of-scope group accepted", bad, got)
		}
	}
	form.Del("groupIds")
	form.Set("inboundIds", strconv.Itoa(f.aliInbound.Id))
	if got := post(); got["success"] != false {
		t.Fatal("individual inbound assignment still accepted", got)
	}
	form.Del("inboundIds")
	form.Set("nickname", "metadata only")
	if got := post(); got["success"] != true {
		t.Fatal("omitted groups did not preserve", got)
	}
	access := &service.AdminService{}
	if ok, err := access.CanAccessInbound(f.aliInbound.Id, f.sara.Id); err != nil || !ok {
		t.Fatal("omitted groups revoked access", ok, err)
	}
	form.Set("groupIds", "")
	if got := post(); got["success"] != true {
		t.Fatal("explicit empty groups failed", got)
	}
	if ok, err := access.CanAccessInbound(f.aliInbound.Id, f.sara.Id); err != nil || ok {
		t.Fatal("empty groups retained stale inbound access", ok, err)
	}
}
