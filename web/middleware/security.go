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
func BrowserOriginProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Explicit foreign origins are rejected even on GET (WebSocket and sensitive
		// read endpoints). Fetch metadata also closes cross-site navigations without Origin.
		if !SameOrigin(c.Request) || (c.GetHeader("Sec-Fetch-Site") == "cross-site" &&
			(c.Request.Method != "GET" && c.Request.Method != "HEAD" || strings.Contains(c.Request.URL.Path, "/panel/") || strings.HasSuffix(c.Request.URL.Path, "/ws") || strings.HasSuffix(c.Request.URL.Path, "/logout"))) {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}
