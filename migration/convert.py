#!/usr/bin/env python3
"""Convert a PostgreSQL PasarGuard backup into a NEW custom vpn-ui database.

Never executes SQL from the archive and never changes the initialized input DB.
Requires the schema from this fork's pasarguard-db command, not stock vpn-ui.
"""
import argparse
import copy
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import secrets
import sqlite3
import zipfile
from urllib.parse import urlparse
from preflight import unescape


def load(path):
    with zipfile.ZipFile(path) as z:
        dumps = [n for n in z.namelist() if re.fullmatch(r'pg_dump/db-\d+\.sql', n)]
        if len(dumps) != 1:
            raise ValueError('Expected one panel database dump')
        sql = z.read(dumps[0]).decode()
    tables = {}
    for m in re.finditer(r'^COPY public\.(\w+) \(([^\n]+)\) FROM stdin;\n(.*?)^\\\.$', sql, re.S | re.M):
        cols = m[2].split(', ')
        rows = []
        for line in m[3].splitlines():
            vals = line.split('\t')
            if len(vals) != len(cols):
                raise ValueError('Invalid COPY data')
            rows.append(dict(zip(cols, map(unescape, vals))))
        tables[m[1]] = rows
    return tables


def ms(s):
    if not s:
        return 0
    d = datetime.fromisoformat(s)
    if d.tzinfo is None:
        d = d.replace(tzinfo=timezone.utc)
    return int(d.timestamp() * 1000)


def compact(v):
    return json.dumps(v, ensure_ascii=False, separators=(',', ':'))

def iso(s):
    if not s:
        return None
    d=datetime.fromisoformat(s)
    if d.tzinfo is None:
        d=d.replace(tzinfo=timezone.utc)
    return d.astimezone(timezone.utc).isoformat()


def insert(db, table, data):
    cols = ','.join('"'+c+'"' for c in data)
    db.execute('INSERT INTO "'+table+'" ('+cols+') VALUES ('+','.join('?' for _ in data)+')', tuple(data.values()))


