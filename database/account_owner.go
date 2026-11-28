package database

import "gorm.io/gorm"

// A legacy default pointing at the panel owner is not proof of ownership.
// A reseller ledger or an explicit assignment remains authoritative.
const verifiedAccountOwnerSQL = `(` + resellerAccountOwnerSQL + ` IS NOT NULL OR a.owner_confirmed OR
	(a.owner_admin_id>0 AND a.owner_admin_id<>` + defaultPanelOwnerSQL + `))`

// AccountOwnerNameSQL leaves the name empty when only the legacy default is known.
const AccountOwnerNameSQL = `CASE WHEN ` + verifiedAccountOwnerSQL + `
	THEN COALESCE((SELECT username FROM users WHERE id=` + ActivityOwnerSQL + `),'') ELSE '' END`

func repairLegacyClientOwners(db *gorm.DB) error {
	return SnapshotTransaction(db, func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE accounts AS a SET owner_confirmed=1
			WHERE NOT a.owner_confirmed AND a.owner_admin_id>0 AND EXISTS (
				SELECT 1 FROM account_events e WHERE e.account_id=a.id AND instr(','||e.changes||',', ',owner,')>0
			)`).Error; err != nil {
			return err
		}
		// Only creation data can recover the original owner. Later editors,
		// renewal operators and shared inbound grants do not establish it.
		var recovered []struct {
			AccountID int
			OwnerID   int
			Username  string
		}
		if err := tx.Raw(`WITH legacy AS (
			SELECT a.id,a.created_at FROM accounts a WHERE NOT ` + verifiedAccountOwnerSQL + `
		), creators AS (
			SELECT l.id AS account_id,e.actor_id AS owner_id
			FROM legacy l JOIN account_events e ON e.account_id=l.id AND e.kind='created' AND e.at=l.created_at
			WHERE e.actor_id>0
			UNION
			SELECT l.id,e.owner_id FROM legacy l JOIN panel_client_events e
				ON e.account_id=l.id AND e.kind='created' AND e.at=l.created_at
			WHERE e.owner_id>0 AND e.owner_id<>` + defaultPanelOwnerSQL + `
		), unambiguous AS (
			SELECT account_id,MIN(owner_id) AS owner_id FROM creators GROUP BY account_id HAVING COUNT(*)=1
		)
		SELECT m.account_id,m.owner_id,u.username FROM unambiguous m JOIN users u ON u.id=m.owner_id`).Scan(&recovered).Error; err != nil {
			return err
		}
		for _, row := range recovered {
			if err := tx.Exec(`UPDATE accounts SET owner_admin_id=?,owner_confirmed=1 WHERE id=?`,
				row.OwnerID, row.AccountID).Error; err != nil {
				return err
			}
			if err := RepairLegacyOwnerEvents(tx, row.AccountID, row.OwnerID, row.Username); err != nil {
				return err
			}
		}
		if err := tx.Exec(`WITH known AS MATERIALIZED (
			SELECT a.id,` + ActivityOwnerSQL + ` AS owner_id,` + AccountOwnerNameSQL + ` AS owner
			FROM accounts a WHERE EXISTS (
				SELECT 1 FROM panel_client_events p WHERE p.account_id=a.id AND
					(p.owner_id=0 OR p.owner_id=` + defaultPanelOwnerSQL + `)
			) AND NOT EXISTS (
				SELECT 1 FROM account_events e WHERE e.account_id=a.id AND instr(','||e.changes||',', ',owner,')>0
			)
		) UPDATE panel_client_events AS p SET owner_id=k.owner_id,owner=k.owner FROM known k
		WHERE p.account_id=k.id AND k.owner<>'' AND (p.owner_id=0 OR p.owner_id=` + defaultPanelOwnerSQL + `)
			AND (p.owner_id<>k.owner_id OR p.owner='')`).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE panel_client_events SET owner='' WHERE owner<>'' AND owner_id=` + defaultPanelOwnerSQL + `
			AND EXISTS (SELECT 1 FROM accounts a WHERE a.id=panel_client_events.account_id AND NOT ` + verifiedAccountOwnerSQL + `)`).Error
	})
}

// RepairLegacyOwnerEvents replaces only default or missing ownership snapshots
// after an original owner has been established.
func RepairLegacyOwnerEvents(tx *gorm.DB, accountID, ownerID int, name string) error {
	return tx.Exec(`UPDATE panel_client_events SET owner_id=?,owner=? WHERE account_id=?
		AND (owner_id=0 OR owner_id=`+defaultPanelOwnerSQL+`)`, ownerID, name, accountID).Error
}
