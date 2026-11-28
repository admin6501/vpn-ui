package service

import (
	"errors"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	err := database.GetDB().Model(&model.User{}).Select("id,username").Where("enable").Order("username").Scan(&rows).Error
	return rows, err
}
func (s *AccountService) TransferOwnership(actor *model.User, accountID, newOwnerID int) error {
	if actor == nil || !actor.IsSuperAdmin {
		return errors.New("only the panel owner can transfer accounts")
	}
	return database.AccountAuditTransaction(database.GetDB(), func(tx *gorm.DB) error {
		var a model.Account
		if err := tx.First(&a, accountID).Error; err != nil {
			return err
		}
		var target model.User
		if err := tx.First(&target, newOwnerID).Error; err != nil {
			return err
		}
		if !target.Enable {
			return errors.New("the selected owner is disabled")
		}
		var old model.ResellerClient
		err := tx.Where("LOWER(TRIM(email)) = ?", accountKey(a.Email)).First(&old).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && old.UserId == newOwnerID {
			return nil
		}
		var panelOwnerID int
		if err := tx.Model(&model.User{}).Where("is_panel_owner").Select("id").Scan(&panelOwnerID).Error; err != nil {
			return err
		}
		unattributed := old.Id == 0 && !a.OwnerConfirmed && a.InferredOwnerID == 0 && (a.OwnerAdminID == 0 || a.OwnerAdminID == panelOwnerID)
		if target.IsRepresentativeAccount() && target.RepresentativeRoleID == 0 && !unattributed {
			return errors.New("legacy prepaid ownership transfer is unsupported")
		}
		// Admin-to-admin transfers change only ownership. Do not touch a disabled
		// account, its usage, credentials, memberships or running core.
		assignWithoutLedger := !target.IsRepresentativeAccount() || target.RepresentativeRoleID == 0
		if assignWithoutLedger && !target.IsSuperAdmin && !target.IsPanelOwner {
			if target.AdminRoleID > 0 && !target.IsRepresentativeAccount() {
				var role model.AdminRole
				if err := tx.First(&role, target.AdminRoleID).Error; err != nil {
					return err
				}
				target.AdminRole = &role
			}
			if !target.Can(model.PermAccessInbounds) {
				return errors.New("the selected owner cannot access clients")
			}
			var ids []int
			if err := tx.Model(&model.AccountInbound{}).Where("account_id=?", accountID).Pluck("inbound_id", &ids).Error; err != nil {
				return err
			}
			if err := checkTrashInboundAccess(tx, &target, ids); err != nil {
				return errors.New("the selected owner needs access to the client's inbounds")
			}
		}
		if old.Id == 0 && assignWithoutLedger {
			if err := tx.Model(&a).Updates(map[string]any{"owner_admin_id": newOwnerID, "owner_confirmed": true}).Error; err != nil {
				return err
			}
			if unattributed {
				return database.RepairLegacyOwnerEvents(tx, accountID, target.Id, target.Username)
			}
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
		ownerID := 0
		if assignWithoutLedger {
			ownerID = target.Id
		}
		ledgerOnlyTransfer := a.OwnerAdminID == ownerID
		if err := tx.Model(&a).Updates(map[string]any{"owner_admin_id": ownerID, "owner_confirmed": true}).Error; err != nil {
			return err
		}
		if !assignWithoutLedger {
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
			if target.GroupAccess {
				ids := []int{}
				if err := tx.Model(&model.AccountGroup{}).Where("account_id = ?", a.Id).Pluck("group_id", &ids).Error; err != nil {
					return err
				}
				for _, gid := range ids {
					if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.RepresentativeGroupAccess{UserID: newOwnerID, GroupID: gid}).Error; err != nil {
						return err
					}
				}
			} else {
				for _, membership := range memberships {
					grant := model.InboundAccess{UserId: newOwnerID, InboundId: membership.InboundId}
					if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant).Error; err != nil {
						return err
					}
				}
			}
			row := model.ResellerClient{Email: a.Email, UserId: newOwnerID, InboundId: home.InboundId, AllTimeBase: ct.AllTime}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		if ledgerOnlyTransfer {
			if err := tx.Create(&model.AccountEvent{AccountID: a.Id, At: time.Now().UnixMilli(),
				Kind: "updated", Changes: "owner", ActorID: actor.Id}).Error; err != nil {
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
		if _, err := reconcileRepresentatives(tx); err != nil {
			return err
		}
		if unattributed {
			return database.RepairLegacyOwnerEvents(tx, accountID, target.Id, target.Username)
		}
		return nil
	}, database.AccountAudit{ActorID: actor.Id})
}
