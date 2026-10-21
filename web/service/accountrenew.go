package service

import (
	"errors"
	"math"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/util/common"
	"github.com/mhsanaei/3x-ui/v2/xray"
	"gorm.io/gorm"
)

const maxRenewalDuration = int64(100 * 365 * 24 * 60 * 60 * 1000)

var ErrRenewalDurationUnknown = errors.New("the original package duration is unknown; enter the renewal period in days")

// A deadline alone cannot reveal a legacy customer's original package duration.
// Never derive it from CreatedAt, remaining time, or the automatic-reset setting.
func captureRenewalDuration(next, old *model.Account, entry map[string]any) error {
	if old != nil {
		next.RenewalDuration = old.RenewalDuration
	}
	if value, mentioned := entry["renewalDuration"]; mentioned {
		f, ok := value.(float64)
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > float64(maxRenewalDuration) || math.Trunc(f) != f {
			return common.NewError("invalid renewal period")
		}
		next.RenewalDuration = int64(f)
	}
	if next.ExpiryTime == 0 {
		next.RenewalDuration = 0
	} else if next.RenewalDuration == 0 && next.ExpiryTime < 0 && next.Enable && (old == nil || old.Enable) {
		// Unstarted first-use plans encode their full duration. A disabled frozen
		// account encodes only its REMAINING time, so cannot supply a full period.
		if next.ExpiryTime < -maxRenewalDuration {
			return common.NewError("invalid renewal period")
		}
		next.RenewalDuration = -next.ExpiryTime
	}
	return nil
}

func renewalPeriod(account *model.Account, requested *int64) (int64, error) {
	if account.ExpiryTime == 0 {
		if requested != nil && *requested != 0 {
			return 0, common.NewError("an unlimited-time account must keep its unlimited duration")
		}
		return 0, nil
	}
	period := account.RenewalDuration
	if period == 0 && account.ExpiryTime < 0 && account.Enable {
		period = -account.ExpiryTime
	}
	if requested != nil {
		if period > 0 && *requested != period {
			return 0, common.NewError("the package duration changed; refresh the client list before renewing")
		}
		period = *requested
	}
	if period <= 0 {
		return 0, ErrRenewalDurationUnknown
	}
	if period > maxRenewalDuration {
		return 0, common.NewError("invalid renewal period")
	}
	return period, nil
}

// RenewAccount resets the current period, not the customer's identity or access.
// Ledger, account, membership breakdown and projection commit together. Cumulative
// core/node cursors and lifetime usage remain monotonic for billing and deltas.
func (s *AccountService) RenewAccount(user *model.User, email string, requested *int64) error {
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		account, err := s.GetAccountByEmailTx(tx, email)
		if err != nil {
			return err
		}
		if account == nil {
			return gorm.ErrRecordNotFound
		}
		period, err := renewalPeriod(account, requested)
		if err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		expiry := int64(0)
		if period > 0 {
			expiry = now + period
		}
		var traffic xray.ClientTraffic
		if err := tx.Where("LOWER(TRIM(email)) = ?", accountKey(email)).First(&traffic).Error; err != nil {
			return err
		}
		if err := chargeAccountRenewal(tx, user, account, &traffic, expiry, now); err != nil {
			return err
		}
		// Representative blocking is separate from the customer's manual switch.
		// The normal representative sweep retains authority over that block.
		if err := tx.Model(account).Updates(map[string]any{
			"enable": true, "expiry_time": expiry, "renewal_duration": period,
			"imported_reset_disabled": false, "imported_last_reset_at": now,
		}).Error; err != nil {
			return err
		}
		account.Enable, account.ExpiryTime, account.RenewalDuration = true, expiry, period
		if err := tx.Model(&traffic).Updates(map[string]any{
			"up": 0, "down": 0, "enable": !account.RepresentativeBlocked,
			"expiry_time": expiry, "total": account.TotalGB,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AccountInbound{}).Where("account_id = ?", account.Id).
			Updates(map[string]any{"up": 0, "down": 0}).Error; err != nil {
			return err
		}
		_, err = s.ProjectAccount(tx, account.Id)
		return err
	})
	if err == nil {
		MarkSpeedLimitsDirty()
		RequestAdmissionRefresh()
	}
	return err
}

// Match traffic-reset pricing without a separate reservation/rollback window.
// A days-per-GB seller purchases a full timed package on renewal; granting its
// original full duration for only the consumed bytes would bypass that policy.
func chargeAccountRenewal(tx *gorm.DB, user *model.User, account *model.Account, traffic *xray.ClientTraffic, expiry, now int64) error {
	if user == nil {
		return common.NewError("authentication required")
	}
	if !user.IsReseller {
		return nil
	}
	var owner model.ResellerClient
	if err := tx.Where("LOWER(TRIM(email)) = ? AND user_id = ?", accountKey(account.Email), user.Id).First(&owner).Error; err != nil {
		return ErrResetNotOwned
	}
	loaded, err := (&ResellerService{}).profileForDB(tx, user.Id)
	if err != nil {
		return err
	}
	profile := *loaded
	if profile.ConsumptionBased {
		// Unlike a plain counter reset, renewal also changes expiry. Validate the
		// ordinary edit limits before applying the reset's billing semantics.
		if _, err := Quote(QuoteInput{Profile: profile, OldTotal: account.TotalGB, NewTotal: account.TotalGB,
			CurrentExpiry: account.ExpiryTime, RequestedExpiry: &expiry, NowMillis: now}); err != nil {
			return err
		}
	}
	cleared := traffic.Up + traffic.Down
	if !profile.ConsumptionBased && profile.DaysPerGB > 0 && cleared < account.TotalGB {
		cleared = account.TotalGB
	}
	if !profile.ConsumptionBased && profile.DaysPerGB > 0 && expiry > 0 {
		allowed := float64(account.TotalGB) / float64(oneGB) * float64(profile.DaysPerGB) * 86400000
		if float64(expiry-now) > allowed {
			return common.NewError("the renewal period exceeds the representative's days-per-GB allowance")
		}
	}
	quote, err := Quote(QuoteInput{Profile: profile, Reset: true, ClearedBytes: cleared,
		OldCharged: owner.ChargedBytes, NewTotal: account.TotalGB, CurrentExpiry: account.ExpiryTime, NowMillis: now})
	if err != nil {
		return err
	}
	if err := addSpent(tx, user.Id, quote.DeltaSpent); err != nil {
		return err
	}
	return tx.Model(&owner).Update("charged_bytes", quote.NewCharged).Error
}
