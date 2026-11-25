package database

import (
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

// Trigger-based history follows writes from the UI, bot, bulk jobs and imports.
// Actual credential values and free-form comments are never copied to history.
func initAccountHistory(db *gorm.DB) error {
	const now = "CAST(unixepoch('subsec') * 1000 AS INTEGER)"
	fields := []struct{ label, columns string }{
		{"email", "email"}, {"quota", "total_gb"}, {"expiry", "expiry_time renewal_duration"},
		{"enable", "enable representative_blocked"}, {"autoRenew", "auto_renew"},
		{"limits", "limit_ip speed_limit_up speed_limit_down user_limit_override"},
		{"reset", "reset"}, {"comment", "comment"}, {"owner", "owner_admin_id"},
		{"credentials", "uuid protocol_credentials password vpn_username auth secret security naive_username sub_id tg_id"},
	}
	var predicates, changes []string
	for _, field := range fields {
		parts := []string{}
		for _, column := range strings.Fields(field.columns) {
			parts = append(parts, "OLD."+column+" IS NOT NEW."+column)
		}
		check := strings.Join(parts, " OR ")
		predicates = append(predicates, check)
		changes = append(changes, "CASE WHEN "+check+" THEN '"+field.label+",' ELSE '' END")
	}
	const renewal = "OLD.last_renewed_at IS NOT NEW.last_renewed_at"
	const quotaChanged = "OLD.total_gb IS NOT NEW.total_gb"
	const expiryChanged = "OLD.expiry_time IS NOT NEW.expiry_time"
	const periodChanged = "OLD.renewal_duration IS NOT NEW.renewal_duration"
	details := `CASE WHEN ` + quotaChanged + ` OR ` + expiryChanged + ` OR ` + periodChanged + ` OR ` + renewal + `
		THEN json_object(
			'quota', CASE WHEN ` + quotaChanged + ` THEN json_object('before', OLD.total_gb, 'after', NEW.total_gb) END,
			'expiry', CASE WHEN ` + expiryChanged + ` THEN json_object('before', OLD.expiry_time, 'after', NEW.expiry_time) END,
			'renewalPeriod', CASE WHEN ` + periodChanged + ` OR ` + renewal + `
				THEN json_object('before', OLD.renewal_duration, 'after', NEW.renewal_duration) END,
			'usageReset', json(CASE WHEN ` + renewal + ` THEN 'true' ELSE 'false' END)
		) END`
	triggers := []string{
		`DROP TRIGGER IF EXISTS client_history_updated`,
		`CREATE TRIGGER IF NOT EXISTS client_history_created AFTER INSERT ON accounts BEGIN
			INSERT INTO account_events(account_id,at,kind) VALUES(NEW.id,NEW.created_at,'created');
		END`,
		`CREATE TRIGGER IF NOT EXISTS client_history_updated AFTER UPDATE ON accounts
		WHEN ` + renewal + ` OR ` + strings.Join(predicates, " OR ") + `
		BEGIN
			INSERT INTO account_events(account_id,at,kind,changes,details) VALUES(NEW.id,` + now + `,
			    CASE WHEN OLD.last_renewed_at IS NEW.last_renewed_at THEN 'updated'
			         WHEN NEW.last_renewal_automatic THEN 'autoRenewed' ELSE 'renewed' END,
			    rtrim(` + strings.Join(changes, " || ") + `, ','), ` + details + `);
		END`,
		`CREATE TRIGGER IF NOT EXISTS client_history_remove AFTER DELETE ON accounts BEGIN
			DELETE FROM account_events WHERE account_id=OLD.id;
		END`,
	}
	for _, table := range []struct{ name, kind string }{{"account_inbounds", "inbounds"}, {"account_groups", "groups"}} {
		for _, operation := range []struct{ name, row string }{{"INSERT", "NEW"}, {"DELETE", "OLD"}} {
			triggers = append(triggers, `CREATE TRIGGER IF NOT EXISTS client_history_`+table.name+"_"+strings.ToLower(operation.name)+
				` AFTER `+operation.name+` ON `+table.name+` BEGIN
				INSERT INTO account_events(account_id,at,kind,changes)
				SELECT id,`+now+`,'updated','`+table.kind+`' FROM accounts WHERE id=`+operation.row+`.account_id;
			END`)
		}
	}
	triggers = append(triggers, `CREATE TRIGGER IF NOT EXISTS client_history_membership_status AFTER UPDATE ON account_inbounds
		WHEN OLD.enable IS NOT NEW.enable BEGIN
		INSERT INTO account_events(account_id,at,kind,changes) VALUES(NEW.account_id,`+now+`,'updated','inbounds');
	END`)
	return db.Transaction(func(tx *gorm.DB) error {
		for _, trigger := range triggers {
			if err := tx.Exec(trigger).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`INSERT INTO account_events(account_id,at,kind)
			SELECT a.id,a.created_at,'created' FROM accounts a WHERE a.created_at>0 AND NOT EXISTS (
				SELECT 1 FROM account_events e WHERE e.account_id=a.id AND e.kind='created'
			)`).Error
	})
}

// RecordAccountTrafficReset records an explicit reset in the same transaction
// as the counters. Renewal operations already capture the reset with their expiry.
func RecordAccountTrafficReset(tx *gorm.DB, accountID int) error {
	return tx.Create(&model.AccountEvent{
		AccountID: accountID, At: time.Now().UnixMilli(), Kind: "updated", Changes: "trafficUsage",
		Details: &model.ClientChangeDetails{UsageReset: true},
	}).Error
}
