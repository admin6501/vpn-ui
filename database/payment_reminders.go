package database

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
		`CREATE INDEX IF NOT EXISTS idx_pending_payment_reminders ON panel_payment_reminders(revision) WHERE confirmed_at=0`,
		`DROP TRIGGER IF EXISTS panel_payment_reminder_changed`,
		`CREATE TRIGGER panel_payment_reminder_changed AFTER INSERT ON account_events
		WHEN NEW.at>0 AND (
			NEW.kind IN ('renewed','autoRenewed')
			OR (NEW.audit_operation<>'' AND (NEW.kind='created'
				OR (NEW.kind='updated' AND ((` + extendedQuota + `) OR (` + extendedExpiry + `)))))
		)
		BEGIN
			INSERT INTO panel_payment_reminders(account_id,revision,kind,at,confirmed_at,confirmed_by)
			VALUES(NEW.account_id,NEW.id,NEW.kind,NEW.at,0,0)
			ON CONFLICT(account_id) DO UPDATE SET revision=excluded.revision,kind=excluded.kind,at=excluded.at,
				confirmed_at=0,confirmed_by=0;
		END`,
		`CREATE TRIGGER IF NOT EXISTS panel_payment_reminder_deleted AFTER DELETE ON accounts
		BEGIN
			DELETE FROM panel_payment_reminders WHERE account_id=OLD.id;
		END`,
	}
}
