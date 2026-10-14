package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestRepresentativeInboundManagementDenied(t *testing.T) {
	f := newResellerFixture(t)
	for _, route := range []struct{ method, path string }{
		{"GET", "/list"}, {"GET", "/get/%d"}, {"GET", "/nodes"}, {"GET", "/pools"}, {"GET", "/%d/addressing"},
		{"POST", "/add"}, {"POST", "/update/%d"}, {"POST", "/del/%d"}, {"POST", "/import"}, {"POST", "/reorder"},
		{"POST", "/resetAllTraffics"}, {"POST", "/resetAllClientTraffics/%d"}, {"POST", "/delDepletedClients/%d"},
		{"POST", "/%d/copyClients"}, {"POST", "/generate-openvpn-certs"}, {"POST", "/%d/generate-openvpn-certs"},
		{"POST", "/generate-ocserv-cert"}, {"POST", "/generate-sstp-cert"}, {"POST", "/generate-ikev2-cert"}, {"POST", "/check-ikev2-cert"},
	} {
		path := route.path
		if strings.Contains(path, "%d") {
			path = fmt.Sprintf(path, f.aliInbound.Id)
		}
		w := f.as(t, f.sara, route.method, "/panel/api/inbounds"+path, "")
		if !strings.Contains(w.Body.String(), "pages.admins.forbidden") {
			t.Fatalf("management route %s %s was not denied: %s", route.method, path, w.Body.String())
		}
	}
	// Even a read-all role's request-scoped IsReseller=false must not reopen management.
	rep := *f.sara
	rep.IsReseller = false
	rep.RepresentativeRoleID = 42
	w := f.as(t, &rep, "GET", "/panel/api/inbounds/list", "")
	if !strings.Contains(w.Body.String(), "pages.admins.forbidden") {
		t.Fatal("role representative bypassed management boundary")
	}
	perms := templatePerms(permCtx(f.sara))
	if perms["accessInboundManagement"] || perms["checkUpdates"] || !perms["accessInbounds"] {
		t.Fatalf("incorrect representative UI capabilities: %+v", perms)
	}
	// Assigned picker data remains accessible and excludes other owners' credentials.
	for _, path := range []string{"/panel/api/clients/assignable", "/panel/api/clients/inbounds"} {
		w = f.as(t, f.sara, "GET", path, "")
		var msg struct {
			Success bool `json:"success"`
		}
		if json.Unmarshal(w.Body.Bytes(), &msg) != nil || !msg.Success {
			t.Fatalf("client selector denied: %s", w.Body.String())
		}
		if strings.Contains(w.Body.String(), "ali-uuid") {
			t.Fatal("another owner's credential leaked")
		}
	}
}

func TestRepresentativeUpdateCheckDeniedAndOwnBalance(t *testing.T) {
	f := newResellerFixture(t)
	r := gin.New()
	r.Use(withUser(f.sara))
	(&ServerController{}).initRouter(r.Group("/panel/api/server"))
	for _, path := range []string{"checkUpdate", "updateProgress"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/panel/api/server/"+path, nil))
		if !strings.Contains(w.Body.String(), "pages.admins.forbidden") {
			t.Fatalf("representative update API accessible: %s", w.Body.String())
		}
	}
	// Balance's identity is always the authenticated caller; query parameters cannot choose an owner.
	w := f.as(t, f.sara, "GET", "/panel/api/clients/balance?userId=1", "")
	var response struct {
		Success bool `json:"success"`
		Obj     struct {
			Allowance int64 `json:"allowanceBytes"`
			Available int64 `json:"availableBytes"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success || response.Obj.Allowance != 100*testGB {
		t.Fatalf("wrong own balance: %s", w.Body.String())
	}

}

func TestRepresentativeClientReadModelRedactsServerSecrets(t *testing.T) {
	f := newResellerFixture(t)
	if err := database.GetDB().Model(f.aliInbound).Update("stream_settings", `{"network":"tcp","security":"reality","realitySettings":{"privateKey":"server-private","mldsa65Seed":"server-seed","settings":{"publicKey":"public-peer-key"}}}`).Error; err != nil {
		t.Fatal(err)
	}
	w := f.as(t, f.sara, "GET", "/panel/api/clients/inbounds", "")
	for _, secret := range []string{"server-private", "server-seed", "ali-uuid"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("client read leaked %s", secret)
		}
	}
	for _, required := range []string{f.saraEmail, "public-peer-key"} {
		if !strings.Contains(w.Body.String(), required) {
			t.Fatalf("client card data missing %s: %s", required, w.Body.String())
		}
	}
	var stored model.Inbound
	if err := database.GetDB().First(&stored, f.aliInbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.StreamSettings, "server-private") {
		t.Fatal("redaction changed stored server configuration")
	}
}

func TestRepresentativeBoundaryPreservesAdministratorAccess(t *testing.T) {
	f := newIdorFixture(t)
	for _, kind := range []string{"ordinary", "imported-super", "imported-owner", "delegated-role"} {
		t.Run(kind, func(t *testing.T) {
			admin := *f.ali
			switch kind {
			case "imported-super":
				admin.IsSuperAdmin = true
				admin.RepresentativeRoleID = 42
			case "imported-owner":
				admin.IsPanelOwner = true
				admin.RepresentativeRoleID = 42
			case "delegated-role":
				admin.AdminRoleID = 9
				admin.AdminRole = &model.AdminRole{ID: 9, Permissions: admin.Permissions}
				admin.RepresentativeRoleID = 42
			}
			if isRepresentative(&admin) {
				t.Fatal("administrator was classified as representative")
			}
			perms := templatePerms(permCtx(&admin))
			if !perms["accessInboundManagement"] || !perms["checkUpdates"] || templateReseller(permCtx(&admin))["isReseller"] != false {
				t.Fatal("administrator UI controls were hidden")
			}
			w := f.as(t, &admin, "GET", "/panel/api/inbounds/list", "")
			if strings.Contains(w.Body.String(), "pages.admins.forbidden") || !strings.Contains(w.Body.String(), f.aliEmail) {
				t.Fatalf("administrator lost inbound API access: %s", w.Body.String())
			}
		})
	}
}
