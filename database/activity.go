package database

import "gorm.io/gorm"

const resellerAccountOwnerSQL = `(SELECT r.user_id FROM reseller_clients r WHERE LOWER(TRIM(r.email))=LOWER(TRIM(a.email)) ORDER BY r.id DESC LIMIT 1)`
const defaultPanelOwnerSQL = `COALESCE(
	(SELECT id FROM users WHERE is_panel_owner ORDER BY id LIMIT 1),
	(SELECT id FROM users WHERE is_super_admin ORDER BY id LIMIT 1),0)`

// ActivityOwnerSQL follows the Clients page's stored ownership.
const ActivityOwnerSQL = `COALESCE(` + knownAccountOwnerSQL + `,` + defaultPanelOwnerSQL + `)`

const eventActorSQL = `COALESCE(NULLIF(NEW.actor_id,0),NULLIF(` + accountAuditActorSQL + `,0))`

const (
	ClientActivitySetting          = "clientActivityLogging"
	ClientActivityRetentionSetting = "clientActivityRetentionDays"
)

func initPanelActivity(db *gorm.DB) error {
	statements := []string{
		`UPDATE panel_client_events SET comment=COALESCE((SELECT comment FROM accounts WHERE id=panel_client_events.account_id),'')
			WHERE comment IS NULL`,
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
		`DROP TRIGGER IF EXISTS assign_unattributed_client_owner`,
		`CREATE TRIGGER assign_unattributed_client_owner BEFORE INSERT ON account_events
		WHEN NEW.kind IN ('updated','renewed','deleted') AND ` + eventActorSQL + ` IS NOT NULL
			AND NEW.audit_operation NOT LIKE '` + groupDefinitionAuditPrefix + `%'
		BEGIN
			UPDATE accounts SET inferred_owner_id=` + eventActorSQL + `
			WHERE id=NEW.account_id AND EXISTS (SELECT 1 FROM accounts a
				WHERE a.id=NEW.account_id AND NOT a.owner_confirmed AND ` + knownAccountOwnerSQL + ` IS NULL)
				AND EXISTS (SELECT 1 FROM users WHERE id=` + eventActorSQL + ` AND enable);
			UPDATE panel_client_events SET owner_id=` + eventActorSQL + `,
				owner=(SELECT username FROM users WHERE id=` + eventActorSQL + `)
			WHERE changes()=1 AND account_id=NEW.account_id AND owner='';
		END`,
		`DROP TRIGGER IF EXISTS panel_client_activity`,
		`CREATE TRIGGER IF NOT EXISTS panel_client_activity AFTER INSERT ON account_events
		WHEN NEW.at > 0 AND (NEW.kind='autoRenewed' OR COALESCE(
			(SELECT value FROM settings WHERE key='` + ClientActivitySetting + `' ORDER BY id LIMIT 1), 'true')='true')
		BEGIN
			INSERT INTO panel_client_events(account_id,owner_id,owner,actor_id,actor,email,comment,kind,changes,details,audit_operation,at)
			SELECT a.id,` + ActivityOwnerSQL + `,` + AccountOwnerNameSQL + `,
				COALESCE(actor.id,0),COALESCE(actor.username,''),
				a.email,COALESCE(a.comment,''),NEW.kind,COALESCE(NEW.changes,''),NEW.details,
				COALESCE(NULLIF(NEW.audit_operation,''),` + accountAuditOperationSQL + `,''),NEW.at
			FROM accounts a LEFT JOIN users actor ON actor.id=` + eventActorSQL + `
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
