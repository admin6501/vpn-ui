package database

import "gorm.io/gorm"

func paymentReminderStatements() []string {
	const oldQuota = "json_extract(NEW.details,'$.quota.before')"
	const newQuota = "json_extract(NEW.details,'$.quota.after')"
	const oldExpiry = "json_extract(NEW.details,'$.expiry.before')"
	const newExpiry = "json_extract(NEW.details,'$.expiry.after')"
	const oldTimeLeft = "(CASE WHEN " + oldExpiry + "<0 THEN -" + oldExpiry + " ELSE MAX(" + oldExpiry + "-NEW.at,0) END)"
	const newTimeLeft = "(CASE WHEN " + newExpiry + "<0 THEN -" + newExpiry + " ELSE MAX(" + newExpiry + "-NEW.at,0) END)"
	const extendedQuota = oldQuota + ">0 AND (" + newQuota + "=0 OR " + newQuota + ">" + oldQuota + ")"
	// Freezing and unfreezing convert between absolute and relative expiry and
	// change enable together. Two absolute dates can still be extended while
	// enabling a client in the same edit.
	const extendedExpiry = oldExpiry + "<>0 AND (" + newExpiry + "=0 OR " + newTimeLeft + ">" + oldTimeLeft + ") AND (" +
		"(" + oldExpiry + ">0 AND " + newExpiry + ">0) OR instr(',' || NEW.changes || ',', ',enable,')=0)"

	return []string{
		`CREATE INDEX IF NOT EXISTS idx_pending_payments_actor ON panel_payment_reminders(actor_id,revision) WHERE confirmed_at=0`,
		`DROP TRIGGER IF EXISTS panel_payment_reminder_changed`,
		`CREATE TRIGGER panel_payment_reminder_changed AFTER INSERT ON account_events
		WHEN NEW.at>0 AND (
			NEW.kind IN ('renewed','autoRenewed')
			OR (NEW.audit_operation<>'' AND (NEW.kind='created'
				OR (NEW.kind='updated' AND ((` + extendedQuota + `) OR (` + extendedExpiry + `)))))
		)
		BEGIN
			INSERT INTO panel_payment_reminders(account_id,actor_id,revision,kind,at,confirmed_at,confirmed_by)
			VALUES(NEW.account_id,COALESCE(` + eventActorSQL + `,0),NEW.id,NEW.kind,NEW.at,0,0)
			ON CONFLICT(account_id) DO UPDATE SET revision=excluded.revision,kind=excluded.kind,at=excluded.at,
				actor_id=excluded.actor_id,confirmed_at=0,confirmed_by=0;
		END`,
		`CREATE TRIGGER IF NOT EXISTS panel_payment_reminder_deleted AFTER DELETE ON accounts
		BEGIN
			DELETE FROM panel_payment_reminders WHERE account_id=OLD.id;
		END`,
	}
}

// The latest toggle in the relevant period identifies the automation's operator.
func autoRenewEditorSQL(accountID, before string) string {
	return `(SELECT CASE WHEN COALESCE(CASE WHEN json_valid(e.details) THEN json_extract(e.details,'$.autoRenewEnabled') END,1)=1
		THEN NULLIF(e.actor_id,0) END FROM account_events e WHERE e.account_id=` + accountID + `
		AND instr(','||e.changes||',', ',autoRenew,')>0 ` + before + `
		ORDER BY e.id DESC LIMIT 1)`
}

func backfillClientNotificationActors(tx *gorm.DB) error {
	if err := tx.Exec(`UPDATE accounts SET auto_renew_actor_id=` + autoRenewEditorSQL("accounts.id", "") + `
		WHERE auto_renew AND auto_renew_actor_id=0 AND ` + autoRenewEditorSQL("accounts.id", "") + ` IS NOT NULL`).Error; err != nil {
		return err
	}
	if err := tx.Exec(`WITH operators AS MATERIALIZED (
		SELECT p.id,COALESCE(` + autoRenewEditorSQL("p.account_id", "AND e.at<=p.at") + `,CASE WHEN p.owner<>'' THEN NULLIF(p.owner_id,0) END) AS actor_id
		FROM panel_client_events p WHERE p.kind='autoRenewed' AND p.actor_id=0 AND (p.owner<>'' OR EXISTS (
			SELECT 1 FROM account_events e WHERE e.account_id=p.account_id AND e.actor_id>0
			AND instr(','||e.changes||',', ',autoRenew,')>0 AND e.at<=p.at))
	) UPDATE panel_client_events SET actor_id=u.id,actor=u.username FROM operators o JOIN users u ON u.id=o.actor_id
		WHERE panel_client_events.id=o.id`).Error; err != nil {
		return err
	}
	return tx.Exec(`WITH operators AS MATERIALIZED (
		SELECT p.account_id,COALESCE(NULLIF(e.actor_id,0),CASE WHEN p.kind='autoRenewed' THEN
			COALESCE(` + autoRenewEditorSQL("p.account_id", "AND e.id<p.revision") + `,` + knownAccountOwnerSQL + `)
		END) AS actor_id FROM panel_payment_reminders p JOIN accounts a ON a.id=p.account_id
		LEFT JOIN account_events e ON e.id=p.revision AND e.account_id=p.account_id
		WHERE p.actor_id=0 AND p.confirmed_at=0
	) UPDATE panel_payment_reminders SET actor_id=o.actor_id FROM operators o
		WHERE panel_payment_reminders.account_id=o.account_id AND o.actor_id IS NOT NULL`).Error
}
