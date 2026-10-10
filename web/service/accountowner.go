package service

import (
	"errors"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type AccountOwnerChoice struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

func (s *AccountService) OwnershipChoices(actor *model.User) ([]AccountOwnerChoice, error) {
	if actor == nil || !actor.IsSuperAdmin {
		return nil, errors.New("only the panel owner can transfer accounts")
	}
	var rows []AccountOwnerChoice
	err := database.GetDB().Model(&model.User{}).Select("id,username").Where("is_super_admin = ? OR (is_reseller = ? AND representative_role_id > 0)", true, true).Scan(&rows).Error
	return rows, err
}
func (s *AccountService) TransferOwnership(actor *model.User, accountID, newOwnerID int) error {
	if actor == nil || !actor.IsSuperAdmin {
		return errors.New("only the panel owner can transfer accounts")
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		var a model.Account
		if err := tx.First(&a, accountID).Error; err != nil {
			return err
		}
		var target model.User
		if err := tx.First(&target, newOwnerID).Error; err != nil {
			return err
		}
		if !target.IsSuperAdmin && (!target.IsReseller || target.RepresentativeRoleID == 0) {
			return errors.New("target must be a PasarGuard representative or panel owner")
		}
		var old model.ResellerClient
		err := tx.Where("LOWER(TRIM(email)) = ?", accountKey(a.Email)).First(&old).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && old.UserId == newOwnerID {
			return nil
		}
		var ct xray.ClientTraffic
		if err := tx.Where("email = ?", a.Email).First(&ct).Error; err != nil {
			return err
		}
		if old.Id > 0 {
			var p model.ResellerProfile
			if err := tx.First(&p, "user_id = ?", old.UserId).Error; err != nil {
				return err
			}
			if !p.ConsumptionBased {
				return errors.New("legacy prepaid ownership transfer is unsupported")
			}
			delta := ct.AllTime - old.AllTimeBase
			if delta > 0 {
				if err := tx.Model(&p).Update("usage_base", gorm.Expr("usage_base + ?", delta)).Error; err != nil {
					return err
				}
			}
			if err := tx.Delete(&old).Error; err != nil {
				return err
			}
		}
		if !target.IsSuperAdmin {
			if err := enforceRepresentativeUserCount(tx, newOwnerID); err != nil {
				return err
			}
			var p model.ResellerProfile
			if err := tx.First(&p, "user_id = ?", newOwnerID).Error; err != nil {
				return err
			}
			if !p.ConsumptionBased {
				return errors.New("target must use consumption-based accounting")
			}
			var home model.AccountInbound
			tx.Where("account_id = ?", a.Id).Order("inbound_id").First(&home)
			var memberships []model.AccountInbound
			if err := tx.Where("account_id = ?", a.Id).Find(&memberships).Error; err != nil {
				return err
			}
			for _, membership := range memberships {
				grant := model.InboundAccess{UserId: newOwnerID, InboundId: membership.InboundId}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant).Error; err != nil {
					return err
				}
			}
			row := model.ResellerClient{Email: a.Email, UserId: newOwnerID, InboundId: home.InboundId, AllTimeBase: ct.AllTime}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		eligible := a.Enable && (ct.Total == 0 || ct.Up+ct.Down < ct.Total) && (ct.ExpiryTime <= 0 || ct.ExpiryTime > time.Now().UnixMilli())
		if err := tx.Model(&a).Updates(map[string]any{"representative_blocked": false, "enable": eligible}).Error; err != nil {
			return err
		}
		if _, err := s.ProjectAccount(tx, a.Id); err != nil {
			return err
		}
		if err := tx.Model(&ct).Update("enable", eligible).Error; err != nil {
			return err
		}
		_, err = reconcileRepresentatives(tx)
		return err
	})
}
