#!/usr/bin/env python3
"""Read-only migration diagnostics. Reports counts, never credentials or user identities."""
import json
from pathlib import Path
import sqlite3
import sys
from datetime import datetime, timezone

path = Path(sys.argv[1] if len(sys.argv) > 1 else '/opt/vpn-ui/vpn-ui.db').resolve()
if not path.is_file():
    raise SystemExit('Database not found / دیتابیس پیدا نشد')
now = int(datetime.now(timezone.utc).timestamp() * 1000)
with sqlite3.connect(path.as_uri() + '?mode=ro', uri=True) as db:
    db.execute('PRAGMA query_only=ON')
    tables = {row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type='table'")}
    if not {'accounts', 'client_traffics', 'settings'} <= tables:
        raise SystemExit('Incompatible database / دیتابیس ناسازگار است')
    settings = dict(db.execute('SELECT key,value FROM settings'))
    def count(sql, args=()):
        return db.execute(sql, args).fetchone()[0]
    result = {
        'checked_at_utc': datetime.now(timezone.utc).isoformat(),
        'accounts_marker_present': bool(settings.get('accountsMigratedAt')),
        'accounts': count('SELECT COUNT(*) FROM accounts'),
        'finite_quota_accounts': count('SELECT COUNT(*) FROM accounts WHERE total_gb>0'),
        'unlimited_quota_accounts': count('SELECT COUNT(*) FROM accounts WHERE total_gb=0'),
        'time_expired_accounts': count('SELECT COUNT(*) FROM accounts WHERE expiry_time>0 AND expiry_time<=?', (now,)),
        'quota_exhausted_traffic_rows': count('SELECT COUNT(*) FROM client_traffics WHERE total>0 AND up+down>=total'),
        'accounts_missing_traffic': count('SELECT COUNT(*) FROM accounts a WHERE NOT EXISTS (SELECT 1 FROM client_traffics t WHERE lower(trim(t.email))=lower(trim(a.email)))'),
        'quota_mismatch_rows': count('SELECT COUNT(*) FROM accounts a JOIN client_traffics t ON lower(trim(t.email))=lower(trim(a.email)) WHERE a.total_gb!=t.total'),
        'subscription_enabled': settings.get('subEnable') == 'true',
        'subscription_port': settings.get('subPort'),
        'subscription_path': settings.get('subPath'),
        'subscription_cert_exists': bool(settings.get('subCertFile')) and Path(settings['subCertFile']).is_file(),
        'subscription_key_exists': bool(settings.get('subKeyFile')) and Path(settings['subKeyFile']).is_file(),
    }
    if 'pasar_guard_subscriptions' in tables:
        result['imported_bindings'] = count('SELECT COUNT(*) FROM pasar_guard_subscriptions')
        result['imported_bindings_missing_accounts'] = count('SELECT COUNT(*) FROM pasar_guard_subscriptions p LEFT JOIN accounts a ON a.id=p.account_id WHERE a.id IS NULL')
        result['imported_bindings_rotated'] = count('SELECT COUNT(*) FROM pasar_guard_subscriptions p JOIN accounts a ON a.id=p.account_id WHERE a.sub_id!=p.sub_id_at_import')
    print(json.dumps(result, ensure_ascii=False, indent=2))
