#!/usr/bin/env python3
"""Read-only backup audit; prints counts only, never credentials."""
import argparse
import collections
import hashlib
import json
import re
import zipfile
from pathlib import Path

def unescape(v):
    if v == r'\N':
        return None
    escapes = {'n': '\n', 'r': '\r', 't': '\t', 'b': '\b', 'f': '\f', 'v': '\v', '\\': '\\'}
    return re.sub(r'\\([0-7]{1,3}|x[0-9a-fA-F]{1,2}|.)', lambda m:
        chr(int(m[1], 8)) if m[1][0] in '01234567' else
        chr(int(m[1][1:], 16)) if m[1].startswith('x') and len(m[1]) > 1 else
        escapes.get(m[1], m[1]), v)

def audit(path):
    with zipfile.ZipFile(path) as z:
        dumps = [n for n in z.namelist() if re.fullmatch(r'pg_dump/db-\d+\.sql', n)]
        if len(dumps) != 1:
            raise ValueError('Select exactly one panel database dump.')
        sql = z.read(dumps[0]).decode()
    tables = {}
    for m in re.finditer(r'^COPY public\.(\w+) \(([^\n]+)\) FROM stdin;\n(.*?)^\\\.$', sql, re.S | re.M):
        cols = m[2].split(', ')
        rows = []
        for line in m[3].splitlines():
            values = line.split('\t')
            if len(values) != len(cols):
                raise ValueError('Invalid COPY row in ' + m[1])
            rows.append(dict(zip(cols, map(unescape, values))))
        tables[m[1]] = rows
    users = tables['users']
    ps = [json.loads(u['proxy_settings']) for u in users]
    conflicts = lambda a,b,k: sum(bool(p.get(a,{}).get(k) and p.get(b,{}).get(k)) and p[a][k] != p[b][k] for p in ps)
    return {
        'backup_sha256': hashlib.sha256(Path(path).read_bytes()).hexdigest(),
        'users': len(users),
        'statuses': dict(collections.Counter(u['status'] for u in users)),
        'reset_strategies': dict(collections.Counter(u['data_limit_reset_strategy'] for u in users)),
        'different_vmess_vless_uuid': conflicts('vmess','vless','id'),
        'different_trojan_shadowsocks_password': conflicts('trojan','shadowsocks','password'),
        'nonempty_tables': {k:len(v) for k,v in tables.items()},
        'subscription_signing_key_present': bool(tables.get('jwt')),
        'import_ready': False,
        'blockers': [
            'Preserve protocol-specific UUIDs: target Account has one UUID.',
            'Implement verification and revocation of legacy subscription tokens.',
            'Map two existing nodes and their endpoints explicitly.',
            'Map next plans, calendar resets, administrators and historical usage.',
            'Reconcile live usage against a fresh final cutover backup.'
        ]
    }

if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('backup', type=Path)
    p.add_argument('--output', type=Path)
    a = p.parse_args()
    report = json.dumps(audit(a.backup), ensure_ascii=False, indent=2) + '\n'
    if a.output:
        a.output.write_text(report)
    else:
        print(report, end='')
