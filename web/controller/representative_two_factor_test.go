package controller

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/util/crypto"
	"github.com/mhsanaei/3x-ui/v2/web/locale"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/mhsanaei/3x-ui/v2/web/session"
	"github.com/xlzd/gotp"
)

// Exercise actual login cookies and both sibling route trees, with a non-root base path.
func TestRepresentativePersonalTwoFactorAndRequiredEnrollment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newPermTestDB(t)
	db := database.GetDB()
	hash, err := crypto.HashPasswordAsBcrypt("representative-password")
	if err != nil {
		t.Fatal(err)
	}
	rep := model.User{Username: "personal-2fa-rep", Password: hash, Enable: true, IsReseller: true}
	victim := model.User{Username: "unrelated-admin", Password: hash, Enable: true, IsSuperAdmin: true}
	for _, u := range []*model.User{&rep, &victim} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	r := gin.New()
	r.Use(sessions.Sessions("vpn-ui", cookie.NewStore([]byte("representative-two-factor-test"))))
	r.Use(func(c *gin.Context) {
		c.Set("base_path", "/private/")
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		c.Next()
	})
	r.SetHTMLTemplate(template.Must(template.New("security.html").Parse(`{{.two_factor}}/{{.two_factor_required}}`)))
	root := r.Group("/private")
	NewIndexController(root)
	NewXUIController(root)
	// Use the production API auth middleware without starting server cron jobs.
	api := root.Group("/panel/api", (&APIController{}).checkAPIAuth)
	NewClientsController(api.Group("/clients", requirePerm(model.PermAccessInbounds)))
	NewInboundController(api.Group("/inbounds"))
	r.GET("/private/ws", NewWebSocketController(nil).HandleWebSocket)
	call := func(method, path string, values url.Values, ck *http.Cookie, html bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/private"+path, strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if html {
			req.Header.Set("Accept", "text/html")
		} else {
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
		}
		if ck != nil {
			req.AddCookie(ck)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	success := func(w *httptest.ResponseRecorder) bool {
		t.Helper()
		var msg struct{ Success bool }
		if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil {
			t.Fatal(w.Body.String(), err)
		}
		return msg.Success
	}
	cookieFrom := func(w *httptest.ResponseRecorder) *http.Cookie {
		t.Helper()
		if len(w.Result().Cookies()) != 1 {
			t.Fatal("missing session cookie", w.Body.String())
		}
		return w.Result().Cookies()[0]
	}
	login := func(code string) *httptest.ResponseRecorder {
		return call("POST", "/login", url.Values{"username": {rep.Username}, "password": {"representative-password"}, "twoFactorCode": {code}}, nil, false)
	}
	if got := call("POST", "/panel/setting/twoFactor", nil, nil, false); got.Code != 401 {
		t.Fatal("anonymous enrollment accepted", got.Body.String())
	}
	original := cookieFrom(login(""))
	if got := call("GET", "/panel/security", nil, original, true); got.Code != 200 || !strings.Contains(got.Body.String(), "false/false") {
		t.Fatal("personal security denied", got.Body.String())
	}
	for _, path := range []string{"all", "update", "updateUser"} {
		if success(call("POST", "/panel/setting/"+path, nil, original, false)) {
			t.Fatal("shared settings opened", path)
		}
	}
	// A newly imposed policy applies immediately to existing sessions.
	if err := db.Model(&rep).Update("two_factor_required", true).Error; err != nil {
		t.Fatal(err)
	}
	if got := call("GET", "/panel/clients", nil, original, true); got.Code != 307 || got.Header().Get("Location") != "/private/panel/security" {
		t.Fatal("missing enrollment redirect", got.Code, got.Body.String())
	}
	for _, path := range []string{"/panel/api/clients/list", "/panel/api/inbounds/list"} {
		got := call("GET", path, nil, original, false)
		if got.Code != 403 || !strings.Contains(got.Body.String(), `"twoFactorEnrollmentRequired":true`) {
			t.Fatal("API bypassed required enrollment", path, got.Body.String())
		}
	}
	if got := call("GET", "/ws", nil, original, false); got.Code != 401 {
		t.Fatal("websocket bypassed enrollment", got.Code)
	}
	if got := session.CurrentUserSession(&rep); got != nil {
		t.Fatal("pending account retained long-lived data access")
	}
	if got := call("GET", "/panel/security", nil, original, true); got.Code != 200 || !strings.Contains(got.Body.String(), "false/true") {
		t.Fatal("enrollment page unavailable", got.Body.String())
	}
	const secret = "JBSWY3DPEHPK3PXP"
	form := url.Values{"enable": {"true"}, "token": {secret}, "code": {gotp.NewDefaultTOTP(secret).Now()}, "userId": {"1"}}
	if success(call("POST", "/panel/setting/twoFactor", form, original, false)) {
		t.Fatal("enrollment accepted without password")
	}
	form.Set("password", "representative-password")
	form.Set("code", "invalid")
	if success(call("POST", "/panel/setting/twoFactor", form, original, false)) {
		t.Fatal("enrollment accepted invalid code")
	}
	form.Set("code", gotp.NewDefaultTOTP(secret).Now())
	enrolled := call("POST", "/panel/setting/twoFactor", form, original, false)
	if !success(enrolled) {
		t.Fatal("enrollment failed", enrolled.Body.String())
	}
	active := cookieFrom(enrolled)
	if got := call("GET", "/panel/security", nil, original, false); got.Code != 401 {
		t.Fatal("old cookie survived enrollment")
	}
	if got := call("GET", "/panel/security", nil, active, true); got.Code != 200 || !strings.Contains(got.Body.String(), "true/true") {
		t.Fatal("current browser lost enrollment session", got.Body.String())
	}
	if success(login("")) {
		t.Fatal("login bypassed authenticator")
	}
	if !success(login(gotp.NewDefaultTOTP(secret).Now())) {
		t.Fatal("authenticator login failed")
	}
	disable := url.Values{"enable": {"false"}, "code": {gotp.NewDefaultTOTP(secret).Now()}}
	if success(call("POST", "/panel/setting/twoFactor", disable, active, false)) {
		t.Fatal("representative disabled required 2FA")
	}
	var svc service.UserService
	if err := svc.SetTwoFactor(rep.Id, false, ""); err == nil {
		t.Fatal("service bypassed required policy")
	}
	// Removing the manager's requirement preserves the enrolled authenticator.
	if err := db.Model(&rep).Update("two_factor_required", false).Error; err != nil {
		t.Fatal(err)
	}
	var stored model.User
	db.First(&stored, rep.Id)
	if !stored.TwoFactorEnable || stored.TwoFactorToken != secret {
		t.Fatal("removing policy cleared enrollment")
	}
	disable.Set("code", "invalid")
	if success(call("POST", "/panel/setting/twoFactor", disable, active, false)) {
		t.Fatal("optional disable accepted invalid code")
	}
	disable.Set("code", gotp.NewDefaultTOTP(secret).Now())
	if !success(call("POST", "/panel/setting/twoFactor", disable, active, false)) {
		t.Fatal("optional disable failed")
	}
	stored = model.User{}
	if err := db.First(&stored, victim.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TwoFactorEnable || stored.TwoFactorToken != "" {
		t.Fatal("personal enrollment mutated another account")
	}
}

func TestRepresentativeTwoFactorPolicyOwnershipAndCompatibility(t *testing.T) {
	f := newResellerFixture(t)
	caller := f.ali
	r := gin.New()
	r.Use(func(c *gin.Context) { withUser(caller)(c) })
	NewResellerController(r.Group("/panel"))
	update := func(extra string) bool {
		req := httptest.NewRequest("POST", "/panel/resellers/update/"+strconv.Itoa(f.sara.Id), strings.NewReader("username=sara&enable=true"+extra))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var msg struct{ Success bool }
		if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil {
			t.Fatal(err)
		}
		return msg.Success
	}
	if !update("&twoFactorRequired=true") {
		t.Fatal("manager could not require 2FA")
	}
	if !update("") {
		t.Fatal("legacy edit rejected")
	}
	var stored model.User
	database.GetDB().First(&stored, f.sara.Id)
	if !stored.TwoFactorRequired {
		t.Fatal("omitted policy cleared requirement")
	}
	caller = f.reza
	if update("&twoFactorRequired=false") {
		t.Fatal("another manager changed policy")
	}
	caller = f.sara
	if update("&twoFactorRequired=false") {
		t.Fatal("representative changed own policy")
	}
	caller = f.ali
	if !update("&twoFactorRequired=false") {
		t.Fatal("manager could not remove policy")
	}
	database.GetDB().First(&stored, f.sara.Id)
	if stored.TwoFactorRequired {
		t.Fatal("false policy not saved")
	}
}
