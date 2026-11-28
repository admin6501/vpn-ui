package service

import (
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

type LowQuotaClient struct {
	ID        int    `json:"id"`
	Email     string `json:"email"`
	Comment   string `json:"comment,omitempty"`
	Remaining int64  `json:"remaining"`
}

const lowQuotaRemainingSQL = "t.total-COALESCE(t.up,0)-COALESCE(t.down,0)"

func lowQuotaClients(user *model.User) *gorm.DB {
	return database.GetDB().Table("accounts a").
		Joins("JOIN client_traffics t ON LOWER(TRIM(t.email))=LOWER(TRIM(a.email))").
		Where("a.enable AND NOT a.representative_blocked AND t.total>0 AND "+lowQuotaRemainingSQL+">0 AND "+lowQuotaRemainingSQL+"<?", 1<<30).
		Where("(t.expiry_time<=0 OR t.expiry_time>?) AND "+database.ActivityOwnerSQL+"=?", time.Now().UnixMilli(), user.Id)
}

func (ActivityService) LowQuotaNotice(user *model.User) (ActivityNotice, error) {
	result := ActivityNotice{Names: []string{}}
	if !user.Can(model.PermAccessInbounds) || user.LowQuotaNoticesDisabled {
		return result, nil
	}
	query := lowQuotaClients(user)
	if err := query.Session(&gorm.Session{}).Count(&result.Count).Error; err != nil || result.Count == 0 {
		return result, err
	}
	result.Through = 1 // the badge is dismissed for this login, independently of its count
	err := query.Order(lowQuotaRemainingSQL).Limit(8).Pluck("a.email", &result.Names).Error
	return result, err
}

func (ActivityService) LowQuotaClients(user *model.User, before int64) ([]LowQuotaClient, error) {
	var rows []LowQuotaClient
	if !user.Can(model.PermAccessInbounds) {
		return rows, nil
	}
	query := lowQuotaClients(user).Select("a.id,a.email,a.comment," + lowQuotaRemainingSQL + " AS remaining")
	if before > 0 {
		query = query.Where("a.id<?", before)
	}
	err := query.Order("a.id DESC").Limit(ActivityPageSize + 1).Scan(&rows).Error
	return rows, err
}

func (ActivityService) SetLowQuotaNotices(user *model.User, enabled bool) error {
	return database.GetDB().Model(&model.User{}).Where("id=?", user.Id).UpdateColumn("low_quota_notices_disabled", !enabled).Error
}
