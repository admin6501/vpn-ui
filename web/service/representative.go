package service

import (
	"encoding/json"
	"errors"
	"github.com/mhsanaei/3x-ui/v2/compat/pasarguard"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
	"strconv"
	"time"
)

// reconcileRepresentatives runs in the same transaction as collected traffic.
// This is a separate block flag: replenishing an admin never re-enables a user
// the operator disabled, and consumption remains after deleting a customer.
func reconcileRepresentatives(tx *gorm.DB) (bool, error) {
	var profiles []model.ResellerProfile
	if err := tx.Where("consumption_based = ?", true).Find(&profiles).Error; err != nil {
		return false, err
	}
	changed := false
	for _, profile := range profiles {
		var admin model.User
		if err := tx.First(&admin, profile.UserId).Error; err != nil {
			return false, err
		}
		var role model.RepresentativeRole
		if err := tx.First(&role, admin.RepresentativeRoleID).Error; err != nil {
			return false, err
		}
		var usage int64
		if err := tx.Raw(`SELECT COALESCE(SUM(MAX(0,ct.all_time-rc.all_time_base)),0)
   FROM reseller_clients rc LEFT JOIN client_traffics ct ON LOWER(TRIM(ct.email))=LOWER(TRIM(rc.email)) WHERE rc.user_id=?`, admin.Id).Scan(&usage).Error; err != nil {
			return false, err
		}
		limited := !profile.Unlimited && profile.AllowanceBytes > 0 && profile.UsageBase+usage >= profile.AllowanceBytes
		blocked := (!admin.Enable && role.DisconnectUsersWhenDisabled) || (limited && role.DisconnectUsersWhenLimited)
		var accounts []model.Account
		if err := tx.Where(`LOWER(TRIM(email)) IN (SELECT LOWER(TRIM(email)) FROM reseller_clients WHERE user_id=?)`, admin.Id).Find(&accounts).Error; err != nil {
			return false, err
		}
		for _, account := range accounts {
			if account.RepresentativeBlocked == blocked {
				continue
			}
			if err := tx.Model(&model.Account{}).Where("id = ?", account.Id).Update("representative_blocked", blocked).Error; err != nil {
				return false, err
			}
			if _, err := (&AccountService{}).ProjectAccount(tx, account.Id); err != nil {
				return false, err
			}
			// Respect the customer's independent quota and expiry. The next existing
			// depletion sweep handles the latter; clearing the block only restores the
			// customer's own Enable, never forces it true.
			if err := tx.Model(&xray.ClientTraffic{}).Where("email = ?", account.Email).Update("enable", account.Enable && !blocked).Error; err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, nil
}

// PasarGuard resets usage every 1/7/30/365 days without extending expiry.
func resetImportedUsage(tx *gorm.DB) (bool, error) {
	var accounts []model.Account
	now := time.Now().UnixMilli()
	if err := tx.Where("imported_reset_days > 0 AND imported_reset_disabled = ?", false).Find(&accounts).Error; err != nil {
		return false, err
	}
	changed := false
	for _, account := range accounts {
		if account.ExpiryTime > 0 && account.ExpiryTime <= now {
			continue
		}
		if now-account.ImportedLastResetAt < int64(account.ImportedResetDays)*86400000 {
			continue
		}
		if err := tx.Model(&model.Account{}).Where("id = ?", account.Id).Updates(map[string]any{"enable": true, "imported_last_reset_at": now}).Error; err != nil {
			return false, err
		}
		if err := tx.Model(&xray.ClientTraffic{}).Where("email = ?", account.Email).Updates(map[string]any{"up": 0, "down": 0, "enable": !account.RepresentativeBlocked}).Error; err != nil {
			return false, err
		}
		if err := tx.Model(&model.AccountInbound{}).Where("account_id = ?", account.Id).Updates(map[string]any{"up": 0, "down": 0}).Error; err != nil {
			return false, err
		}
		if _, err := (&AccountService{}).ProjectAccount(tx, account.Id); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func enforceRepresentativeUserCount(tx *gorm.DB, userID int) error {
	var user model.User
	if err := tx.First(&user, userID).Error; err != nil {
		return err
	}
	if user.RepresentativeRoleID <= 0 {
		return nil
	}
	var role model.RepresentativeRole
	if err := tx.First(&role, user.RepresentativeRoleID).Error; err != nil {
		return err
	}
	var limits pasarguard.Limits
	if role.Limits != "" && json.Unmarshal([]byte(role.Limits), &limits) != nil {
		return errors.New("invalid representative role limits")
	}
	if limits.MaxUsers != nil {
		var count int64
		if err := tx.Model(&model.ResellerClient{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(*limits.MaxUsers) {
			return errors.New("representative has reached the user limit")
		}
	}
	return nil
}

func enforceRoleInboundAccess(role *model.RepresentativeRole, inboundIDs []int) error {
	var access struct {
		AllowedGroups   *[]int `json:"allowed_group_ids"`
		RequireTemplate bool   `json:"require_template"`
	}
	if json.Unmarshal([]byte(role.Access), &access) != nil {
		return errors.New("invalid role access policy")
	}
	if access.RequireTemplate {
		return errors.New("template-required roles need the PasarGuard template adapter")
	}
	if access.AllowedGroups == nil {
		return nil
	}
	allowedGroups := map[int]bool{}
	for _, id := range *access.AllowedGroups {
		allowedGroups[id] = true
	}
	var resources []model.PasarGuardResource
	if err := database.GetDB().Where("kind IN ?", []string{"inbounds", "inbounds_groups_association"}).Find(&resources).Error; err != nil {
		return err
	}
	tags := map[string]string{}
	allowedSourceIDs := map[string]bool{}
	for _, resource := range resources {
		var row map[string]string
		if json.Unmarshal([]byte(resource.Data), &row) != nil {
			return errors.New("invalid imported group mapping")
		}
		if resource.Kind == "inbounds" {
			tags[row["id"]] = row["tag"]
		} else {
			gid, _ := strconv.Atoi(row["group_id"])
			if allowedGroups[gid] {
				allowedSourceIDs[row["inbound_id"]] = true
			}
		}
	}
	allowedTags := map[string]bool{}
	for id := range allowedSourceIDs {
		allowedTags[tags[id]] = true
	}
	for _, id := range inboundIDs {
		var inbound model.Inbound
		if err := database.GetDB().First(&inbound, id).Error; err != nil {
			return err
		}
		if !allowedTags[inbound.Tag] {
			return errors.New("inbound is outside the role's allowed groups")
		}
	}
	return nil
}
