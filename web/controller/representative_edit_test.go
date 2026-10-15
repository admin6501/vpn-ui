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
)

func TestRepresentativeEditPreservesImportedGrants(t *testing.T) {
	for _, access := range []string{`{"allowed_group_ids":[]}`, `{"require_template":true}`} {
		for _, kind := range []string{"super", "delegated", "limited-owner"} {
			t.Run(access+"/"+kind, func(t *testing.T) {
				f := newResellerFixture(t)
				db := database.GetDB()
				role := model.RepresentativeRole{Name: "Imported role", Permissions: `{"users":{"read":{"scope":1}}}`, Limits: `{}`, Features: `{}`, Access: access}
				if err := db.Create(&role).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Model(f.sara).Update("representative_role_id", role.ID).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Model(&model.ResellerProfile{}).Where("user_id = ?", f.sara.Id).Update("consumption_based", true).Error; err != nil {
					t.Fatal(err)
				}
				actor := *f.ali
				if kind == "super" {
					actor.IsSuperAdmin = true
				}
				if kind == "limited-owner" {
					actor.IsPanelOwner = true
				}
				currentActor := &actor
				r := gin.New()
				r.Use(func(c *gin.Context) { withUser(currentActor)(c) })
				NewResellerController(r.Group("/panel"))
				form := url.Values{"username": {f.sara.Username}, "enable": {"true"}, "nickname": {"saved-metadata"}, "allowanceGb": {"100"}, "representativeRoleId": {strconv.Itoa(role.ID)}, "inboundIds": {strconv.Itoa(f.aliInbound.Id)}, "twoFactorRequired": {"true"}}
				post := func(values url.Values) map[string]any {
					t.Helper()
					req := httptest.NewRequest("POST", fmt.Sprintf("/panel/resellers/update/%d", f.sara.Id), strings.NewReader(values.Encode()))
					req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
					var result map[string]any
					if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					return result
				}
				assertStored := func(nickname string, ids []int) {
					t.Helper()
					var u model.User
					if err := db.First(&u, f.sara.Id).Error; err != nil {
						t.Fatal(err)
					}
					if u.Nickname != nickname || u.RepresentativeRoleID != role.ID || u.Password != f.sara.Password || !u.TwoFactorRequired {
						t.Fatal("incorrect saved representative", u.Nickname, u.RepresentativeRoleID, u.TwoFactorRequired)
					}
					var grants []int
					if err := db.Model(&model.InboundAccess{}).Where("user_id = ?", f.sara.Id).Order("inbound_id").Pluck("inbound_id", &grants).Error; err != nil {
						t.Fatal(err)
					}
					if fmt.Sprint(grants) != fmt.Sprint(ids) {
						t.Fatalf("grants changed: %v, want %v", grants, ids)
					}
				}
				if got := post(form); got["success"] != true {
					t.Fatal("unchanged imported grants blocked metadata edit", got)
				}
				assertStored("saved-metadata", []int{f.aliInbound.Id})
				form.Del("representativeRoleId")
				form.Set("nickname", "omitted-role")
				if got := post(form); got["success"] != true {
					t.Fatal("omitted role was not preserved", got)
				}
				assertStored("omitted-role", []int{f.aliInbound.Id})
				// New assignments remain subject to group/template restrictions, for every actor.
				form.Set("nickname", "must-not-save")
				form.Add("inboundIds", strconv.Itoa(f.rezaInbound.Id))
				if got := post(form); got["success"] != false {
					t.Fatal("new forbidden grant accepted", got)
				}
				assertStored("omitted-role", []int{f.aliInbound.Id})
				form["inboundIds"] = []string{strconv.Itoa(f.aliInbound.Id)}
				otherRole := model.RepresentativeRole{Name: "Another role", Access: access, Limits: `{}`, Permissions: `{}`, Features: `{}`}
				if err := db.Create(&otherRole).Error; err != nil {
					t.Fatal(err)
				}
				form.Set("representativeRoleId", strconv.Itoa(otherRole.ID))
				if got := post(form); got["success"] != false {
					t.Fatal("role reassignment bypassed permission or full grant validation", got)
				}
				assertStored("omitted-role", []int{f.aliInbound.Id})
				form.Set("representativeRoleId", strconv.Itoa(role.ID))
				currentActor = f.reza
				if got := post(form); got["success"] != false {
					t.Fatal("another owner changed representative", got)
				}
				currentActor = &actor
				// If grant storage fails, metadata/profile updates must roll back with it.
				if err := db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_rep_grant BEFORE INSERT ON inbound_accesses WHEN NEW.user_id = %d BEGIN SELECT RAISE(ABORT, 'test grant failure'); END`, f.sara.Id)).Error; err != nil {
					t.Fatal(err)
				}
				form.Set("allowanceGb", "7")
				if got := post(form); got["success"] != false {
					t.Fatal("grant storage failure ignored", got)
				}
				assertStored("omitted-role", []int{f.aliInbound.Id})
				var profile model.ResellerProfile
				if err := db.Where("user_id = ?", f.sara.Id).First(&profile).Error; err != nil {
					t.Fatal(err)
				}
				if profile.AllowanceBytes != 100*testGB {
					t.Fatal("partial allowance edit survived failed save")
				}
				if err := db.Exec(`DROP TRIGGER reject_rep_grant`).Error; err != nil {
					t.Fatal(err)
				}
				form["inboundIds"] = []string{""}
				form.Set("nickname", "revoked")
				if got := post(form); got["success"] != true {
					t.Fatal("revoking grandfathered access failed", got)
				}
				assertStored("revoked", []int{})
			})
		}
	}
}

func TestRepresentativeEditAddsAllowedGrantBesideImportedGrant(t *testing.T) {
	f := newResellerFixture(t)
	db := database.GetDB()
	role := model.RepresentativeRole{Name: "Restricted imported role", Permissions: `{}`, Limits: `{}`, Features: `{}`, Access: `{"allowed_group_ids":[7]}`}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(f.sara).Update("representative_role_id", role.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.ResellerProfile{}).Where("user_id = ?", f.sara.Id).Update("consumption_based", true).Error; err != nil {
		t.Fatal(err)
	}
	resources := []model.PasarGuardResource{
		{Kind: "inbounds", SourceID: 20, Data: fmt.Sprintf(`{"id":"20","tag":%q}`, f.rezaInbound.Tag)},
		{Kind: "inbounds_groups_association", SourceID: 21, Data: `{"group_id":"7","inbound_id":"20"}`},
	}
	if err := db.Create(&resources).Error; err != nil {
		t.Fatal(err)
	}
	actor := *f.ali
	actor.IsSuperAdmin = true
	r := gin.New()
	r.Use(withUser(&actor))
	NewResellerController(r.Group("/panel"))
	form := url.Values{"username": {f.sara.Username}, "enable": {"true"}, "allowanceGb": {"100"}, "representativeRoleId": {strconv.Itoa(role.ID)}, "inboundIds": {strconv.Itoa(f.aliInbound.Id), strconv.Itoa(f.rezaInbound.Id)}}
	save := func() {
		t.Helper()
		req := httptest.NewRequest("POST", fmt.Sprintf("/panel/resellers/update/%d", f.sara.Id), strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result["success"] != true {
			t.Fatal("valid assignment was rejected", result)
		}
	}
	save()
	var n int64
	if err := db.Model(&model.InboundAccess{}).Where("user_id = ?", f.sara.Id).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatal("allowed new grant or existing imported grant was lost")
	}
	// A super administrator can still make a valid role reassignment.
	next := model.RepresentativeRole{Name: "Unrestricted role", Permissions: `{}`, Limits: `{}`, Features: `{}`, Access: `{}`}
	if err := db.Create(&next).Error; err != nil {
		t.Fatal(err)
	}
	form.Set("representativeRoleId", strconv.Itoa(next.ID))
	save()
	var stored model.User
	if err := db.First(&stored, f.sara.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.RepresentativeRoleID != next.ID {
		t.Fatal("valid role reassignment was not saved")
	}
}
