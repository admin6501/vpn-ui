package session

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestSecurityAdminRoleRevocationUsesCurrentRoleWithoutNewLogin(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "role-session.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	role := model.AdminRole{Name: "Node access", Permissions: model.PermManageNodes}
	db.Create(&role)
	user := model.User{Username: "role-member", Password: "test-password-hash", Enable: true, AdminRoleID: role.ID, Permissions: model.PermManageNodes}
	db.Create(&user)
	r := gin.New()
	r.Use(sessions.Sessions("vpn-ui", cookie.NewStore([]byte("admin-role-session-test-key"))))
	r.POST("/login", func(c *gin.Context) { SetLoginUser(c, &user); sessions.Default(c).Save(); c.Status(200) })
	r.GET("/nodes", func(c *gin.Context) {
		u := GetLoginUser(c)
		if u == nil {
			c.Status(401)
			return
		}
		if !u.Can(model.PermManageNodes) {
			c.Status(403)
			return
		}
		c.Status(200)
	})
	r.GET("/management", func(c *gin.Context) {
		u := GetLoginUser(c)
		if u != nil && u.CanManageAdmins() {
			c.Status(200)
		} else {
			c.Status(403)
		}
	})
	call := func(method, path string, ck *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if ck != nil {
			req.AddCookie(ck)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	login := call("POST", "/login", nil)
	ck := login.Result().Cookies()[0]
	if call("GET", "/nodes", ck).Code != 200 {
		t.Fatal("role permission was not loaded")
	}
	db.Model(&role).Update("permissions", 0)
	if call("GET", "/nodes", ck).Code != 403 {
		t.Fatal("role revocation did not affect an existing session")
	}
	if call("GET", "/management", ck).Code != 403 {
		t.Fatal("role member gained administrator management")
	}
	db.Delete(&role)
	if call("GET", "/nodes", ck).Code != 401 {
		t.Fatal("missing role falls back to stale mask")
	}
	db.Model(&user).Update("is_panel_owner", true)
	if call("GET", "/management", ck).Code != 200 {
		t.Fatal("owner cannot repair a missing role")
	}
}
