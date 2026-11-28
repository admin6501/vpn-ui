package database

import "gorm.io/gorm"

const resellerAccountOwnerSQL = `(SELECT r.user_id FROM reseller_clients r WHERE LOWER(TRIM(r.email))=LOWER(TRIM(a.email)) ORDER BY r.id DESC LIMIT 1)`
const defaultPanelOwnerSQL = `COALESCE(
	(SELECT id FROM users WHERE is_panel_owner ORDER BY id LIMIT 1),
	(SELECT id FROM users WHERE is_super_admin ORDER BY id LIMIT 1),0)`

// ActivityOwnerSQL follows the Clients page's stored ownership.
const ActivityOwnerSQL = `COALESCE(` + resellerAccountOwnerSQL + `,NULLIF(a.owner_admin_id,0),` + defaultPanelOwnerSQL + `)`

const (
	ClientActivitySetting          = "clientActivityLogging"
	ClientActivityRetentionSetting = "clientActivityRetentionDays"
)

func initPanelActivity(db *gorm.DB) error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_reseller_clients_email_key ON reseller_clients(LOWER(TRIM(email)))`,
		`CREATE INDEX IF NOT EXISTS idx_panel_client_event_operation ON panel_client_events(account_id,audit_operation) WHERE audit_operation<>''`,
		`CREATE INDEX IF NOT EXISTS idx_account_event_actor_history ON account_events(account_id,id) WHERE actor_id>0`,
		`UPDATE account_events AS e SET actor_id=COALESCE((
			SELECT MAX(p.actor_id) FROM panel_client_events p
			WHERE p.account_id=e.account_id AND p.audit_operation=e.audit_operation AND p.audit_operation<>''
			HAVING COUNT(DISTINCT NULLIF(p.actor_id,0))=1
		),0) WHERE e.actor_id=0 AND e.audit_operation<>'' AND EXISTS (
			SELECT 1 FROM panel_client_events p WHERE p.account_id=e.account_id AND p.audit_operation=e.audit_operation
			AND p.audit_operation<>'' AND p.actor_id>0)`,
		`DROP TRIGGER IF EXISTS panel_client_activity`,
		`CREATE TRIGGER IF NOT EXISTS panel_client_activity AFTER INSERT ON account_events
		WHEN NEW.at > 0 AND (NEW.kind='autoRenewed' OR COALESCE(
			(SELECT value FROM settings WHERE key='` + ClientActivitySetting + `' ORDER BY id LIMIT 1), 'true')='true')
		BEGIN
			INSERT INTO panel_client_events(account_id,owner_id,owner,actor_id,actor,email,kind,changes,details,audit_operation,at)
			SELECT a.id,` + ActivityOwnerSQL + `,` + AccountOwnerNameSQL + `,
				COALESCE(actor.id,0),COALESCE(actor.username,''),
				a.email,NEW.kind,COALESCE(NEW.changes,''),NEW.details,
				COALESCE(NULLIF(NEW.audit_operation,''),` + accountAuditOperationSQL + `,''),NEW.at
			FROM accounts a LEFT JOIN users actor ON actor.id=COALESCE(NULLIF(NEW.actor_id,0),` + accountAuditActorSQL + `)
			WHERE a.id=NEW.account_id;
		END`,
		`DROP TRIGGER IF EXISTS panel_client_activity_delete`,
		`CREATE TRIGGER IF NOT EXISTS panel_client_activity_remove_hidden AFTER DELETE ON panel_client_events
		BEGIN
			DELETE FROM panel_client_hiddens WHERE event_id=OLD.id;
		END`,
	}
	statements = append(statements, paymentReminderStatements()...)
	return db.Transaction(func(tx *gorm.DB) error {
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
