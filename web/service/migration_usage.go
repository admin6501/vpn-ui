package service

import (
	"errors"
	"math"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

// ImportMigrationUsage initializes aggregate counters on a disabled, unused
// destination account. The imported bytes cannot be attributed to a destination
// inbound because source and destination groups may use different protocols.
// A repeated identical request is safe; any other existing usage is preserved.
func ImportMigrationUsage(email string, up, down, lifetime int64) error {
	if accountKey(email) == "" || up < 0 || down < 0 || lifetime < 0 || up > math.MaxInt64-down {
		return errors.New("invalid migration usage")
	}
	total := up + down
	if lifetime < total {
		lifetime = total
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		var account model.Account
		if err := tx.Where("LOWER(TRIM(email)) = ?", accountKey(email)).First(&account).Error; err != nil {
			return err
		}
		if account.Enable {
			return errors.New("destination account must be disabled")
		}
		var traffic xray.ClientTraffic
		if err := tx.Where("LOWER(TRIM(email)) = ?", accountKey(email)).First(&traffic).Error; err != nil {
			return err
		}
		if traffic.Enable {
			return errors.New("destination traffic must be disabled")
		}
		var usedMemberships int64
		if err := tx.Model(&model.AccountInbound{}).Where("account_id = ? AND (up != 0 OR down != 0 OR all_time != 0)", account.Id).Count(&usedMemberships).Error; err != nil {
			return err
		}
		if usedMemberships != 0 {
			return errors.New("destination membership already has usage")
		}
		if traffic.Up == up && traffic.Down == down && traffic.AllTime == lifetime {
			return nil
		}
		if traffic.Up != 0 || traffic.Down != 0 || traffic.AllTime != 0 {
			return errors.New("destination account already has usage")
		}
		return tx.Model(&traffic).Updates(map[string]any{"up": up, "down": down, "all_time": lifetime}).Error
	})
}