def convert(backup, schema, output, destination_db=None):
    if output.exists():
        raise ValueError('Output already exists; choose a new file')
    destination_settings = None
    if destination_db is not None:
        with sqlite3.connect(destination_db.resolve().as_uri()+'?mode=ro', uri=True) as destination:
            tables = {r[0] for r in destination.execute("SELECT name FROM sqlite_master WHERE type='table'")}
            if 'settings' not in tables or 'inbounds' not in tables:
                raise ValueError('Destination backup must be a vpn-ui SQLite database')
            for table in ['accounts', 'inbounds']:
                if table in tables and destination.execute('SELECT COUNT(*) FROM "'+table+'"').fetchone()[0]:
                    raise ValueError('Destination must have no VPN users or inbounds')
            destination_settings = dict(destination.execute('SELECT key,value FROM settings'))
    source = load(backup)
    if not source.get('users') or not source.get('admins') or not source.get('jwt'):
        raise ValueError('Missing required source tables')
    roles = {int(r['id']): r for r in source['admin_roles']}
    owners = [a for a in source['admins'] if roles[int(a['role_id'])]['is_owner'] == 't']
    if len(owners) != 1:
        raise ValueError('Expected exactly one owner; explicit ownership mapping required')
    if any(a['hashed_password'].split('$')[1] not in ['2a', '2b', '2y'] for a in source['admins']):
        raise ValueError('Unsupported admin password format')
    usernames = [u['username'].strip().lower() for u in source['users']]
    if len(set(usernames)) != len(usernames):
        raise ValueError('Case-insensitive username collision in destination model')
    if any(u['status'] == 'on_hold' for u in source['users']):
        raise ValueError('On-hold lifecycle requires a separate adapter')
    if source.get('next_plans'):
        raise ValueError('Next-plan execution is not supported by this converter')
    db = sqlite3.connect(schema.as_uri()+'?mode=ro', uri=True)
    if db.execute('SELECT COUNT(*) FROM accounts').fetchone()[0] or db.execute('SELECT COUNT(*) FROM inbounds').fetchone()[0]:
        raise ValueError('Schema database must have no VPN accounts or inbounds')
    if db.execute('SELECT COUNT(*) FROM users').fetchone()[0] > 1:
        raise ValueError('Schema database is not empty')
    columns = {r[1] for r in db.execute('PRAGMA table_info(accounts)')}
    if not {'protocol_credentials', 'imported_reset_days', 'representative_blocked'} <= columns:
        raise ValueError('Use the custom fork schema, not the stock panel')
    temporary = output.with_name(output.name+'.tmp-'+secrets.token_hex(6))
    fd = os.open(temporary, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    os.close(fd)
    target = sqlite3.connect(temporary)
    db.backup(target)
    db.close()
    try:
        with target:
            for table in ['users', 'representative_roles', 'reseller_profiles', 'reseller_clients', 'inbound_accesses', 'pasar_guard_resources', 'pasar_guard_subscriptions', 'pasar_guard_secrets']:
                target.execute('DELETE FROM "'+table+'"')
            owner_id = int(owners[0]['id'])
            for r in source['admin_roles']:
                insert(target, 'representative_roles', {
                    'id': int(r['id']), 'name': r['name'], 'is_owner': r['is_owner'] == 't',
                    'permissions': r['permissions'], 'limits': r['limits'], 'features': r['features'], 'access': r['access'],
                    'disabled_when_limited': r['disabled_when_limited'] == 't',
                    'disconnect_users_when_limited': r.get('disconnect_users_when_limited', r.get('disable_users_when_limited', 't')) == 't',
                    'disconnect_users_when_disabled': r.get('disconnect_users_when_disabled', r.get('disable_users_when_disabled', 't')) == 't',
                })
            for a in source['admins']:
                aid = int(a['id']); owner = aid == owner_id
                insert(target, 'users', {'id': aid, 'username': a['username'], 'password': a['hashed_password'],
                    'is_super_admin': owner, 'is_reseller': not owner, 'representative_role_id': int(a['role_id']),
                    'enable': a['status'] != 'disabled', 'permissions': 0,
                    'representative_overrides': a.get('permission_overrides') or ''})
                if not owner:
                    insert(target, 'reseller_profiles', {'user_id': aid, 'consumption_based': True,
                        'usage_base': int(a['used_traffic'] or 0), 'allowance_bytes': int(a['data_limit'] or 0),
                        'unlimited': not int(a['data_limit'] or 0), 'allow_external_proxy': True, 'created_by': owner_id})
            insert(target, 'pasar_guard_secrets', {'id': 1, 'signing_key': source['jwt'][0]['secret_key']})
            # Preserve full records, including host fields unsupported by the native UI.
            for kind in ['hosts', 'groups', 'inbounds', 'core_configs', 'settings', 'user_usage_logs', 'user_hwids', 'users', 'admins', 'admin_roles', 'inbounds_groups_association']:
                for idx, row in enumerate(source.get(kind, [])):
                    insert(target, 'pasar_guard_resources', {'kind': kind, 'source_id': int(row.get('id') or idx+1), 'data': compact(row)})
            inbound_by_tag = {}
            existing_template = target.execute('SELECT value FROM settings WHERE key=?', ('xrayTemplateConfig',)).fetchone()
            if not existing_template:
                raise ValueError('Initialize the schema with pasarguard-db first')
            template = json.loads(existing_template[0])
            template['outbounds'] = []
            # Retain the target API routing and stats infrastructure. Source rules
            # remain scoped to their original inbound sets.
            template['routing']['rules'] = [r for r in template.get('routing', {}).get('rules', []) if r.get('outboundTag') == 'api']
            outbound_tags = {}
            next_id = 1
            for core in source['core_configs']:
                cfg = json.loads(core['config'])
                if core['type'] != 'xray':
                    raise ValueError('Non-Xray core requires an adapter')
                # Scope outbound tags to each source core to preserve distinct routes.
                prefix = 'pg-core-'+core['id']+'-'
                tags = {o.get('tag'): prefix+o['tag'] for o in cfg.get('outbounds', []) if o.get('tag')}
                for outbound in cfg.get('outbounds', []):
                    outbound = copy.deepcopy(outbound)
                    if outbound.get('tag'):
                        outbound['tag'] = tags[outbound['tag']]
                    proxy = outbound.get('proxySettings', {})
                    if proxy.get('tag') in tags:
                        proxy['tag'] = tags[proxy['tag']]
                    template['outbounds'].append(outbound)
                core_tags = [i['tag'] for i in cfg.get('inbounds', [])]
                for rule in cfg.get('routing', {}).get('rules', []):
                    rule = copy.deepcopy(rule)
                    if rule.get('outboundTag') in tags:
                        rule['outboundTag'] = tags[rule['outboundTag']]
                    if rule.get('balancerTag'):
                        raise ValueError('Routing balancer requires explicit conversion')
                    if not rule.get('inboundTag'):
                        rule['inboundTag'] = core_tags
                    template['routing']['rules'].append(rule)
                if cfg.get('outbounds') and cfg['outbounds'][0].get('tag'):
                    template['routing']['rules'].append({'type':'field','inboundTag':core_tags,'outboundTag':tags[cfg['outbounds'][0]['tag']]})
                for i in cfg.get('inbounds', []):
                    tag = i['tag']
                    if tag in inbound_by_tag:
                        raise ValueError('Duplicate inbound tag across cores')
                    if i['protocol'] not in ['vmess', 'vless', 'shadowsocks', 'trojan']:
                        raise ValueError('Unsupported inbound protocol')
                    settings = copy.deepcopy(i.get('settings', {})); settings['clients'] = []
                    if i['protocol'] == 'shadowsocks':
                        settings.setdefault('password', '')
                    stream = copy.deepcopy(i.get('streamSettings', {})); stream.setdefault('network', 'tcp'); stream.setdefault('security', 'none')
                    hosts = [h for h in source['hosts'] if h['inbound_tag'] == tag and h['is_disabled'] != 't']
                    for h in hosts:
                        if any(h.get(k) not in [None, '', 'null', '{}', '[]', 'f'] for k in ['sni', 'host', 'path', 'alpn', 'http_headers', 'transport_settings']):
                            raise ValueError('Per-host transport overrides need a dedicated adapter')
                        if '{' in h['address']:
                            raise ValueError('Dynamic host address requires explicit mapping')
                    stream['externalProxy'] = [{'dest': h['address'], 'port': int(h['port'] or i['port']),
                        'remark': h['remark'], 'forceTls': 'same' if h['security'] == 'inbound_default' else h['security']} for h in hosts]
                    insert(target, 'inbounds', {'id': next_id, 'user_id': owner_id, 'enable': True, 'listen': i.get('listen', ''),
                        'port': int(i['port']), 'protocol': i['protocol'], 'settings': compact(settings),
                        'stream_settings': compact(stream), 'tag': tag, 'remark': tag, 'sniffing': compact(i.get('sniffing', {}))})
                    inbound_by_tag[tag] = next_id; next_id += 1
            tags_by_id = {r['id']: r['tag'] for r in source['inbounds']}
            groups = {r['id']: r for r in source['groups']}
            groups_inbounds = {}
            for r in source['inbounds_groups_association']:
                tag = tags_by_id[r['inbound_id']]
                if tag not in inbound_by_tag:
                    raise ValueError('Group references an inbound missing from core config')
                groups_inbounds.setdefault(r['group_id'], set()).add(inbound_by_tag[tag])
            user_groups = {}
            for r in source['users_groups_association']:
                user_groups.setdefault(r['user_id'], []).append(r['groups_id'])
            admins = {a['id']: a for a in source['admins']}
            historical = {}; last_reset = {}
            for r in source.get('user_usage_logs', []):
                historical[r['user_id']] = historical.get(r['user_id'], 0) + int(r['used_traffic_at_reset'])
                last_reset[r['user_id']] = max(last_reset.get(r['user_id'], 0), ms(r['reset_at']))
            for u in source['users']:
                uid = int(u['id']); credentials = json.loads(u['proxy_settings']); subid = secrets.token_urlsafe(24)
                active = u['status'] == 'active'; memberships = {}
                for gid in user_groups.get(u['id'], []):
                    for iid in groups_inbounds.get(gid, set()):
                        memberships[iid] = memberships.get(iid, False) or groups[gid]['is_disabled'] != 't'
                aid = int(u['admin_id'] or owner_id)
                if str(aid) not in admins:
                    raise ValueError('User references a missing administrator')
                used = int(u['used_traffic'] or 0); lifetime = used + historical.get(u['id'], 0)
                insert(target, 'accounts', {'id': uid, 'email': u['username'], 'sub_id': subid,
                    'uuid': credentials.get('vmess', credentials.get('vless', {})).get('id', ''),
                    'password': credentials.get('trojan', credentials.get('shadowsocks', {})).get('password', ''),
                    'auth': credentials.get('hysteria', {}).get('auth', ''), 'security': 'auto',
                    'protocol_credentials': compact(credentials), 'total_gb': int(u['data_limit'] or 0),
                    'expiry_time': ms(u['expire']), 'enable': active, 'comment': u['note'] or '',
                    'created_at': ms(u['created_at']), 'updated_at': ms(u['edit_at'] or u['created_at']),
                    'imported_reset_days': {'no_reset': 0, 'day': 1, 'week': 7, 'month': 30, 'year': 365}[u['data_limit_reset_strategy']],
                    'imported_last_reset_at': last_reset.get(u['id'], ms(u['created_at'])),
                    'imported_reset_disabled': u['status'] not in ['active', 'limited']})
                home = min(memberships) if memberships else 0
                insert(target, 'client_traffics', {'email': u['username'], 'inbound_id': home, 'enable': active,
                    'up': 0, 'down': used, 'all_time': lifetime, 'total': int(u['data_limit'] or 0), 'expiry_time': ms(u['expire']), 'last_online': ms(u['online_at'])})
                if aid != owner_id:
                    insert(target, 'reseller_clients', {'email': u['username'], 'user_id': aid, 'inbound_id': home, 'charged_bytes': 0, 'all_time_base': lifetime})
                insert(target, 'pasar_guard_subscriptions', {'source_user_id': uid, 'username': u['username'], 'account_id': uid,
                    'sub_id_at_import': subid, 'created_at': iso(u['created_at']), 'revoked_at': iso(u['sub_revoked_at'])})
                for iid, enabled in memberships.items():
                    inbound = target.execute('SELECT protocol,settings FROM inbounds WHERE id=?', (iid,)).fetchone()
                    proto, settings = inbound[0], json.loads(inbound[1])
                    client = dict(credentials.get(proto, {})); client.update({'email': u['username'], 'subId': subid, 'enable': active and enabled,
                        'security': 'auto', 'totalGB': int(u['data_limit'] or 0), 'expiryTime': ms(u['expire']), 'comment': u['note'] or '', 'reset': 0})
                    settings['clients'].append(client)
                    target.execute('UPDATE inbounds SET settings=? WHERE id=?', (compact(settings), iid))
                    insert(target, 'account_inbounds', {'account_id': uid, 'inbound_id': iid, 'enable': enabled, 'extra': compact(client), 'created_at': ms(u['created_at'])})
            for a in source['admins']:
                if int(a['id']) == owner_id:
                    continue
                access = json.loads(roles[int(a['role_id'])]['access']); allowed = access.get('allowed_group_ids')
                grants = set(inbound_by_tag.values()) if allowed is None else set().union(*(groups_inbounds.get(str(g), set()) for g in allowed))
                for iid in grants:
                    insert(target, 'inbound_accesses', {'user_id': int(a['id']), 'inbound_id': iid})
            # Write the merged core through the panel's native setting.
            target.execute('DELETE FROM settings WHERE key=?', ('xrayTemplateConfig',))
            insert(target, 'settings', {'key': 'xrayTemplateConfig', 'value': compact(template)})
            # The source ran directly: preserve its subscription listener port and
            # route. Do not bind the management UI to the same TCP port.
            with zipfile.ZipFile(backup) as z:
                env = {}
                for line in z.read('.env').decode().splitlines():
                    if '=' in line and not line.lstrip().startswith('#'):
                        key,value=line.split('=',1);env[key.strip()]=value.strip().strip('"').strip("'")
            sub_settings=json.loads(source['settings'][0]['subscription'])
            prefix=sub_settings.get('url_prefix') or ''
            parsed=urlparse(prefix)
            port=parsed.port or int(env.get('UVICORN_PORT','8000'))
            subpath=env.get('XRAY_SUBSCRIPTION_PATH') or env.get('SUBSCRIPTION_PATH') or 'sub'
            configured={'subEnable':'true','subPort':str(port),'subPath':'/'+subpath.strip('/')+'/',
                'webPort':'2083' if port!=2083 else '2084','subJsonEnable':'true','subClashEnable':'true'}
            if prefix:
                configured['subURI']=prefix.rstrip('/')+'/'+subpath.strip('/')+'/'
            if env.get('UVICORN_SSL_CERTFILE'):
                configured['subCertFile']='/etc/vpn-ui/pasarguard-certs/fullchain.pem'
                configured['subKeyFile']='/etc/vpn-ui/pasarguard-certs/privkey.pem'
                configured['webCertFile']=configured['subCertFile']
                configured['webKeyFile']=configured['subKeyFile']
            if destination_settings is not None:
                # Preserve the NEW server's listener/domain/TLS settings, never source TLS.
                keys = ['webListen','webDomain','webPort','webBasePath','webCertFile','webKeyFile',
                        'subEnable','subListen','subDomain','subPort','subPath','subURI','subCertFile','subKeyFile',
                        'subJsonEnable','subJsonPath','subClashEnable','subClashPath']
                configured = {key: destination_settings[key] for key in keys if key in destination_settings}
                for key in keys:
                    target.execute('DELETE FROM settings WHERE key=?',(key,))
            for key,value in configured.items():
                target.execute('DELETE FROM settings WHERE key=?',(key,))
                insert(target,'settings',{'key':key,'value':value})
            if target.execute('PRAGMA integrity_check').fetchone()[0] != 'ok':
                raise ValueError('SQLite integrity check failed')
        target.close()
        os.link(temporary, output)  # Refuse a destination appearing concurrently.
        temporary.unlink()
    except Exception:
        target.close(); temporary.unlink(missing_ok=True); raise
    return {'users': len(source['users']), 'admins': len(source['admins']), 'roles': len(roles),
        'inbounds': len(inbound_by_tag), 'hosts_retained': len(source['hosts']), 'nodes_imported': 0,
        'live_connection_tested': False, 'source_backup_is_sample': True}


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('backup', type=Path); p.add_argument('schema', type=Path); p.add_argument('output', type=Path)
    p.add_argument('--destination-db', type=Path, help='Empty freshly installed vpn-ui database backup; preserve its domain, ports and TLS paths')
    a = p.parse_args()
    print(json.dumps(convert(a.backup, a.schema.resolve(), a.output.resolve(), a.destination_db), ensure_ascii=False, indent=2))
