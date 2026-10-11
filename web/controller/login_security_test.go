package controller

import (
	"encoding/json"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/util/crypto"
	"github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mhsanaei/3x-ui/v2/web/session"
	"github.com/xlzd/gotp"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSecurityLoginLimiterCannotBeBypassedWithForwardedIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := newLoginLimiter()
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.POST("/login", limiter.middleware(), func(c *gin.Context) { c.Status(200) })
	for i := 0; i < 11; i++ {
		req := httptest.NewRequest("POST", "/login", nil)
		req.RemoteAddr = "198.51.100.9:4567"
		req.Header.Set("X-Forwarded-For", string(rune('a'+i)))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if i < 10 && w.Code != 200 {
			t.Fatal("legitimate burst blocked")
		}
		if i == 10 && (w.Code != 429 || w.Header().Get("Retry-After") == "") {
			t.Fatal("spoofed forwarded IP bypassed limiter")
		}
	}
	now := time.Now()
	if limiter.allow("198.51.100.9", now) {
		t.Fatal("exhausted source admitted")
	}
	if !limiter.allow("198.51.100.9", now.Add(7*time.Second)) {
		t.Fatal("limiter does not recover")
	}
}
func TestSecurityWebsocketRejectsForeignPortAndMalformedOrigins(t *testing.T) {
	for _, origin := range []string{"http://panel.test:9090", "http://", "http://[::2]:8080", "null"} {
		req := httptest.NewRequest("GET", "http://panel.test:8080/ws", nil)
		req.Header.Set("Origin", origin)
		if upgrader.CheckOrigin(req) {
			t.Errorf("origin accepted: %q", origin)
		}
	}
}

func TestSecurityCredentialChangesAndAuthenticatorReplacement(t *testing.T) {
	newPermTestDB(t)
	hash, err := crypto.HashPasswordAsBcrypt("current-password")
	if err != nil {
		t.Fatal(err)
	}
	oldSecret := "JBSWY3DPEHPK3PXP"
	newSecret := "KRSXG5DSNFXGOIDB"
	user := model.User{Username: "security-settings-admin", Password: hash, Enable: true, IsSuperAdmin: true, TwoFactorEnable: true, TwoFactorToken: oldSecret}
	db := database.GetDB()
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(sessions.Sessions("vpn-ui", cookie.NewStore([]byte("security-settings-test-key"))))
	r.Use(func(c *gin.Context) {
		c.Set("base_path", "/")
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewIndexController(r.Group("/"))
	NewSettingController(r.Group("/panel"))
	r.GET("/private", func(c *gin.Context) {
		if session.GetLoginUser(c) == nil {
			c.Status(401)
		} else {
			c.Status(200)
		}
	})
	post := func(path string, values url.Values, loginCookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if loginCookie != nil {
			req.AddCookie(loginCookie)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	authenticated := func(loginCookie *http.Cookie) bool {
		req := httptest.NewRequest("GET", "/private", nil)
		req.AddCookie(loginCookie)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code == 200
	}
	success := func(w *httptest.ResponseRecorder) bool {
		var msg struct{ Success bool }
		if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil {
			t.Fatal(err)
		}
		return msg.Success
	}
	freshCookie := func(w *httptest.ResponseRecorder) *http.Cookie {
		cookies := w.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatal("fresh authenticated cookie missing")
		}
		return cookies[0]
	}
	login := post("/login", url.Values{"username": {user.Username}, "password": {"current-password"}, "twoFactorCode": {gotp.NewDefaultTOTP(oldSecret).Now()}}, nil)
	if !success(login) {
		t.Fatal("login failed")
	}
	original := freshCookie(login)
	replacement := post("/panel/setting/twoFactor", url.Values{"enable": {"true"}, "token": {newSecret}, "code": {gotp.NewDefaultTOTP(newSecret).Now()}}, original)
	if success(replacement) {
		t.Fatal("live session replaced existing authenticator without old code")
	}
	var current model.User
	if err := db.First(&current, user.Id).Error; err != nil {
		t.Fatal(err)
	}
	if current.TwoFactorToken != oldSecret {
		t.Fatal("rejected replacement changed authenticator")
	}
	disabled := post("/panel/setting/twoFactor", url.Values{"enable": {"false"}, "code": {gotp.NewDefaultTOTP(oldSecret).Now()}}, original)
	if !success(disabled) {
		t.Fatal("valid authenticator disable failed")
	}
	active := freshCookie(disabled)
	if authenticated(original) || !authenticated(active) {
		t.Fatal("2FA change did not revoke old cookie or keep current browser authenticated")
	}
	enrollment := url.Values{"enable": {"true"}, "token": {newSecret}, "code": {gotp.NewDefaultTOTP(newSecret).Now()}}
	if success(post("/panel/setting/twoFactor", enrollment, active)) {
		t.Fatal("initial enrollment accepted without current password")
	}
	enrollment.Set("password", "current-password")
	enabled := post("/panel/setting/twoFactor", enrollment, active)
	if !success(enabled) {
		t.Fatal("verified enrollment failed")
	}
	beforePassword := freshCookie(enabled)
	changed := post("/panel/setting/updateUser", url.Values{"oldUsername": {user.Username}, "oldPassword": {"current-password"}, "newUsername": {user.Username}, "newPassword": {"new-password"}}, beforePassword)
	if !success(changed) {
		t.Fatal("password change failed")
	}
	afterPassword := freshCookie(changed)
	if authenticated(beforePassword) || !authenticated(afterPassword) {
		t.Fatal("password change did not bind cookie to actual stored credentials")
	}
}

func TestSecurityFailedLoginDoesNotLogPasswords(t *testing.T) {
	newPermTestDB(t)
	r := gin.New()
	r.Use(sessions.Sessions("vpn-ui", cookie.NewStore([]byte("security-log-test"))))
	r.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	NewIndexController(r.Group("/"))
	const secret = "DO-NOT-LOG-TYPED-PASSWORD-658392"
	req := httptest.NewRequest("POST", "/login", strings.NewReader(url.Values{"username": {"nonexistent-security-admin"}, "password": {secret}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(httptest.NewRecorder(), req)
	logs := strings.Join(logger.GetLogs(100, "debug"), "\n")
	if strings.Contains(logs, secret) {
		t.Fatal("failed login leaked submitted password")
	}
	if !strings.Contains(logs, "nonexistent-security-admin") {
		t.Fatal("login attempt was not logged")
	}
}
