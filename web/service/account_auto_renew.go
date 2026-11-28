package service

import (
	"errors"
	"sort"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/util/common"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

func (s *AccountService) SetAutoRenew(email string, enabled bool, audit ...database.AccountAudit) error {
	return s.ConfigureAutoRenew(email, enabled, nil, audit...)
}

const MaxAutoRenewLimit = 10000

var ErrInvalidAutoRenewLimit = errors.New("invalid automatic renewal limit")

func (s *AccountService) ConfigureAutoRenew(email string, enabled bool, limit *int, audit ...database.AccountAudit) error {
	if limit != nil && (*limit < 0 || *limit > MaxAutoRenewLimit) {
		return ErrInvalidAutoRenewLimit
	}
	actorID := 0
	if len(audit) > 0 {
		actorID = audit[0].ActorID
	}
	return database.AccountAuditTransaction(database.GetDB(), func(tx *gorm.DB) error {
		account, err := s.GetAccountByEmailTx(tx, email)
		if err != nil {
			return err
		}
		if account == nil {
			return gorm.ErrRecordNotFound
		}
		if enabled {
			if _, err := renewalPeriod(account, nil); err != nil {
				return err
			}
			if account.TotalGB <= 0 && account.ExpiryTime == 0 {
				return common.NewError("automatic renewal requires a quota or expiration")
			}
		}
		newLimit := account.AutoRenewLimit
		if limit != nil {
			newLimit = *limit
		}
		return tx.Exec(`UPDATE accounts SET auto_renew=?,auto_renew_limit=?,
			auto_renew_count=CASE WHEN auto_renew_limit<>? OR (NOT auto_renew AND ?) THEN 0 ELSE auto_renew_count END,
			auto_renew_actor_id=
			CASE WHEN ? THEN COALESCE(NULLIF(?,0), auto_renew_actor_id) ELSE 0 END
			WHERE id=? AND (auto_renew<>? OR auto_renew_limit<>?)`,
			enabled, newLimit, newLimit, enabled, enabled, actorID, account.Id, enabled, newLimit).Error
	}, audit...)
}

func renewalDepleted(traffic *xray.ClientTraffic, now int64) bool {
	return (traffic.ExpiryTime > 0 && traffic.ExpiryTime <= now) ||
		(traffic.Total > 0 && traffic.Up >= traffic.Total-traffic.Down)
}

// RenewDueAccounts rechecks eligibility and billing inside each transaction.
// The account's Enable represents manual intent; traffic.Enable may already be
// false from depletion and must not prevent a permitted renewal.
func (s *AccountService) RenewDueAccounts() ([]int, error) {
	db := database.GetDB()
	now := time.Now().UnixMilli()
	var ids []int
	err := db.Model(&model.Account{}).
		Joins("JOIN client_traffics t ON LOWER(TRIM(t.email)) = LOWER(TRIM(accounts.email))").
		Where("accounts.auto_renew = ? AND accounts.enable = ? AND accounts.representative_blocked = ?", true, true, false).
		Where("(accounts.auto_renew_limit = 0 OR accounts.auto_renew_count < accounts.auto_renew_limit)").
		Where("(t.expiry_time > 0 AND t.expiry_time <= ?) OR (t.total > 0 AND t.up >= t.total-t.down)", now).
		Order("accounts.id").Pluck("accounts.id", &ids).Error
	if err != nil {
		return nil, err
	}
	touched := map[int]bool{}
	for _, id := range ids {
		var inbounds []int
		err := db.Transaction(func(tx *gorm.DB) error {
			var account model.Account
			if err := tx.First(&account, id).Error; err != nil {
				return err
			}
			if !account.AutoRenew || !account.Enable || account.RepresentativeBlocked ||
				(account.AutoRenewLimit > 0 && account.AutoRenewCount >= account.AutoRenewLimit) {
				return nil
			}
			var traffic xray.ClientTraffic
			if err := tx.Where("LOWER(TRIM(email)) = ?", accountKey(account.Email)).First(&traffic).Error; err != nil {
				return err
			}
			if !renewalDepleted(&traffic, now) {
				return nil
			}

			// A representative's plan must use the same transactional quota ledger
			// as their manual renewal, regardless of who enabled the switch.
			var ownership model.ResellerClient
			owner := &model.User{}
			err := tx.Where("LOWER(TRIM(email)) = ?", accountKey(account.Email)).First(&ownership).Error
			if err == nil {
				if err = tx.First(owner, ownership.UserId).Error; err != nil {
					return err
				}
				if !owner.Enable {
					return nil
				}
				owner.IsReseller = true
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			inbounds, err = s.renewAccountTx(tx, owner, &account, &traffic, nil, now, true)
			return err
		})
		if err != nil {
			// A failed or unaffordable renewal has no partial billing/usage effects.
			if !errors.Is(err, ErrInsufficientBalance) && !errors.Is(err, ErrRenewalDurationUnknown) && !errors.Is(err, gorm.ErrRecordNotFound) {
				logger.Warning("automatic client renewal:", err)
			}
			continue
		}
		for _, inbound := range inbounds {
			touched[inbound] = true
		}
	}
	out := make([]int, 0, len(touched))
	for id := range touched {
		out = append(out, id)
	}
	sort.Ints(out)
	if len(out) > 0 {
		MarkSpeedLimitsDirty()
		RequestAdmissionRefresh()
	}
	return out, nil
}
