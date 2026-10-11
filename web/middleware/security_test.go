package middleware

import (
	"crypto/tls"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestSecurityOriginValidation(t *testing.T) {
	t.Setenv("VPN_UI_TRUSTED_PROXIES", "")
	for _, tc := range []struct {
		target, origin string
		want           bool
	}{
		{"http://panel.test:8080/", "http://panel.test:8080", true},
		{"http://panel.test:8080/", "http://panel.test:9090", false},
		{"http://panel.test/", "http://panel.test:80", true},
		{"http://panel.test/", "http://evil.test", false},
		{"http://panel.test/", "http://", false},
		{"http://panel.test/", "null", false},
		{"http://panel.test/", "http://attacker@panel.test", false},
		{"http://panel.test/", "http://panel.test/evil", false},
		{"http://panel.test/", "https://panel.test", false},
		{"http://[::1]:8080/", "http://[::1]:8080", true},
		{"http://[::1]:8080/", "http://[::2]:8080", false},
		{"http://panel.test/", "", true},
	} {
		r := httptest.NewRequest("POST", tc.target, nil)
		r.Header.Set("Origin", tc.origin)
		if got := SameOrigin(r); got != tc.want {
			t.Errorf("%s from %q = %v", tc.target, tc.origin, got)
		}
	}
}
func TestSecurityProxyHeadersCannotSpoofTLSOrClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("VPN_UI_TRUSTED_PROXIES", "127.0.0.0/8,::1/128")
	r := gin.New()
	if err := r.SetTrustedProxies(TrustedProxies()); err != nil {
		t.Fatal(err)
	}
	r.GET("/", func(c *gin.Context) { c.String(200, c.ClientIP()) })
	req := httptest.NewRequest("GET", "http://panel.test/", nil)
	req.RemoteAddr = "198.51.100.9:1234"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	req.Header.Set("X-Real-IP", "127.0.0.1")
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Body.String() != "198.51.100.9" || IsSecureRequest(req) {
		t.Fatal("untrusted proxy headers accepted")
	}
	req.RemoteAddr = "127.0.0.1:1234"
	if !IsSecureRequest(req) {
		t.Fatal("trusted local TLS proxy rejected")
	}
	req.Header.Del("X-Forwarded-Proto")
	req.TLS = &tls.ConnectionState{}
	if !IsSecureRequest(req) {
		t.Fatal("native TLS rejected")
	}
}
func TestSecurityBrowserOriginProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BrowserOriginProtection())
	r.Any("/panel/api/test", func(c *gin.Context) { c.Status(200) })
	for _, tc := range []struct {
		method, origin, site string
		want                 int
	}{
		{"POST", "http://evil.test", "", 403},
		{"POST", "", "cross-site", 403},
		{"GET", "", "cross-site", 403},
		{"POST", "http://panel.test", "same-origin", 200},
		{"POST", "", "", 200},
	} {
		req := httptest.NewRequest(tc.method, "http://panel.test/panel/api/test", nil)
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Sec-Fetch-Site", tc.site)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%+v = %d", tc, w.Code)
		}
	}
}
