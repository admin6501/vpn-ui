package database

import (
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

const (
	accountAuditOperationSQL = "(SELECT operation FROM account_audit_context WHERE id=1)"
	accountAuditActorSQL     = "COALESCE((SELECT actor_id FROM account_audit_context WHERE id=1),0)"
	accountAuditNowSQL       = "CAST(unixepoch('subsec') * 1000 AS INTEGER)"
)

// An operation can span more than one transaction. Use its first timestamp so
// the monthly view and retention cutoff keep the whole entry together.
func accountAuditAtSQL(accountID string) string {
	return `COALESCE((SELECT at FROM account_events WHERE audit_operation<>'' AND audit_operation=` +
		accountAuditOperationSQL + ` AND account_id=` + accountID + ` ORDER BY id LIMIT 1),` + accountAuditNowSQL + `)`
}

// Trigger-based history follows writes from the UI, bot, bulk jobs and imports.
// Actual credential values and free-form comments are never copied to history.
// A legacy account can be imported with its current terms; only a tracked
// creation can attest to the terms at the point of sale.
func initAccountHistory(db *gorm.DB) error {
	const operation = accountAuditOperationSQL
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
	const autoRenewChanged = "OLD.auto_renew IS NOT NEW.auto_renew"
	details := `CASE WHEN ` + quotaChanged + ` OR ` + expiryChanged + ` OR ` + periodChanged + ` OR ` + renewal + ` OR ` + autoRenewChanged + `
		THEN json_object(
			'quota', CASE WHEN ` + quotaChanged + ` THEN json_object('before', OLD.total_gb, 'after', NEW.total_gb) END,
			'expiry', CASE WHEN ` + expiryChanged + ` THEN json_object('before', OLD.expiry_time, 'after', NEW.expiry_time) END,
			'renewalPeriod', CASE WHEN ` + periodChanged + ` OR ` + renewal + `
				THEN json_object('before', OLD.renewal_duration, 'after', NEW.renewal_duration) END,
			'usageReset', json(CASE WHEN ` + renewal + ` THEN 'true' ELSE 'false' END),
			'autoRenewEnabled', CASE WHEN ` + autoRenewChanged + ` THEN json(CASE WHEN NEW.auto_renew THEN 'true' ELSE 'false' END) END
		) END`
	triggers := []string{
		`CREATE TABLE IF NOT EXISTS account_audit_context (id INTEGER PRIMARY KEY, operation TEXT NOT NULL, actor_id INTEGER NOT NULL DEFAULT 0)`,
		`INSERT OR IGNORE INTO account_audit_context(id,operation) VALUES(1,'')`,
		`CREATE INDEX IF NOT EXISTS idx_account_events_audit_operation ON account_events(audit_operation,account_id)
			WHERE audit_operation<>''`,
		`DROP TRIGGER IF EXISTS client_history_created`,
		`DROP TRIGGER IF EXISTS client_history_updated`,
		`DROP TRIGGER IF EXISTS client_history_remove`,
		`CREATE TRIGGER client_history_created AFTER INSERT ON accounts BEGIN
			INSERT INTO account_events(account_id,at,kind,details,audit_operation,actor_id)
			VALUES(NEW.id,NEW.created_at,'created',
				CASE WHEN ` + operation + ` <> '' THEN
					json_object('created',json_object('quota',COALESCE(NEW.total_gb,0),
						'expiry',COALESCE(NEW.expiry_time,0),'period',COALESCE(NEW.renewal_duration,0)))
				END,` + operation + `,` + accountAuditActorSQL + `);
		END`,
		`CREATE TRIGGER client_history_updated AFTER UPDATE ON accounts
		WHEN ` + renewal + ` OR ` + strings.Join(predicates, " OR ") + `
		BEGIN
			INSERT INTO account_events(account_id,at,kind,changes,details,audit_operation,actor_id) VALUES(NEW.id,` + accountAuditAtSQL("NEW.id") + `,
			    CASE WHEN OLD.last_renewed_at IS NEW.last_renewed_at THEN 'updated'
			         WHEN NEW.last_renewal_automatic THEN 'autoRenewed' ELSE 'renewed' END,
			    rtrim(` + strings.Join(changes, " || ") + `, ','), ` + details + `, ` + operation + `,` + accountAuditActorSQL + `);
		END`,
		`CREATE TRIGGER client_history_remove BEFORE DELETE ON accounts BEGIN
			INSERT INTO account_events(account_id,at,kind,details,audit_operation,actor_id)
			SELECT OLD.id,` + accountAuditAtSQL("OLD.id") + `,'deleted',
				json_object('deleted',json_object(
					'quota', COALESCE(t.total,OLD.total_gb,0),
					'expiry', COALESCE(t.expiry_time,OLD.expiry_time,0),
					'period', COALESCE(OLD.renewal_duration,0),
					'enabled', json(CASE WHEN COALESCE(t.enable,OLD.enable) AND NOT OLD.representative_blocked THEN 'true' ELSE 'false' END),
					'initialQuota', (SELECT CASE WHEN json_valid(e.details) THEN json_extract(e.details,'$.created.quota') END FROM account_events e
						WHERE e.account_id=OLD.id AND e.kind='created' ORDER BY e.id LIMIT 1),
					'upload', t.up, 'download', t.down, 'lifetime', t.all_time)),
				` + operation + `,` + accountAuditActorSQL + `
			FROM (SELECT 1) LEFT JOIN client_traffics t ON t.id=(
				SELECT id FROM client_traffics WHERE LOWER(TRIM(email))=LOWER(TRIM(OLD.email)) ORDER BY id LIMIT 1
			);
		END`,
	}
	for _, table := range []struct{ name, kind string }{{"account_inbounds", "inbounds"}, {"account_groups", "groups"}} {
		for _, change := range []struct{ name, row string }{{"INSERT", "NEW"}, {"DELETE", "OLD"}} {
			trigger := "client_history_" + table.name + "_" + strings.ToLower(change.name)
			details := "NULL"
			if table.kind == "inbounds" {
				before, after := "NULL", inboundAuditState("NEW")
				if change.name == "DELETE" {
					before, after = inboundAuditState("OLD"), "NULL"
				}
				details = inboundAuditDetails(change.row, before, after)
			}
			triggers = append(triggers, "DROP TRIGGER IF EXISTS "+trigger, `CREATE TRIGGER `+trigger+
				` AFTER `+change.name+` ON `+table.name+` BEGIN
				INSERT INTO account_events(account_id,at,kind,changes,details,audit_operation,actor_id)
				SELECT id,`+accountAuditAtSQL(change.row+".account_id")+`,'updated','`+table.kind+`',`+details+`,`+operation+`,`+accountAuditActorSQL+`
				FROM accounts WHERE id=`+change.row+`.account_id;
			END`)
		}
	}
	triggers = append(triggers, `DROP TRIGGER IF EXISTS client_history_membership_status`, `
		CREATE TRIGGER client_history_membership_status AFTER UPDATE ON account_inbounds
		WHEN COALESCE(OLD.enable,1) IS NOT COALESCE(NEW.enable,1) BEGIN
			INSERT INTO account_events(account_id,at,kind,changes,details,audit_operation,actor_id)
			VALUES(NEW.account_id,`+accountAuditAtSQL("NEW.account_id")+`,'updated','inbounds',
				`+inboundAuditDetails("NEW", inboundAuditState("OLD"), inboundAuditState("NEW"))+`,`+operation+`,`+accountAuditActorSQL+`);
		END`)
	return db.Transaction(func(tx *gorm.DB) error {
		for _, trigger := range triggers {
			if err := tx.Exec(trigger).Error; err != nil {
				return err
			}
		}
		if !tx.Migrator().HasColumn("account_audit_context", "actor_id") {
			if err := tx.Exec("ALTER TABLE account_audit_context ADD COLUMN actor_id INTEGER NOT NULL DEFAULT 0").Error; err != nil {
				return err
			}
		}
		return tx.Exec(`INSERT INTO account_events(account_id,at,kind)
			SELECT a.id,a.created_at,'created' FROM accounts a WHERE a.created_at>0 AND NOT EXISTS (
				SELECT 1 FROM account_events e WHERE e.account_id=a.id AND e.kind='created'
			)`).Error
	})
}

func inboundAuditState(row string) string {
	return "json(CASE WHEN COALESCE(" + row + ".enable,1) THEN 'true' ELSE 'false' END)"
}

func inboundAuditDetails(row, before, after string) string {
	id := row + ".inbound_id"
	return `json_object('inbounds',json_array(json_object(
		'id',` + id + `, 'name',COALESCE((SELECT COALESCE(NULLIF(remark,''),tag) FROM inbounds WHERE id=` + id + `),''),
		'protocol',COALESCE((SELECT protocol FROM inbounds WHERE id=` + id + `),''),
		'port',COALESCE((SELECT port FROM inbounds WHERE id=` + id + `),0),
		'before',` + before + `,'after',` + after + `)))`
}

// RecordAccountTrafficReset records an explicit reset in the same transaction
// as the counters. Renewal operations already capture the reset with their expiry.
func RecordAccountTrafficReset(tx *gorm.DB, accountID int) error {
	return tx.Create(&model.AccountEvent{
		AccountID: accountID, At: time.Now().UnixMilli(), Kind: "updated", Changes: "trafficUsage",
		Details: &model.ClientChangeDetails{UsageReset: true},
	}).Error
}
