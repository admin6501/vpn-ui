package controller

import (
	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/web/websocket"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRepresentativeOverviewExplicitProfileCannotBeBypassedByRole(t *testing.T) {
	newPermTestDB(t)
	user := newReseller(t, 71, false, false)
	user.RepresentativeRoleID = 7
	user.RepresentativeRole = &model.RepresentativeRole{ID: 7, Permissions: `{"users":{"read":{"scope":1}},"system":{"read":true}}`}
	if !user.Can(model.PermAccessOverview) {
		t.Fatal("role fixture must reproduce implicit system read")
	}
	if overviewAccess(user) {
		t.Fatal("role's implicit system read bypassed explicit disabled Overview")
	}
	if templatePerms(permCtx(user))["accessOverview"] {
		t.Fatal("Overview menu visible without profile grant")
	}
	if location := landingPath(permCtx(user)); location != "/panel/clients" {
		t.Fatal("wrong landing page:", location)
	}
	response := overviewNav(t, user, "/panel/")
	if response.Header().Get("Location") != "/panel/clients" || strings.Contains(response.Body.String(), overviewBody) {
		t.Fatal("disabled Overview page was rendered")
	}
	// The request-scoped read-all flag must not erase representative identity.
	scoped := *user
	scoped.IsReseller = false
	if overviewAccess(&scoped) {
		t.Fatal("role scope flag bypassed disabled Overview")
	}
	database.GetDB().Model(&model.ResellerProfile{}).Where("user_id=?", user.Id).Update("allow_overview", true)
	if !overviewAccess(user) || !templatePerms(permCtx(user))["accessOverview"] {
		t.Fatal("explicitly granted Overview was rejected")
	}
}

func TestRepresentativeOverviewExplicitGrantDoesNotDependOnHiddenRoleSystemBit(t *testing.T) {
	newPermTestDB(t)
	user := newReseller(t, 72, true, true)
	user.RepresentativeRoleID = 8
	user.RepresentativeRole = &model.RepresentativeRole{ID: 8, Permissions: `{"users":{"read":{"scope":1}},"system":{"read":false}}`}
	if user.Can(model.PermAccessOverview) {
		t.Fatal("fixture must have no role-level system read")
	}
	if !overviewAccess(user) || !overviewManage(user) {
		t.Fatal("explicit representative switches must govern Overview")
	}
	perms := templatePerms(permCtx(user))
	if !perms["accessOverview"] || !perms["manageOverview"] {
		t.Fatal("menu disagrees with route permissions")
	}
	database.GetDB().Model(&model.ResellerProfile{}).Where("user_id=?", user.Id).Update("allow_overview", false)
	if overviewAccess(user) || overviewManage(user) {
		t.Fatal("manage-only profile opened Overview")
	}
}

func TestRepresentativeOverviewRoleCannotReadHostStatusOrXrayEvents(t *testing.T) {
	f := newResellerFixture(t)
	f.sara.RepresentativeRoleID = 7
	f.sara.RepresentativeRole = &model.RepresentativeRole{ID: 7, Permissions: `{"users":{"read":{"scope":1}},"system":{"read":true}}`}
	for _, route := range []string{"status", "cpuHistory/minute", "panelLocation", "serverName", "distroStatus", "geofileStatus", "geofileProgress", "getXrayVersion"} {
		body := overviewCall(t, f.sara, "GET", "/panel/api/server/"+route, "")
		if !strings.Contains(body, `"overviewDenied":true`) || !strings.Contains(body, forbiddenKey) {
			t.Fatalf("role bypassed %s: %s", route, body)
		}
	}
	for _, message := range []string{`{"type":"status","payload":{"hostname":"private-host"}}`, `{"type":"xray_state","payload":{"state":"error","errorMsg":"private-host"}}`} {
		if _, allowed := filterSocketMessage(f.sara, []byte(message)); allowed {
			t.Fatal("role bypassed live host status")
		}
	}
	if _, allowed := filterSocketMessage(f.sara, []byte(`{"type":"invalidate","payload":{"type":"inbounds"}}`)); !allowed {
		t.Fatal("client changes were blocked with Overview")
	}
	// Assigned client read models remain usable.
	response := f.as(t, f.sara, "GET", "/panel/api/clients/inbounds", "")
	if !strings.Contains(response.Body.String(), `"success":true`) || !strings.Contains(response.Body.String(), f.saraEmail) {
		t.Fatal("representative lost own client access")
	}
}

func TestRepresentativeOverviewIdentityPreservesImportedAdministratorGrants(t *testing.T) {
	newPermTestDB(t)
	for _, user := range []*model.User{
		{Enable: true, IsSuperAdmin: true, IsReseller: true, RepresentativeRoleID: 8},
		{Enable: true, IsPanelOwner: true, Permissions: model.PermAccessOverview | model.PermOverviewManage, RepresentativeRoleID: 8},
		{Enable: true, AdminRoleID: 9, AdminRole: &model.AdminRole{ID: 9, Permissions: model.PermAccessOverview | model.PermOverviewManage}, RepresentativeRoleID: 8},
	} {
		if isRepresentative(user) {
			t.Fatal("administrator fixture was classified as representative")
		}
		if !overviewAccess(user) || !overviewManage(user) {
			t.Fatal("imported administrator permission was replaced by a missing representative profile")
		}
		perms := templatePerms(permCtx(user))
		if !perms["accessOverview"] || !perms["manageOverview"] {
			t.Fatal("imported administrator menu hidden")
		}
	}
}

func TestRepresentativeOverviewRoleProfileRevocationOnOpenSocket(t *testing.T) {
	f := newResellerFixture(t)
	db := database.GetDB()
	role := model.RepresentativeRole{Name: "implicit-system-read", Permissions: `{"users":{"read":{"scope":1}},"system":{"read":true}}`}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	db.Model(f.sara).Update("representative_role_id", role.ID)
	db.Model(&model.ResellerProfile{}).Where("user_id=?", f.sara.Id).Update("allow_overview", true)
	snapshot := *f.sara
	messages := make(chan []byte, 6)
	r := gin.New()
	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		(&WebSocketController{}).writePump(&websocket.Client{ID: "rep-overview-role", Send: messages}, conn, &snapshot)
	})
	server := httptest.NewServer(r)
	defer server.Close()
	conn, _, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer close(messages)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	status := []byte(`{"type":"status","payload":{"hostname":"private-host"},"time":1}`)
	messages <- status
	if _, message, err := conn.ReadMessage(); err != nil || !strings.Contains(string(message), "private-host") {
		t.Fatalf("granted representative status denied: %s %v", message, err)
	}
	if err := db.Model(&model.ResellerProfile{}).Where("user_id=?", f.sara.Id).Update("allow_overview", false).Error; err != nil {
		t.Fatal(err)
	}
	messages <- status
	_, notice, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(notice), "overview_access") || strings.Contains(string(notice), "private-host") {
		t.Fatalf("revoked profile still sees status or lacks navigation notice: %s %v", notice, err)
	}
	// No repeated denial notices or host status; own client updates remain live.
	messages <- status
	messages <- []byte(`{"type":"invalidate","payload":{"type":"inbounds"},"time":2}`)
	_, update, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(update), `"invalidate"`) {
		t.Fatalf("client refresh lost after profile revocation: %s %v", update, err)
	}
}
