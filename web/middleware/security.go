package middleware

import (
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// Only explicitly trusted proxies may supply client IPs or the original TLS scheme.
func TrustedProxies() []string {
	if value, ok := os.LookupEnv("VPN_UI_TRUSTED_PROXIES"); ok {
		var proxies []string
		for _, value := range strings.Split(value, ",") {
			if value = strings.TrimSpace(value); value != "" {
				proxies = append(proxies, value)
			}
		}
		return proxies
	}
	return nil
}
func IsSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if r.Header.Get("X-Forwarded-Proto") != "https" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, proxy := range TrustedProxies() {
		if proxyIP := net.ParseIP(proxy); proxyIP != nil && proxyIP.Equal(ip) {
			return true
		}
		if _, network, err := net.ParseCIDR(proxy); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}
func originPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

// Browser origins contain only a scheme and authority. Never strip the port or
// accept empty hosts/userinfo: another application on this host is another origin.
func SameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	} // CLI and credential-authenticated agents.
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	scheme := "http"
	if IsSecureRequest(r) {
		scheme = "https"
	}
	target, err := url.Parse(scheme + "://" + r.Host)
	return err == nil && target.Hostname() != "" && target.User == nil && u.Scheme == scheme && strings.EqualFold(u.Hostname(), target.Hostname()) && originPort(u) == originPort(target)
}

// intentionalPanelNavigation permits only a user-initiated top-level GET to a
// registered HTML page. APIs, logout, WebSocket, iframes and background fetches
// remain protected. FullPath is the matched route pattern, not a user-controlled
// suffix that could disguise an API request as a page.
func intentionalPanelNavigation(c *gin.Context, basePath string) bool {
	r := c.Request
	if r.Method != http.MethodGet || r.Header.Get("Sec-Fetch-Mode") != "navigate" ||
		r.Header.Get("Sec-Fetch-Dest") != "document" || r.Header.Get("Sec-Fetch-User") != "?1" {
		return false
	}
	for _, page := range []string{
		"/panel/", "/panel/inbounds", "/panel/clients", "/panel/settings",
		"/panel/xray", "/panel/core", "/panel/admins", "/panel/resellers",
		"/panel/nodes/", "/panel/nodes/:id/xray",
	} {
		if c.FullPath() == strings.TrimSuffix(basePath, "/")+page {
			return true
		}
	}
	return false
}

func BrowserOriginProtection(basePaths ...string) gin.HandlerFunc {
	basePath := "/"
	if len(basePaths) > 0 && basePaths[0] != "" {
		basePath = strings.TrimRight(basePaths[0], "/") + "/"
	}
	return func(c *gin.Context) {
		// An external link may retain cross-site metadata across the login-to-panel
		// redirect when a session exists. That navigation must reach the ordinary
		// login/permission guards; it grants no authentication or authorization.
		crossSite := c.GetHeader("Sec-Fetch-Site") == "cross-site"
		protected := c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead ||
			strings.Contains(c.Request.URL.Path, "/panel/") ||
			strings.HasSuffix(c.Request.URL.Path, "/ws") || strings.HasSuffix(c.Request.URL.Path, "/logout")
		if !SameOrigin(c.Request) || (crossSite && protected && !intentionalPanelNavigation(c, basePath)) {
			c.Header("Cache-Control", "no-store")
			c.Header("X-VPN-UI-Error", "origin-rejected")
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}
