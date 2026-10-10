#!/usr/bin/env bash
# Repair missing imported counters with a consistent backup and automatic restart.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo 'Run as root / با دسترسی روت اجرا کنید'; exit 1; }
db=${1:-/opt/vpn-ui/vpn-ui.db}
[[ -f "$db" ]] || { echo 'Database not found / دیتابیس پیدا نشد'; exit 1; }
command -v python3 >/dev/null
umask 077
work=$(mktemp -d /opt/vpn-ui/accounting-repair.XXXXXXXX)
active=0
systemctl is-active --quiet vpn-ui && active=1
cleanup() {
    if [[ $active == 1 ]]; then systemctl start vpn-ui; fi
}
trap cleanup EXIT
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/repair.py -o "$work/repair.py"
if [[ $active == 1 ]]; then systemctl stop vpn-ui; fi
python3 - "$db" "$work/before.db" <<'PY'
import sqlite3, sys
with sqlite3.connect(sys.argv[1]) as source, sqlite3.connect(sys.argv[2]) as backup:
    source.backup(backup)
PY
python3 "$work/repair.py" "$db" --apply
echo "Backup / بکاپ: $work/before.db"
echo 'Repair completed / ترمیم انجام شد'
