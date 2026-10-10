package service

import (
	"errors"
	"github.com/mhsanaei/3x-ui/v2/compat/pasarguard"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
	"time"
)

// attachImportedSubscriptionTokens runs only on rows already scoped and paged
// for the caller. Native IDs stay intact for edits, rotation and compatibility.
func attachImportedSubscriptionTokens(db *gorm.DB, rows []AccountRow) error {
	if len(rows) == 0 {
		return nil
	}
	var key model.PasarGuardSecret
	err := db.First(&key, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	ids := make([]int, 0, len(rows))
	byID := map[int]*AccountRow{}
	for i := range rows {
		ids = append(ids, rows[i].Id)
		byID[rows[i].Id] = &rows[i]
	}
	var bindings []model.PasarGuardSubscription
	if err := db.Where("account_id IN ?", ids).Find(&bindings).Error; err != nil {
		return err
	}
	for _, binding := range bindings {
		row := byID[binding.AccountID]
		if row == nil || row.SubID == "" || row.SubID != binding.SubIDAtImport {
			continue
		}
		revoked := time.Time{}
		if binding.RevokedAt != nil {
			revoked = *binding.RevokedAt
		}
		token, err := pasarguard.DisplayToken(binding.SourceUserID, binding.CreatedAt, revoked, key.SigningKey)
		if err == nil {
			row.LegacySubID = token
		}
	}
	return nil
}
