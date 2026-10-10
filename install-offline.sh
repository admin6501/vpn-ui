#!/usr/bin/env bash
# Install/update from a local release binary, with an optional apt prerequisite step.
set -Eeuo pipefail
umask 077
lang=${VPNUI_LANG:-fa}
if [[ ${1:-} == --lang ]]; then lang=${2:-}; shift 2; fi
say() { if [[ $lang == fa ]]; then printf '%s\n' "$1"; else printf '%s\n' "$2"; fi; }
die() { say "$1" "$2" >&2; exit 1; }
# This helper is sourceable for tests; the actual installer still needs only two files.
offer_apt_prerequisites() {
    if ! command -v apt-get >/dev/null 2>&1; then
        say 'apt موجود نیست؛ پیش‌نیازهای سیستم‌عامل را با مدیر بستهٔ توزیع خود نصب کنید.' 'apt is unavailable; use your distribution package manager for OS prerequisites.'
        return 0
    fi
    local answer pkg kernel_version
    local -a packages=(ca-certificates curl unzip python3 sqlite3 iproute2 nftables kmod openssl cron coreutils gawk build-essential dkms pkg-config)
    say 'پیش‌نیازهای قابل نصب با apt:' 'Prerequisites available through apt:'
    printf '  %s\n' "${packages[*]}"
    say 'هدرها و ماژول‌های اضافی کرنل فعلی هم، اگر در مخزن موجود باشند، نصب می‌شوند.' 'Matching running-kernel headers and extra modules are included when available in the repository.'
    while true; do
        say 'این پیش‌نیازها با apt نصب شوند؟ اینترنت لازم است. Yes/No [No]:' 'Install these prerequisites with apt? Internet access is required. Yes/No [No]:'
        if ! IFS= read -r answer; then answer=no; fi
        case "${answer,,}" in
            y|yes|بله) break ;;
            ''|n|no|خیر)
                say 'نصب apt رد شد؛ نصب پنل از فایل محلی ادامه دارد.' 'apt installation skipped; continuing with the local panel binary.'
                return 0 ;;
            *) say 'فقط Yes یا No وارد کنید.' 'Please enter Yes or No.' ;;
        esac
    done
    # This is the only optional online step. It happens before stopping the panel.
    local -a apt_options=(-o Acquire::Retries=0 -o Acquire::http::Timeout=20 -o Acquire::https::Timeout=20)
    if ! DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get "${apt_options[@]}" update; then
        say 'به‌روزرسانی مخزن apt شکست خورد؛ پیش‌نیازها نصب نشدند. نصب محلی پنل ادامه دارد.' 'apt index refresh failed; prerequisites were not installed. Continuing local panel installation.'
        return 0
    fi
    kernel_version=$(uname -r)
    for pkg in "linux-headers-$kernel_version" "linux-modules-extra-$kernel_version"; do
        if command -v apt-cache >/dev/null 2>&1 && apt-cache show "$pkg" >/dev/null 2>&1; then
            packages+=("$pkg")
        else
            say "بستهٔ اختیاری کرنل در مخزن موجود نیست، رد شد: $pkg" "Kernel package unavailable (optional, skipped): $pkg"
        fi
    done
    if DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get "${apt_options[@]}" \
        -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold \
        install -y --no-install-recommends "${packages[@]}"; then
        say 'نصب پیش‌نیازهای apt انجام شد؛ راه‌اندازی هر پروتکل را از صفحهٔ هسته‌ها بررسی کنید.' 'apt prerequisites installed; verify each protocol in the cores page.'
    else
        say 'نصب apt کامل نشد؛ خروجی بالا را بررسی کنید. نصب محلی پنل ادامه دارد.' 'apt installation did not complete; check the output above. Continuing local panel installation.'
    fi
    return 0
}

# Sourcing exposes the helper without installing anything.
[[ ${BASH_SOURCE[0]} == "$0" ]] || return 0

if [[ ${1:-} == --help ]]; then
    printf 'sudo bash install-offline.sh [--lang fa|en] [/path/vpn-ui-{amd64,arm64}]\nVPNUI_PUBLIC_IP=server-ip (optional, for the printed URL)\n'; exit 0
