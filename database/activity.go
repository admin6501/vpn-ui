package database

import "gorm.io/gorm"

// ActivityOwnerSQL follows the Clients page: representative ledger first,
// explicit admin second, panel owner for older accounts or system-created rows.
const ActivityOwnerSQL = `COALESCE(
	(SELECT r.user_id FROM reseller_clients r WHERE LOWER(TRIM(r.email))=LOWER(TRIM(a.email)) ORDER BY r.id DESC LIMIT 1),
	NULLIF(a.owner_admin_id,0),
	(SELECT id FROM users WHERE is_panel_owner ORDER BY id LIMIT 1),
	(SELECT id FROM users WHERE is_super_admin ORDER BY id LIMIT 1),0)`

const ClientActivitySetting = "clientActivityLogging"

func initPanelActivity(db *gorm.DB) error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_reseller_clients_email_key ON reseller_clients(LOWER(TRIM(email)))`,
		`DROP TRIGGER IF EXISTS panel_client_activity`,
		`CREATE TRIGGER IF NOT EXISTS panel_client_activity AFTER INSERT ON account_events
		WHEN NEW.at > 0 AND (NEW.kind='autoRenewed' OR COALESCE(
			(SELECT value FROM settings WHERE key='` + ClientActivitySetting + `' ORDER BY id LIMIT 1), 'true')='true')
		BEGIN
			INSERT INTO panel_client_events(account_id,owner_id,owner,email,kind,changes,details,at)
			SELECT a.id,` + ActivityOwnerSQL + `,COALESCE((SELECT username FROM users WHERE id=` + ActivityOwnerSQL + `),''),
				a.email,NEW.kind,COALESCE(NEW.changes,''),NEW.details,NEW.at
			FROM accounts a WHERE a.id=NEW.account_id;
		END`,
		`CREATE TRIGGER IF NOT EXISTS panel_client_activity_delete BEFORE DELETE ON accounts
		WHEN COALESCE((SELECT value FROM settings WHERE key='` + ClientActivitySetting + `' ORDER BY id LIMIT 1), 'true')='true'
		BEGIN
			INSERT INTO panel_client_events(account_id,owner_id,owner,email,kind,changes,at)
			SELECT a.id,` + ActivityOwnerSQL + `,COALESCE((SELECT username FROM users WHERE id=` + ActivityOwnerSQL + `),''),
				a.email,'deleted','',CAST(unixepoch('subsec')*1000 AS INTEGER)
			FROM accounts a WHERE a.id=OLD.id;
		END`,
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
