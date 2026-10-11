package session

import (
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/securecookie"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestSecuritySessionsRevokeCopiedCookies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if err := database.InitDB(filepath.Join(t.TempDir(), "session.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	user := model.User{Username: "security-admin", Password: "not-a-plaintext-password-hash", Enable: true, IsSuperAdmin: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	const key = "session-security-test-key"
	r := gin.New()
	r.Use(sessions.Sessions("vpn-ui", cookie.NewStore([]byte(key))))
	r.POST("/login", func(c *gin.Context) {
		var current model.User
		if err := db.First(&current, user.Id).Error; err != nil {
			t.Fatal(err)
		}
		SetLoginUser(c, &current)
		if err := sessions.Default(c).Save(); err != nil {
			t.Fatal(err)
		}
		c.Status(200)
	})
	r.GET("/private", func(c *gin.Context) {
		if GetLoginUser(c) == nil {
			c.Status(401)
		} else {
			c.Status(200)
		}
	})
	r.POST("/logout", func(c *gin.Context) { ClearSession(c); _ = sessions.Default(c).Save(); c.Status(200) })
	request := func(method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	login := func() *http.Cookie {
		w := request("POST", "/login", nil)
		cookies := w.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatal("login cookie missing")
		}
		return cookies[0]
	}
	old := login()
	var values map[interface{}]interface{}
	if err := securecookie.New([]byte(key), nil).Decode("vpn-ui", old.Value, &values); err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[loginUserKey] != user.Id || values[loginAuthKey] == user.Password {
		t.Fatalf("session exposes credentials: %v", values)
	}
	if request("GET", "/private", old).Code != 200 {
		t.Fatal("fresh session rejected")
	}
	for _, updates := range []map[string]any{
		{"password": "new-password-hash"},
		{"two_factor_enable": true, "two_factor_token": "new-otp-secret"},
	} {
		if err := db.Model(&user).Updates(updates).Error; err != nil {
			t.Fatal(err)
		}
		if request("GET", "/private", old).Code != 401 {
			t.Fatal("credential change did not revoke copied cookie")
		}
		old = login()
	}
	request("POST", "/logout", old)
	if request("GET", "/private", old).Code != 401 {
		t.Fatal("copied cookie still works after logout")
	}
	// Pre-upgrade ID-only cookies cannot bypass the new credential proof.
	legacy := map[interface{}]interface{}{loginUserKey: user.Id}
	encoded, err := securecookie.New([]byte(key), nil).Encode("vpn-ui", legacy)
	if err != nil {
		t.Fatal(err)
	}
	if request("GET", "/private", &http.Cookie{Name: "vpn-ui", Value: encoded}).Code != 401 {
		t.Fatal("legacy cookie bypassed binding")
	}
}
