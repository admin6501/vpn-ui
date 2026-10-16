package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityUserNavigationDoesNotBypassAPIsOrPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(BrowserOriginProtection("/private/"))
	for _, path := range []string{"/private/panel/", "/private/panel/clients", "/private/panel/nodes/", "/private/panel/nodes/:id/xray", "/private/panel/api/test", "/private/panel/api/alias/panel/clients", "/private/ws", "/private/logout"} {
		router.Any(path, func(c *gin.Context) {
			if c.GetHeader("Authorization") != "test-session" {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			c.Status(http.StatusOK)
		})
	}
	for _, tc := range []struct {
		name, method, path, mode, dest, user, origin, auth string
		want                                               int
	}{
		{"overview", "GET", "/private/panel/", "navigate", "document", "?1", "", "test-session", 200},
		{"client-link", "GET", "/private/panel/clients", "navigate", "document", "?1", "", "test-session", 200},
		{"node-link", "GET", "/private/panel/nodes/", "navigate", "document", "?1", "", "test-session", 200},
		{"node-editor", "GET", "/private/panel/nodes/3/xray", "navigate", "document", "?1", "", "test-session", 200},
		{"anonymous-still-denied", "GET", "/private/panel/clients", "navigate", "document", "?1", "", "", 401},
		{"iframe", "GET", "/private/panel/clients", "navigate", "iframe", "?1", "", "test-session", 403},
		{"fetch", "GET", "/private/panel/clients", "cors", "empty", "", "", "test-session", 403},
		{"app-navigation-without-activation", "GET", "/private/panel/clients", "navigate", "document", "", "", "test-session", 200},
		{"no-activation-still-needs-session", "GET", "/private/panel/clients", "navigate", "document", "", "", "", 401},
		{"no-mode", "GET", "/private/panel/clients", "", "document", "", "", "test-session", 403},
		{"no-destination", "GET", "/private/panel/clients", "navigate", "", "", "", "test-session", 403},
		{"API-even-as-document", "GET", "/private/panel/api/test", "navigate", "document", "?1", "", "test-session", 403},
		{"alias-cannot-disguise-API", "GET", "/private/panel/api/alias/panel/clients", "navigate", "document", "?1", "", "test-session", 403},
		{"logout", "GET", "/private/logout", "navigate", "document", "?1", "", "test-session", 403},
		{"websocket", "GET", "/private/ws", "navigate", "document", "?1", "", "test-session", 403},
		{"foreign-origin", "GET", "/private/panel/clients", "navigate", "document", "?1", "http://evil.test", "test-session", 403},
		{"POST-navigation", "POST", "/private/panel/clients", "navigate", "document", "?1", "", "test-session", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://panel.test"+tc.path, nil)
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.Header.Set("Sec-Fetch-Mode", tc.mode)
			req.Header.Set("Sec-Fetch-Dest", tc.dest)
			req.Header.Set("Sec-Fetch-User", tc.user)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d", w.Code, tc.want)
			}
			if tc.want == 403 && (w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-VPN-UI-Error") != "origin-rejected") {
				t.Fatal("uncacheable denial reason missing")
			}
		})
	}
}

func TestSecurityDomainCanonicalizationDoesNotWidenHosts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		domain, host string
		want         int
	}{
		{"Panel.Example.", "panel.example:34587", 200},
		{"panel.example", "PANEL.EXAMPLE.:34587", 200},
		{"panel.example", "evil.example:34587", 403},
		{"panel.example", "panel.example.evil.test:34587", 403},
		{"panel.example", "attacker@panel.example:34587", 403},
		{"panel.example", "panel.example/path", 403},
		{"panel.example", "panel.example?", 403},
		{"panel.example", "panel.example#", 403},
		{"panel.example", "panel.example:bad", 403},
		{"panel.example", "", 403},
		{"http://panel.example", "panel.example", 403},
		{"::1", "[::1]:34587", 200},
		{"[2001:db8::1]", "[2001:0db8:0:0:0:0:0:1]:34587", 200},
		{"::1", "[::2]:34587", 403},
	} {
		r := gin.New()
		r.Use(DomainValidatorMiddleware(tc.domain))
		r.GET("/", func(c *gin.Context) { c.Status(200) })
		req := httptest.NewRequest("GET", "http://panel.test/", nil)
		req.Host = tc.host
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("domain=%q host=%q got %d want %d", tc.domain, tc.host, w.Code, tc.want)
		}
		if tc.want == 403 && w.Header().Get("X-VPN-UI-Error") != "domain-rejected" {
			t.Fatal("domain denial reason missing")
		}
	}
}
