package service

import (
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

const ActivityPageSize = 30

type ActivityService struct{}

func (ActivityService) RecordLogin(user *model.User, ip, os, kind string) error {
	return database.GetDB().Create(&model.PanelLoginEvent{
		UserID: user.Id, Username: user.Username, IP: ip, OS: os, Kind: kind, At: time.Now().UnixMilli(),
	}).Error
}

func (ActivityService) LoginEvents(user *model.User, own bool, before int64) ([]model.PanelLoginEvent, error) {
	db := database.GetDB().Model(&model.PanelLoginEvent{})
	if !user.IsPanelOwner || own {
		db = db.Where("user_id = ?", user.Id)
	}
	var rows []model.PanelLoginEvent
	err := eventPage(db, before).Find(&rows).Error
	return rows, err
}

func (ActivityService) PurgeLoginEvents(cutoff, now time.Time) error {
	cutoff = cutoff.AddDate(0, 0, 1)
	if protected := now.Add(-24 * time.Hour); protected.Before(cutoff) {
		cutoff = protected
	}
	return database.GetDB().Where("at < ? AND id < (SELECT id FROM panel_login_events ORDER BY id DESC LIMIT 1 OFFSET 59)",
		cutoff.UnixMilli()).Delete(&model.PanelLoginEvent{}).Error
}

func eventPage(db *gorm.DB, before int64) *gorm.DB {
	if before > 0 {
		db = db.Where("id < ?", before)
	}
	return db.Order("id DESC").Limit(ActivityPageSize + 1)
}

func renewalEvents(user *model.User) *gorm.DB {
	db := database.GetDB().Model(&model.PanelClientEvent{}).Where("kind = ?", "autoRenewed")
	if !user.IsPanelOwner {
		db = db.Where("owner_id = ?", user.Id)
	}
	return db
}

func (ActivityService) RenewalEvents(user *model.User, before int64) ([]model.PanelClientEvent, error) {
	var rows []model.PanelClientEvent
	err := eventPage(renewalEvents(user), before).Find(&rows).Error
	return rows, err
}

func (ActivityService) ChangeEvents(month time.Time, owner string, before int64) ([]model.PanelClientEvent, error) {
	db := monthEvents(month)
	if owner != "" {
		db = db.Where("owner = ?", owner)
	}
	var rows []model.PanelClientEvent
	err := eventPage(db, before).Find(&rows).Error
	return rows, err
}

func monthEvents(month time.Time) *gorm.DB {
	return database.GetDB().Model(&model.PanelClientEvent{}).Where("kind <> ? AND at >= ? AND at < ?",
		"autoRenewed", month.UnixMilli(), month.AddDate(0, 1, 0).UnixMilli())
}

func (ActivityService) ChangeMonths() ([]string, error) {
	months := []string{}
	err := database.GetDB().Model(&model.PanelClientEvent{}).Where("kind <> ?", "autoRenewed").
		Select("DISTINCT strftime('%Y-%m', at/1000, 'unixepoch') AS month").Order("month DESC").Scan(&months).Error
	return months, err
}

func (ActivityService) ChangeOwners(month time.Time) ([]string, error) {
	owners := []string{}
	err := monthEvents(month).Distinct("owner").Order("owner").Pluck("owner", &owners).Error
	return owners, err
}

func (ActivityService) PurgeChangeEvents(month time.Time) error {
	return monthEvents(month).Delete(&model.PanelClientEvent{}).Error
}

func (ActivityService) ChangeLogging() (bool, error) {
	setting, err := (&SettingService{}).getSetting(database.ClientActivitySetting)
	if database.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return setting.Value == "true", nil
}

func (ActivityService) SetChangeLogging(enabled bool) error {
	return (&SettingService{}).setBool(database.ClientActivitySetting, enabled)
}

// ScanExpired rotates the notice ID only when an account expires again or moves
// to another owner. An absent traffic row falls back to the account's expiry.
func (ActivityService) ScanExpired(now time.Time) error {
	const expired = `WITH expired AS (
		SELECT a.id account_id,` + database.ActivityOwnerSQL + ` owner_id, a.email
		FROM accounts a LEFT JOIN client_traffics t ON LOWER(TRIM(t.email))=LOWER(TRIM(a.email))
		WHERE (COALESCE(t.expiry_time,a.expiry_time)>0 AND COALESCE(t.expiry_time,a.expiry_time)<=?)
			OR (COALESCE(t.total,a.total_gb)>0 AND COALESCE(t.up,0)>=COALESCE(t.total,a.total_gb)-COALESCE(t.down,0))
	) `
	return database.SnapshotTransaction(database.GetDB(), func(tx *gorm.DB) error {
		var changed bool
		err := tx.Raw(expired+`SELECT EXISTS(
			SELECT 1 FROM expired e LEFT JOIN panel_expiries p ON p.account_id=e.account_id AND p.owner_id=e.owner_id
			WHERE e.owner_id>0 AND (p.id IS NULL OR p.email<>e.email)
			UNION ALL
			SELECT 1 FROM panel_expiries p LEFT JOIN expired e ON p.account_id=e.account_id AND p.owner_id=e.owner_id
			WHERE e.account_id IS NULL)`, now.UnixMilli()).Scan(&changed).Error
		if err != nil || !changed {
			return err
		}
		if err := tx.Exec(expired+`DELETE FROM panel_expiries WHERE NOT EXISTS
			(SELECT 1 FROM expired e WHERE e.account_id=panel_expiries.account_id AND e.owner_id=panel_expiries.owner_id)`, now.UnixMilli()).Error; err != nil {
			return err
		}
		return tx.Exec(expired+`INSERT INTO panel_expiries(account_id,owner_id,email)
			SELECT e.account_id,e.owner_id,e.email FROM expired e LEFT JOIN panel_expiries p ON p.account_id=e.account_id
			WHERE e.owner_id>0 AND (p.id IS NULL OR p.email<>e.email)
			ON CONFLICT(account_id) DO UPDATE SET email=excluded.email`, now.UnixMilli()).Error
	})
}

type ActivityNotice struct {
	Count   int64    `json:"count"`
	Through int64    `json:"through"`
	Names   []string `json:"names" gorm:"-"`
}

func noticeEvents(user *model.User, kind string) *gorm.DB {
	if kind == "renewals" {
		return renewalEvents(user)
	}
	return database.GetDB().Model(&model.PanelExpiry{}).Where("owner_id = ?", user.Id)
}

func (ActivityService) Notifications(user *model.User, kind string) (ActivityNotice, error) {
	result := ActivityNotice{Names: []string{}}
	db := noticeEvents(user, kind).Where("id > COALESCE((SELECT through FROM panel_activity_reads WHERE user_id=? AND kind=?),0)", user.Id, kind)
	err := db.Session(&gorm.Session{}).Select("COUNT(*) AS count, COALESCE(MAX(id),0) AS through").Scan(&result).Error
	if err != nil || result.Count == 0 {
		return result, err
	}
	err = db.Where("id <= ?", result.Through).Order("id DESC").Limit(8).Pluck("email", &result.Names).Error
	return result, err
}

func (ActivityService) DismissNotice(user *model.User, kind string, through int64) error {
	var valid int64
	err := noticeEvents(user, kind).Where("id <= ?", through).Select("COALESCE(MAX(id),0)").Scan(&valid).Error
	if err != nil || valid == 0 {
		return err
	}
	return database.GetDB().Exec(`INSERT INTO panel_activity_reads(user_id,kind,through) VALUES(?,?,?)
		ON CONFLICT(user_id,kind) DO UPDATE SET through=MAX(panel_activity_reads.through,excluded.through)`, user.Id, kind, valid).Error
}
