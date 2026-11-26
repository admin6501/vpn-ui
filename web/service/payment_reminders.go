package service

import (
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

type PaymentReminder struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Kind  string `json:"kind"`
	At    int64  `json:"at"`
}

func pendingPayments(user *model.User) *gorm.DB {
	return database.GetDB().Table("panel_payment_reminders p").
		Joins("JOIN accounts a ON a.id=p.account_id").
		Where("p.confirmed_at=0 AND "+database.ActivityOwnerSQL+"=?", user.Id)
}

func (ActivityService) PendingPayments(user *model.User, before int64) ([]PaymentReminder, error) {
	rows := []PaymentReminder{}
	if !user.Can(model.PermAccessInbounds) || !user.PaymentReminders {
		return rows, nil
	}
	query := pendingPayments(user).Select("p.revision AS id, a.email, p.kind, p.at")
	if before > 0 {
		query = query.Where("p.revision < ?", before)
	}
	err := query.Order("p.revision DESC").Limit(ActivityPageSize + 1).Scan(&rows).Error
	return rows, err
}

func (ActivityService) PaymentNotice(user *model.User) (ActivityNotice, error) {
	notice := ActivityNotice{Names: []string{}}
	if !user.Can(model.PermAccessInbounds) || !user.PaymentReminders {
		return notice, nil
	}
	query := pendingPayments(user)
	err := query.Session(&gorm.Session{}).Count(&notice.Count).Error
	if err != nil || notice.Count == 0 {
		return notice, err
	}
	err = query.Order("p.revision DESC").Limit(8).Pluck("a.email", &notice.Names).Error
	return notice, err
}

func (ActivityService) SetPaymentReminders(user *model.User, enabled bool) error {
	if !user.Can(model.PermAccessInbounds) {
		return gorm.ErrRecordNotFound
	}
	return database.GetDB().Model(&model.User{}).Where("id=?", user.Id).UpdateColumn("payment_reminders", enabled).Error
}

func (ActivityService) ConfirmPayment(user *model.User, revision int64) error {
	if !user.Can(model.PermAccessInbounds) || revision <= 0 {
		return gorm.ErrRecordNotFound
	}
	// Authorize and confirm the exact displayed revision in one write, so an
	// ownership transfer or a newer renewal cannot be approved from a stale tab.
	result := database.GetDB().Exec(`UPDATE panel_payment_reminders
		SET confirmed_at=?, confirmed_by=? WHERE revision=? AND confirmed_at=0
		AND EXISTS (SELECT 1 FROM accounts a WHERE a.id=panel_payment_reminders.account_id
			AND `+database.ActivityOwnerSQL+`=?)
		AND EXISTS (SELECT 1 FROM users WHERE id=? AND enable AND payment_reminders)`,
		time.Now().UnixMilli(), user.Id, revision, user.Id, user.Id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
