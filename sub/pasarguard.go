package sub

import (
	"github.com/gin-gonic/gin"
	"github.com/mhsanaei/3x-ui/v2/compat/pasarguard"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"time"
)

// resolvePasarGuardSubscription leaves native subIds alone. Imported token
// requests are resolved to the account's CURRENT subId; rotating that subId
// revokes the legacy binding until an operator explicitly rebinds it.
func resolvePasarGuardSubscription(c *gin.Context) {
	token := c.Param("subid")
	db := database.GetDB()
	if db == nil {
		c.AbortWithStatus(503)
		return
	}
	// Native tokens always win; a legacy token is never a second route to a native
	// account unless its signature and imported identity both match.
	var native int64
	if err := db.Model(&model.Account{}).Where("sub_id = ?", token).Count(&native).Error; err != nil {
		c.AbortWithStatus(503)
		return
	}
	if native > 0 {
		c.Next()
		return
	}
	var key model.PasarGuardSecret
	if err := db.First(&key, 1).Error; err != nil {
		c.AbortWithStatus(404)
		return
	}
	payload, err := pasarguard.Verify(token, key.SigningKey, time.Now())
	if err != nil {
		c.AbortWithStatus(404)
		return
	}
	var binding model.PasarGuardSubscription
	q := db.Model(&binding)
	if payload.UserID > 0 {
		q = q.Where("source_user_id = ?", payload.UserID)
	} else {
		q = q.Where("username = ?", payload.Username)
	}
	if q.First(&binding).Error != nil {
		c.AbortWithStatus(404)
		return
	}
	revoked := time.Time{}
	if binding.RevokedAt != nil {
		revoked = *binding.RevokedAt
	}
	if !payload.ValidAfter(binding.CreatedAt, revoked) {
		c.AbortWithStatus(404)
		return
	}
	var account model.Account
	if db.First(&account, binding.AccountID).Error != nil || account.SubID == "" || account.SubID != binding.SubIDAtImport {
		c.AbortWithStatus(404)
		return
	}
	for i := range c.Params {
		if c.Params[i].Key == "subid" {
			c.Params[i].Value = account.SubID
		}
	}
	c.Next()
}
