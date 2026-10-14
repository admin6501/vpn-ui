// Package session provides session management utilities for the vpn-ui web panel.
// It handles user authentication state, login sessions, and session storage using Gin sessions.
package session

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/middleware"
	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	loginUserKey = "LOGIN_USER"
	loginAuthKey = "LOGIN_AUTH"
	// loginUserCtxKey caches the loaded user for the life of one request, so the
	// several handlers that ask for it don't each re-query.
	loginUserCtxKey = "LOGIN_USER_ROW"
)

// The session cookie holds the user ID and a credential binding, never the user row.
//
// The store is signed but not encrypted (cookie.NewStore -> securecookie with a
// nil blockKey), so its gob body is readable in the browser with base64 -d. It
// used to carry the whole model.User, which put the admin's bcrypt hash in every
// browser and would have put their TOTP secret there too.
//
// Loading the row per request also means permission changes and account disables
// take effect immediately instead of lingering until the cookie's 6h expiry, and
// it sidesteps securecookie's 4096-byte cap.
//
// Upgrade note: cookies without the credential binding require one fresh login.
// Legacy cookies carrying a whole User are also rejected.

// SetLoginUser stores the authenticated user's id in the session.
func SetLoginUser(c *gin.Context, user *model.User) {
	if user == nil {
		return
	}
	s := sessions.Default(c)
	s.Clear()
	s.Set(loginUserKey, user.Id)
	s.Set(loginAuthKey, authBinding(user))
	c.Set(loginUserCtxKey, user)
}

// A digest binds the signed cookie to current credentials and revocation state.
// Neither bcrypt hashes nor TOTP secrets are carried in the browser cookie.
func authBinding(user *model.User) string {
	raw, _ := json.Marshal([]any{user.Id, user.Username, user.Password, user.TwoFactorEnable, user.TwoFactorToken, user.SessionVersion})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

// loginUserId returns the session's user id, or 0 when absent or unreadable.
func loginUserId(c *gin.Context) int {
	s := sessions.Default(c)
	obj := s.Get(loginUserKey)
	if obj == nil {
		return 0
	}
	id, ok := obj.(int)
	if !ok || id <= 0 {
		// Garbage, or a pre-upgrade cookie holding a gob'd model.User.
		s.Delete(loginUserKey)
		return 0
	}
	return id
}

// GetLoginUser returns the CURRENT row for the logged-in admin, read fresh from
// the database rather than trusted from the cookie. Returns nil when logged out,
// when the account has been deleted, or when it has been disabled.
func GetLoginUser(c *gin.Context) *model.User {
	if cached, exists := c.Get(loginUserCtxKey); exists {
		user, _ := cached.(*model.User)
		return user
	}
	user := loadLoginUser(c)
	c.Set(loginUserCtxKey, user)
	return user
}

func loadLoginUser(c *gin.Context) *model.User {
	id := loginUserId(c)
	if id == 0 {
		return nil
	}
	db := database.GetDB()
	if db == nil {
		return nil
	}
	user := &model.User{}
	if err := db.Model(model.User{}).Where("id = ?", id).First(user).Error; err != nil {
		return nil // deleted mid-session, or the DB is unavailable
	}
	proof, ok := sessions.Default(c).Get(loginAuthKey).(string)
	if !ok || subtle.ConstantTimeCompare([]byte(proof), []byte(authBinding(user))) != 1 {
		return nil
	}
	if !user.Enable {
		return nil // disabled admins are logged out on their next request
	}
	if user.AdminRoleID > 0 && !user.IsSuperAdmin && !user.IsReseller {
		var role model.AdminRole
		if err := db.First(&role, user.AdminRoleID).Error; err != nil {
			// An owner can repair an invalid assignment; operational permissions stay empty.
			if !user.IsPanelOwner {
				return nil
			}
		} else {
			user.AdminRole = &role
		}
	}
	if user.RepresentativeRoleID > 0 && !user.IsPanelOwner && (user.AdminRoleID == 0 || user.IsReseller) {
		var role model.RepresentativeRole
		if db.First(&role, user.RepresentativeRoleID).Error != nil {
			return nil
		}
		user.RepresentativeRole = &role
		var profile model.ResellerProfile
		if user.IsReseller {
			if db.Where("user_id = ?", user.Id).First(&profile).Error != nil {
				return nil
			}
			if profile.ConsumptionBased {
				var usage int64
				if err := db.Raw(`SELECT COALESCE(SUM(MAX(0, ct.all_time-rc.all_time_base)),0)
				FROM reseller_clients rc LEFT JOIN client_traffics ct ON LOWER(TRIM(ct.email))=LOWER(TRIM(rc.email))
				WHERE rc.user_id=?`, user.Id).Scan(&usage).Error; err != nil {
					return nil
				}
				user.RepresentativeLimited = !profile.Unlimited && profile.AllowanceBytes > 0 && profile.UsageBase+usage >= profile.AllowanceBytes
			}
		}
	}
	return user
}

// IsLogin checks if a user is currently authenticated in the session.
// Returns true if a valid user session exists, false otherwise.
func IsLogin(c *gin.Context) bool {
	return GetLoginUser(c) != nil
}

// ClearSession removes all session data and invalidates the session.
// This effectively logs out the user and clears any stored session information.
func ClearSession(c *gin.Context) {
	if user := GetLoginUser(c); user != nil {
		// Revokes copied cookies too, including other browsers for this account.
		if err := database.GetDB().Model(&model.User{}).Where("id = ?", user.Id).
			UpdateColumn("session_version", gorm.Expr("session_version + 1")).Error; err != nil {
			logger.Warning("Unable to revoke login sessions: ", err)
		}
	}
	c.Set(loginUserCtxKey, (*model.User)(nil))
	s := sessions.Default(c)
	s.Clear()
	cookiePath := c.GetString("base_path")
	if cookiePath == "" {
		cookiePath = "/"
	}
	s.Options(sessions.Options{
		Path:     cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   middleware.IsSecureRequest(c.Request),
		SameSite: http.SameSiteLaxMode,
	})
}

// IsCurrentUserSession revalidates a captured login for long-lived connections.
// The snapshot is taken at authentication; changes must not inherit new credentials.
func IsCurrentUserSession(snapshot *model.User) bool {
	return CurrentUserSession(snapshot) != nil
}

// CurrentUserSession returns fresh permissions for a long-lived connection,
// retaining the original credential binding. Revocation applies before delivery.
func CurrentUserSession(snapshot *model.User) *model.User {
	if snapshot == nil || database.GetDB() == nil {
		return nil
	}
	db := database.GetDB()
	var current model.User
	if db.First(&current, snapshot.Id).Error != nil || !current.Enable {
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(authBinding(snapshot)), []byte(authBinding(&current))) != 1 {
		return nil
	}
	if current.AdminRoleID > 0 && !current.IsSuperAdmin && !current.IsReseller {
		var role model.AdminRole
		if db.First(&role, current.AdminRoleID).Error != nil {
			if !current.IsPanelOwner {
				return nil
			}
		} else {
			current.AdminRole = &role
		}
	}
	if current.RepresentativeRoleID > 0 && current.IsRepresentativeAccount() {
		var role model.RepresentativeRole
		if db.First(&role, current.RepresentativeRoleID).Error != nil {
			return nil
		}
		current.RepresentativeRole = &role
	}
	return &current
}
