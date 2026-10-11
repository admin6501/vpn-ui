package controller

import (
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
	"net/http"
	"sync"
	"time"
)

type loginBucket struct {
	limiter *rate.Limiter
	last    time.Time
}
type loginLimiter struct {
	mu     sync.Mutex
	ips    map[string]loginBucket
	global *rate.Limiter
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{ips: map[string]loginBucket{}, global: rate.NewLimiter(10, 20)}
}
func (l *loginLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket, found := l.ips[ip]
	if !found {
		if len(l.ips) >= 4096 {
			for key, b := range l.ips {
				if now.Sub(b.last) > 10*time.Minute {
					delete(l.ips, key)
				}
			}
			if len(l.ips) >= 4096 {
				return false
			}
		}
		bucket = loginBucket{limiter: rate.NewLimiter(rate.Every(6*time.Second), 10)}
	}
	bucket.last = now
	l.ips[ip] = bucket
	// Check per-IP first: one source cannot consume the global allowance once blocked.
	return bucket.limiter.AllowN(now, 1) && l.global.AllowN(now, 1)
}
func (l *loginLimiter) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		if !l.allow(c.ClientIP(), time.Now()) {
			c.Header("Retry-After", "6")
			msg := "Too many login attempts. Please try again later."
			if _, ok := c.Get("I18n"); ok {
				if translated := I18nWeb(c, "pages.login.toasts.tooManyAttempts"); translated != "" {
					msg = translated
				}
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"success": false, "msg": msg})
			return
		}
		c.Next()
	}
}
