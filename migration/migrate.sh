#!/usr/bin/env bash
# Run from the unpacked migration bundle. Supports Linux amd64 + systemd.
set -Eeuo pipefail
umask 077
HERE=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
LANGUAGE=${VPNUI_MIGRATION_LANG:-}
if [[ -z $LANGUAGE ]]; then
    printf 'Language / زبان: 1) فارسی  2) English [1]: '
    read -r LANGUAGE </dev/tty
    [[ $LANGUAGE == 2 ]] && LANGUAGE=en || LANGUAGE=fa
fi
[[ $LANGUAGE == fa || $LANGUAGE == en ]] || { echo 'VPNUI_MIGRATION_LANG must be fa or en' >&2; exit 1; }
export VPNUI_MIGRATION_LANG=$LANGUAGE
say() { if [[ $LANGUAGE == fa ]]; then printf '%s\n' "$2"; else printf '%s\n' "$1"; fi; }
DRY_RUN=0
if [[ ${1:-} == --dry-run ]]; then DRY_RUN=1; shift; fi
BACKUP=${1:-}
if [[ -z $BACKUP ]]; then say 'Full path to PasarGuard backup:' 'مسیر کامل بکاپ پاسارگارد:'; read -r BACKUP </dev/tty; fi
[[ -f $BACKUP ]] || { say 'Backup file not found.' 'فایل بکاپ پیدا نشد.' >&2; exit 1; }
command -v python3 >/dev/null || { say 'python3 is required.' 'python3 لازم است.' >&2; exit 1; }
[[ $(uname -m) == x86_64 ]] || { say 'This bundle requires Linux amd64.' 'این بسته برای Linux amd64 است.' >&2; exit 1; }
UNIT=${VPNUI_SERVICE:-vpn-ui.service}
[[ $UNIT =~ ^[a-zA-Z0-9_.@-]+$ ]] || { say 'Invalid service name.' 'نام سرویس نامعتبر است.' >&2; exit 1; }
command -v systemctl >/dev/null || { say 'systemd is required.' 'systemd لازم است.' >&2; exit 1; }
INSTALLED=${VPNUI_BINARY:-}
if [[ -z $INSTALLED ]]; then
    EXEC=$(systemctl show "$UNIT" --property=ExecStart --value)
    INSTALLED=$(python3 - "$EXEC" <<'PY'
import sys,re
m=re.search(r'path=([^ ;}]+)',sys.argv[1])
if not m: raise SystemExit('Cannot detect service binary; set VPNUI_BINARY.')
print(m[1])
PY
)
fi
[[ -f $INSTALLED && $INSTALLED == /* ]] || { say 'Installed panel binary not found.' 'فایل اجرایی نصب موجود پیدا نشد.' >&2; exit 1; }
DB=${VPNUI_DATABASE:-}
if [[ -z $DB ]]; then
    ENVIRONMENT=$(systemctl show "$UNIT" --property=Environment --value)
    DB=$(python3 - "$INSTALLED" "$ENVIRONMENT" <<'PY'
import sys,shlex,pathlib
folder=pathlib.Path(sys.argv[1]).parent
for item in shlex.split(sys.argv[2]):
 if item.startswith('VPNUI_DB_FOLDER='): folder=pathlib.Path(item.split('=',1)[1])
print(folder/'vpn-ui.db')
PY
)
    ENVFILES=$(systemctl show "$UNIT" --property=EnvironmentFiles --value)
    [[ -z $ENVFILES ]] || { say 'Service uses EnvironmentFile; set VPNUI_DATABASE explicitly.' 'سرویس EnvironmentFile دارد؛ مسیر دیتابیس را با VPNUI_DATABASE مشخص کنید.' >&2; exit 1; }
fi
[[ -f $DB && $DB == /* ]] || { say 'Installed database not found; set VPNUI_DATABASE.' 'دیتابیس نصب موجود پیدا نشد؛ VPNUI_DATABASE را مشخص کنید.' >&2; exit 1; }
for f in convert.py preflight.py pasarguard-db vpn-ui-custom; do
    [[ -f $HERE/$f ]] || { say "Bundle file missing: $f" "فایل بسته موجود نیست: $f" >&2; exit 1; }
done
[[ $DRY_RUN == 1 || $EUID == 0 ]] || { say 'Run the migration as root or with sudo.' 'برای مهاجرت واقعی با sudo اجرا کنید.' >&2; exit 1; }
# Check empty destination and existing TLS before touching the running service.
python3 - "$DB" <<'PY'
import sqlite3,sys,pathlib,os
with sqlite3.connect(pathlib.Path(sys.argv[1]).as_uri()+'?mode=ro',uri=True) as c:
 tables={r[0] for r in c.execute("select name from sqlite_master where type='table'")}
 for t in ['accounts','inbounds']:
  if t in tables and c.execute('select count(*) from "'+t+'"').fetchone()[0]: raise SystemExit('مقصد خالی نیست؛ مهاجرت متوقف شد.' if os.environ.get('VPNUI_MIGRATION_LANG')=='fa' else 'Destination is not empty; migration refused.')
 settings=dict(c.execute('select key,value from settings'))
 for key in ['webCertFile','webKeyFile','subCertFile','subKeyFile']:
  value=settings.get(key,'')
  if value and not pathlib.Path(value).is_file(): raise SystemExit('Missing destination TLS file: '+key)
PY
WORK=$(mktemp -d "$(dirname -- "$DB")/pasarguard-migration.XXXXXXXX")
STOPPED=0
CHANGED=0
WAS_ACTIVE=0
rollback() {
    code=$?
    trap - EXIT
    if [[ $code != 0 && $STOPPED == 1 ]]; then
        say 'Migration failed; restoring the previous installation...' 'مهاجرت شکست خورد؛ بازگردانی نصب قبلی...' >&2
        systemctl stop "$UNIT" || true
        if [[ $CHANGED == 1 ]]; then
            cp -p -- "$WORK/previous-binary" "$INSTALLED"
            rm -f -- "${DB}-wal" "${DB}-shm"
            cp -p -- "$WORK/previous.db" "$DB"
        fi
        if [[ $WAS_ACTIVE == 1 ]]; then systemctl start "$UNIT" || true; fi
    fi
    if [[ $code != 0 ]]; then say "Diagnostics and rollback files: $WORK" "فایل‌های بررسی و بازگشت: $WORK" >&2; fi
    exit "$code"
}
trap rollback EXIT
"$HERE/pasarguard-db" "$WORK/schema" > "$WORK/schema.log" 2>&1
if systemctl is-active --quiet "$UNIT"; then WAS_ACTIVE=1; fi
if [[ $DRY_RUN == 0 ]]; then
    cp -p -- "$INSTALLED" "$WORK/previous-binary"
    systemctl stop "$UNIT"
    STOPPED=1
fi
python3 - "$DB" "$WORK/previous.db" <<'PY'
import sqlite3,sys,pathlib,os
with sqlite3.connect(pathlib.Path(sys.argv[1]).as_uri()+'?mode=ro',uri=True) as source, sqlite3.connect(sys.argv[2]) as target:
 source.backup(target)
PY
if [[ $DRY_RUN == 0 ]]; then
    chown --reference="$DB" "$WORK/previous.db"
    chmod --reference="$DB" "$WORK/previous.db"
fi
python3 "$HERE/convert.py" "$BACKUP" "$WORK/schema/vpn-ui.db" "$WORK/converted.db" --destination-db "$WORK/previous.db" > "$WORK/conversion.json"
if [[ $DRY_RUN == 0 ]]; then
    # Preserve service-user ownership and file permissions.
    chown --reference="$DB" "$WORK/converted.db"
    chmod --reference="$DB" "$WORK/converted.db"
    CHANGED=1
    cp -- "$HERE/vpn-ui-custom" "$INSTALLED"
    chmod --reference="$WORK/previous-binary" "$INSTALLED"
    chown --reference="$WORK/previous-binary" "$INSTALLED"
    rm -f -- "${DB}-wal" "${DB}-shm"
    mv -f -- "$WORK/converted.db" "$DB"
    systemctl start "$UNIT"
    sleep 3
    systemctl is-active --quiet "$UNIT" || { say 'The new service failed to start.' 'سرویس جدید فعال نشد.' >&2; exit 1; }
    STOPPED=0
fi
python3 - "$( [[ $DRY_RUN == 1 ]] && echo "$WORK/converted.db" || echo "$DB" )" <<'PY'
import sqlite3,sys,os
with sqlite3.connect(sys.argv[1]) as c:
 row=c.execute('select username from users where is_super_admin=1').fetchone()
 s=dict(c.execute('select key,value from settings'))
 fa=os.environ.get('VPNUI_MIGRATION_LANG')=='fa'
 print(('نام کاربری مدیر اصلی: ' if fa else 'Main administrator username: ')+row[0])
 print('رمز ورود: همان رمز قبلی این ادمین در پاسارگارد؛ رمز از هش قابل بازیابی نیست.' if fa else 'Password: the same administrator password as in PasarGuard; hashes cannot be recovered.')
 print(('پورت پنل: ' if fa else 'Panel port: ')+s.get('webPort','2083'))
 print(('مسیر پنل: ' if fa else 'Panel path: ')+s.get('webBasePath','/'))
PY
cat "$WORK/conversion.json"
if [[ $DRY_RUN == 1 ]]; then say 'Dry run succeeded; the installed panel was not changed.' 'تست تبدیل موفق بود؛ نصب موجود تغییر نکرد.'; else say 'Conversion completed and the destination service is active; verify VPN connections.' 'تبدیل انجام شد و سرویس مقصد فعال است؛ اتصال VPN را بررسی کنید.'; fi
say "Migration backup and report: $WORK" "بکاپ و گزارش مهاجرت: $WORK"