fi
[[ $lang == fa || $lang == en ]] || die 'زبان باید fa یا en باشد.' 'Language must be fa or en.'
[[ $EUID == 0 ]] || die 'اسکریپت را با sudo یا کاربر root اجرا کنید.' 'Run with sudo or as root.'
[[ $(uname -s) == Linux ]] || die 'این بسته فقط Linux است.' 'Linux is required.'
case $(uname -m) in
    x86_64|amd64) asset=vpn-ui-amd64 ;;
    aarch64|arm64) asset=vpn-ui-arm64 ;;
    *) die 'معماری پشتیبانی نمی‌شود.' 'Supported architectures: amd64 and arm64.' ;;
esac
[[ -d /run/systemd/system ]] || die 'systemd باید مدیر سرویس فعال سیستم باشد.' 'A running systemd system is required.'
for command in systemctl install cp mv mktemp timeout sha256sum awk readlink; do
    command -v "$command" >/dev/null || die "پیش‌نیاز موجود نیست: $command" "Missing prerequisite: $command"
done
source_binary=${1:-"$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/$asset"}
[[ -f $source_binary && ! -L $source_binary ]] || die 'فایل باینری محلی پیدا نشد.' 'Local binary not found (symlinks are not accepted).'
source_binary=$(readlink -f -- "$source_binary")
base=/opt/vpn-ui
mkdir -p "$base"
# Freeze a private candidate before executing/verifying it.
stage=$(mktemp -d "$base/.offline-install.XXXXXXXX")
trap 'rm -rf -- "$stage"' EXIT
install -m 700 -- "$source_binary" "$stage/vpn-ui-amd64"
checksums="$(dirname -- "$source_binary")/SHA256SUMS"
if [[ -f $checksums ]]; then
    expected=$(awk -v asset="$asset" '$2 == asset {print $1}' "$checksums")
    [[ $expected =~ ^[a-fA-F0-9]{64}$ ]] || die 'چک‌سام باینری معتبر نیست.' 'Missing or ambiguous binary checksum.'
    actual=$(sha256sum "$stage/vpn-ui-amd64"); actual=${actual%% *}
    [[ ${expected,,} == "$actual" ]] || die 'چک‌سام باینری تطابق ندارد.' 'Binary checksum mismatch.'
else
    say 'SHA256SUMS موجود نیست؛ صحت نسخه را از منبع مطمئن بررسی کنید.' 'SHA256SUMS absent; use a binary from a trusted source.'
fi
export VPNUI_OFFLINE=1
version=$(timeout 30 "$stage/vpn-ui-amd64" -v)
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die 'نسخهٔ باینری معتبر نیست.' 'Invalid panel binary version.'
offer_apt_prerequisites
unit=vpn-ui
if [[ -x $base/vpn-ui-amd64 ]]; then
    unit=$(timeout 30 "$base/vpn-ui-amd64" info --get systemdUnit)
    [[ $unit =~ ^[a-zA-Z0-9_-]+$ ]] || die 'نام سرویس فعلی معتبر نیست.' 'Invalid existing service name.'
fi
unit_file=/etc/systemd/system/$unit.service
dropin=/etc/systemd/system/$unit.service.d/90-offline.conf
# Custom storage/launch paths must not silently be replaced by this installer.
if systemctl cat "$unit" >/dev/null 2>&1; then
    launch=$(systemctl show "$unit" -p ExecStart --value)
    [[ $launch == *"$base/vpn-ui-amd64"* ]] || die 'مسیر سرویس سفارشی است؛ نصب متوقف شد.' 'Custom service executable; refusing to replace it.'
    envfiles=$(systemctl show "$unit" -p EnvironmentFiles --value)
    [[ -z $envfiles ]] || die 'سرویس EnvironmentFile سفارشی دارد؛ نصب دستی انجام دهید.' 'Custom EnvironmentFile; use the manual update procedure.'
    envs=$(systemctl show "$unit" -p Environment --value)
    [[ $envs != *VPNUI_DB_FOLDER=* ]] || die 'مسیر دیتابیس سفارشی است؛ از راهنمای نصب استفاده کنید.' 'Custom database directory; use the documented manual procedure.'
fi
backup=$(mktemp -d "$base/offline-backup.XXXXXXXX")
chmod 700 "$backup"
was_active=0; systemctl is-active --quiet "$unit" && was_active=1
was_enabled=0; systemctl is-enabled --quiet "$unit" && was_enabled=1
systemctl stop "$unit" 2>/dev/null || { [[ $was_active == 0 ]] || die 'توقف سرویس ممکن نشد.' 'Cannot stop the existing service.'; }
# A failed backup must restart the original service before any replacement.
trap 'if [[ $was_active == 1 ]]; then systemctl start "$unit" || true; fi; exit 1' ERR
# SQLite sidecars are copied only after the writer has stopped.
for name in vpn-ui-amd64 vpn-ui.db vpn-ui.db-wal vpn-ui.db-shm; do
    [[ ! -e $base/$name ]] || cp -a -- "$base/$name" "$backup/$name"
