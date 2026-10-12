// Package middleware provides HTTP middleware functions for the vpn-ui web panel.
package middleware

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// canonicalRequestDomain compares DNS names independently of case and their
// optional final root dot, and IP literals independently of their spelling.
// A port may accompany the host. URLs, userinfo and empty hosts fail closed.
func canonicalRequestDomain(value string) string {
	value = strings.TrimSpace(value)
	if strings.ContainsAny(value, "/\\?#@") {
		return ""
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	u, err := url.Parse("//" + value)
	if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" {
		return ""
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

func DomainValidatorMiddleware(domain string) gin.HandlerFunc {
	allowed := canonicalRequestDomain(domain)
	return func(c *gin.Context) {
		host := canonicalRequestDomain(c.Request.Host)
		if allowed == "" || host == "" || host != allowed {
			c.Header("Cache-Control", "no-store")
			c.Header("X-VPN-UI-Error", "domain-rejected")
			c.AbortWithStatus(http.StatusForbidden)
			return
		}
		c.Next()
	}
}
