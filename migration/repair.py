#!/usr/bin/env python3
"""Restore only absent imported accounting rows; never overwrite existing counters."""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import sqlite3


def milliseconds(value):
    if not value:
        return 0
    dt = datetime.fromisoformat(value)
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    return int(dt.timestamp() * 1000)


def repair(path, apply=False):
    path = Path(path).resolve()
    with sqlite3.connect(path.as_uri() + ('?mode=rw' if apply else '?mode=ro'), uri=True) as db:
        db.row_factory = sqlite3.Row
        db.execute('BEGIN IMMEDIATE' if apply else 'BEGIN')
        originals = {}
        history = {}
        last_reset = {}
        for row in db.execute("SELECT kind,data FROM pasar_guard_resources WHERE kind IN ('users','user_usage_logs')"):
            data = json.loads(row['data'])
            if row['kind'] == 'users':
                originals[int(data['id'])] = data
            else:
                uid = int(data['user_id'])
                history[uid] = history.get(uid, 0) + int(data['used_traffic_at_reset'] or 0)
                last_reset[uid] = max(last_reset.get(uid, 0), milliseconds(data['reset_at']))
        missing = db.execute('''SELECT a.*,p.source_user_id FROM accounts a
            LEFT JOIN pasar_guard_subscriptions p ON p.account_id=a.id
            WHERE NOT EXISTS (SELECT 1 FROM client_traffics t
            WHERE lower(trim(t.email))=lower(trim(a.email)))''').fetchall()
        rows = []
        reset_accounts = 0
        for account in missing:
            uid = account['source_user_id']
            source = originals.get(uid)
            if source is None:
                raise ValueError('Missing source usage for an account; no changes applied / اطلاعات مصرف مبدا کافی نیست؛ تغییری اعمال نشد')
            base = int(source['used_traffic'] or 0)
            source_reset = last_reset.get(uid, milliseconds(source['created_at']))
            if account['imported_last_reset_at'] > source_reset:
                current_base = 0
                reset_accounts += 1
            else:
                current_base = base
            counters = db.execute('''SELECT COALESCE(SUM(up),0),COALESCE(SUM(down),0),COALESCE(SUM(all_time),0),
                COALESCE(MIN(CASE WHEN inbound_id IN (SELECT id FROM inbounds) THEN inbound_id END),0)
                FROM account_inbounds WHERE account_id=?''', (account['id'],)).fetchone()
            rows.append((counters[3], bool(account['enable']) and not bool(account['representative_blocked']),
                         account['email'], counters[0], current_base + counters[1],
                         base + history.get(uid, 0) + counters[2], account['expiry_time'],
                         account['total_gb'], account['reset'], milliseconds(source.get('online_at'))))
        if apply:
            db.executemany('''INSERT INTO client_traffics
                (inbound_id,enable,email,up,down,all_time,expiry_time,total,reset,last_online)
                VALUES (?,?,?,?,?,?,?,?,?,?)''', rows)
            if db.execute('PRAGMA integrity_check').fetchone()[0] != 'ok':
                raise ValueError('Integrity check failed')
            db.commit()
        else:
            db.rollback()
        return {'applied': apply, 'missing_rows_restored' if apply else 'missing_rows_recoverable': len(rows),
                'accounts_reset_since_import': reset_accounts,
                'recovered_current_usage_bytes': sum(r[3] + r[4] for r in rows),
                'usage_on_deleted_inbounds_after_import': 'unknown; requires a backup before deletion',
                'existing_counters_and_accounts_changed': 0}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('database')
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    print(json.dumps(repair(args.database, args.apply), ensure_ascii=False, indent=2))