done
[[ ! -e $unit_file ]] || cp -a "$unit_file" "$backup/unit"
[[ ! -e $dropin ]] || cp -a "$dropin" "$backup/dropin"
[[ ! -e /usr/bin/vpn-ui ]] || cp -a /usr/bin/vpn-ui "$backup/menu"
rollback() {
    trap - ERR
    systemctl stop "$unit" 2>/dev/null || true
    for name in vpn-ui-amd64 vpn-ui.db vpn-ui.db-wal vpn-ui.db-shm; do
        rm -f -- "$base/$name"
        [[ ! -e $backup/$name ]] || cp -a -- "$backup/$name" "$base/$name"
    done
    rm -f "$unit_file" "$dropin" /usr/bin/vpn-ui
    [[ ! -e $backup/unit ]] || cp -a "$backup/unit" "$unit_file"
    [[ ! -e $backup/dropin ]] || cp -a "$backup/dropin" "$dropin"
    [[ ! -e $backup/menu ]] || cp -a "$backup/menu" /usr/bin/vpn-ui
    systemctl daemon-reload || true
    if [[ $was_enabled == 0 ]]; then systemctl disable "$unit" 2>/dev/null || true; fi
    if [[ $was_active == 1 ]]; then systemctl start "$unit" || true; fi
    say "نصب ناموفق بود؛ نسخهٔ قبلی بازگردانده شد. بکاپ: $backup" "Installation failed; previous files restored. Backup: $backup" >&2
    exit 1
}
trap rollback ERR
install -m 755 "$stage/vpn-ui-amd64" "$base/.vpn-ui-amd64.new"
mv -f "$base/.vpn-ui-amd64.new" "$base/vpn-ui-amd64"
cd "$base"
fresh=0
if [[ ! -s vpn-ui.db ]]; then
    fresh=1
    ./vpn-ui-amd64 --random > "$backup/login.txt" 2>&1
fi
./vpn-ui-amd64 install-menu > "$backup/menu-install.log" 2>&1
if [[ ! -f $unit_file ]] && ! systemctl cat "$unit" >/dev/null 2>&1; then
    cat > "$unit_file" <<UNIT
[Unit]
Description=VPN-UI panel
After=network.target
[Service]
Type=simple
User=root
Group=root
LimitNOFILE=1048576
WorkingDirectory=$base
ExecStart=$base/vpn-ui-amd64
Restart=on-failure
RestartSec=5
[Install]
WantedBy=multi-user.target
UNIT
fi
mkdir -p "$(dirname "$dropin")"
printf '[Service]\nEnvironment=VPNUI_OFFLINE=1\n' > "$dropin"
systemctl daemon-reload
systemctl enable "$unit" > "$backup/service.log" 2>&1
systemctl start "$unit"
sleep 2
systemctl is-active --quiet "$unit"
port=$(./vpn-ui-amd64 info --get port)
path=$(./vpn-ui-amd64 info --get webBasePath)
username=$(./vpn-ui-amd64 info --get username)
[[ $port =~ ^[0-9]+$ && -n $username && -n $path ]]
trap - ERR
say "نصب نسخه $version انجام شد. بکاپ: $backup" "Installed version $version. Backup: $backup"
if [[ $fresh == 1 ]]; then cat "$backup/login.txt"; else say "نام کاربری: $username؛ رمز ورود قبلی حفظ شده است." "Username: $username; existing password preserved."; fi
say "پورت: $port — مسیر: $path" "Port: $port — path: $path"
say 'برای ورود از IP عمومی یا دامنهٔ سرور استفاده کنید. نصب پنل به معنی آماده بودن همهٔ پروتکل‌ها نیست؛ وضعیت هسته‌ها را بررسی کنید.' 'Open the panel using the server public IP/domain. Check the cores page: OS prerequisites for some protocols may still be needed.'
say 'راهنمای فعال‌سازی دوبارهٔ دانلودها: docs/OFFLINE-INSTALL.md' 'To re-enable downloads, see docs/OFFLINE-INSTALL.md.'
