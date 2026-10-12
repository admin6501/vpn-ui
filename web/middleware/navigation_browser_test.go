package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSecurityBrowserPanelNavigationRuntime(t *testing.T) {
	if os.Getenv("RUN_NODE_UI_BROWSER") != "1" {
		t.Skip("enable real Chromium navigation checks")
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Header("X-Test-Fetch-Site", c.GetHeader("Sec-Fetch-Site")); c.Next() })
	r.Use(BrowserOriginProtection("/private/"), DomainValidatorMiddleware("127.0.0.1"))
	loggedIn := func(c *gin.Context) bool {
		value, _ := c.Cookie("navigation-session")
		return value == "test-only-session"
	}
	r.GET("/private/", func(c *gin.Context) {
		if loggedIn(c) {
			c.Redirect(http.StatusTemporaryRedirect, "/private/panel/")
			return
		}
		c.String(200, "Login")
	})
	r.GET("/private/test-session", func(c *gin.Context) {
		http.SetCookie(c.Writer, &http.Cookie{Name: "navigation-session", Value: "test-only-session", Path: "/private/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		c.String(200, "Test session")
	})
	for _, path := range []string{"/private/panel/", "/private/panel/clients"} {
		r.GET(path, func(c *gin.Context) {
			if !loggedIn(c) {
				c.Redirect(307, "/private/")
				return
			}
			c.String(200, "Authenticated panel")
		})
	}
	panel := httptest.NewServer(r)
	defer panel.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<a id="root" href="%s/private/">Panel link</a><a id="clients" href="%s/private/panel/clients">Clients link</a>`, panel.URL, panel.URL)
	}))
	defer source.Close()
	script, err := filepath.Abs("../../test_unit/panel_navigation_browser_test.cjs")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", script)
	cmd.Env = append(os.Environ(), "PANEL_NAV_URL="+panel.URL, "PANEL_NAV_SOURCE="+strings.Replace(source.URL, "127.0.0.1", "localhost", 1))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser navigation: %v\n%s", err, out)
	}
}
