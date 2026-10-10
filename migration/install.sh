#!/usr/bin/env bash
# Install the migration bundle; vpn-ui must already be installed.
set -Eeuo pipefail
umask 077
LANGUAGE=${VPNUI_MIGRATION_LANG:-}
if [[ -z $LANGUAGE ]]; then
    printf 'Language / زبان: 1) فارسی  2) English [1]: '
    read -r LANGUAGE </dev/tty
    [[ $LANGUAGE == 2 ]] && LANGUAGE=en || LANGUAGE=fa
fi
[[ $LANGUAGE == fa || $LANGUAGE == en ]] || { echo 'VPNUI_MIGRATION_LANG must be fa or en' >&2; exit 1; }
export VPNUI_MIGRATION_LANG=$LANGUAGE
say() { if [[ $LANGUAGE == fa ]]; then printf '%s\n' "$2"; else printf '%s\n' "$1"; fi; }
[[ $EUID == 0 ]] || { say 'Run this command as root.' 'این دستور را با کاربر root اجرا کنید.' >&2; exit 1; }
[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]] || { say 'This bundle requires Linux amd64.' 'این بسته برای Linux amd64 است.' >&2; exit 1; }
if ! command -v python3 >/dev/null || ! command -v curl >/dev/null; then
    say 'Installing Python and curl…' 'در حال نصب پایتون و curl…'
    if command -v apt-get >/dev/null; then apt-get update; apt-get install -y python3 curl ca-certificates;
    elif command -v dnf >/dev/null; then dnf install -y python3 curl ca-certificates;
    else say 'Install python3 and curl first.' 'ابتدا python3 و curl را نصب کنید.' >&2; exit 1; fi
fi
BACKUP=${1:-}
if [[ -z $BACKUP ]]; then
    say 'Full path to PasarGuard backup:' 'مسیر کامل فایل بکاپ پاسارگارد:'
    read -r BACKUP </dev/tty
fi
[[ -f $BACKUP ]] || { say 'Backup file not found.' 'فایل بکاپ پیدا نشد.' >&2; exit 1; }
WORK=$(mktemp -d /root/vpn-ui-migration-install.XXXXXXXX)
say 'Downloading and verifying the migration bundle…' 'در حال دانلود و بررسی بسته مهاجرت…'
curl -fsSL --retry 3 https://api.github.com/repos/admin6501/vpn-ui/releases/latest -o "$WORK/release.json"
VERSION=$(python3 - "$WORK/release.json" <<'PY'
import json,re,sys
v=json.load(open(sys.argv[1]))['tag_name']
if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+',v): raise SystemExit('Invalid release version')
print(v)
PY
)
BASE="https://github.com/admin6501/vpn-ui/releases/download/$VERSION"
curl -fsSL --retry 3 "$BASE/vpn-ui-auto-migration.zip" -o "$WORK/vpn-ui-auto-migration.zip"
curl -fsSL --retry 3 "$BASE/SHA256SUMS" -o "$WORK/SHA256SUMS"
python3 - "$WORK" <<'PY'
import hashlib,pathlib,sys,zipfile
p=pathlib.Path(sys.argv[1]); name='vpn-ui-auto-migration.zip'
checks={line.split()[1].lstrip('*'):line.split()[0] for line in (p/'SHA256SUMS').read_text().splitlines()}
with (p/name).open('rb') as f: digest=hashlib.file_digest(f,'sha256').hexdigest()
if checks.get(name)!=digest: raise SystemExit('SHA256 mismatch; installation aborted')
with zipfile.ZipFile(p/name) as z:
 for entry in z.infolist():
  target=(p/'bundle'/entry.filename).resolve()
  if not target.is_relative_to((p/'bundle').resolve()): raise SystemExit('Unsafe archive path')
 z.extractall(p/'bundle')
PY
SCRIPT=$(find "$WORK/bundle" -name migrate.sh -type f -print -quit)
[[ -n $SCRIPT ]] || { say 'Migration script missing.' 'اسکریپت مهاجرت در بسته موجود نیست.' >&2; exit 1; }
chmod 700 "$(dirname "$SCRIPT")/pasarguard-db" "$(dirname "$SCRIPT")/vpn-ui-custom"
say 'Starting migration. The destination must have no users or inbounds.' 'شروع مهاجرت؛ مقصد باید بدون کاربر و ورودی باشد.'
bash "$SCRIPT" "$BACKUP"
