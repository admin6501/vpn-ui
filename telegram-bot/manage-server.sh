#!/usr/bin/env bash
set -Eeuo pipefail
APP=/opt/vpn-ui-telegram-bot
CONF=/etc/vpn-ui-bot
DATA=/var/lib/vpn-ui-bot
NODE=/opt/vpn-ui-bot-node/bin/node
HERE=$(cd -- "$(dirname -- "$0")" && pwd)
[[ $EUID == 0 ]] || { echo 'لطفاً با sudo اجرا کنید.'; exit 1; }
pause(){ read -r -p 'برای ادامه Enter بزنید... ' _ || true; }
install_bot(){
 command -v apt-get >/dev/null || { echo 'این نصب‌کننده برای Ubuntu/Debian است.'; return 1; }
 [[ -f "$HERE/server.mjs" && -f "$HERE/worker.js" ]] || { echo 'فایل‌های ربات باید کنار اسکریپت باشند.'; return 1; }
 if [[ -f "$CONF/config.json" || -e "$DATA/bot.sqlite" ]]; then echo 'نصب یا دیتابیس قبلی وجود دارد. برای جلوگیری از جایگزینی، نصب تازه متوقف شد.'; return 1; fi
 apt-get update && apt-get install -y ca-certificates curl xz-utils python3
 local arch; case "$(uname -m)" in x86_64) arch=x64;; aarch64) arch=arm64;; *) echo 'معماری پشتیبانی نمی‌شود.'; return 1;; esac
 local version=v22.18.0 tmp; tmp=$(mktemp -d)
 curl -fsSL "https://nodejs.org/dist/$version/node-$version-linux-$arch.tar.xz" -o "$tmp/node.tar.xz"
 curl -fsSL "https://nodejs.org/dist/$version/SHASUMS256.txt" -o "$tmp/SHASUMS256.txt"
 (cd "$tmp"; local expected; expected=$(awk -v f="node-$version-linux-$arch.tar.xz" '$2==f {print $1}' SHASUMS256.txt); [[ -n "$expected" ]]; echo "$expected  node.tar.xz" | sha256sum -c -)
 mkdir -p /opt/vpn-ui-bot-node "$APP" "$CONF" "$DATA"
 tar -xJf "$tmp/node.tar.xz" --strip-components=1 -C /opt/vpn-ui-bot-node
 rm -rf "$tmp"
 chmod 700 "$CONF" "$DATA"
 cp "$HERE"/{worker.js,qr.js,panel-config.js,wallet.js,server.mjs,plan-migration.mjs,schema.sql,package.json,package-lock.json,manage-server.sh} "$APP/"
 chmod 700 "$APP/manage-server.sh"
 ln -sfn "$APP/manage-server.sh" /usr/local/bin/vpn-ui-bot
 (cd "$APP"; PATH="/opt/vpn-ui-bot-node/bin:$PATH" npm ci --omit=dev --no-audit --no-fund)
 local bot_token admin_id
 read -r -s -p 'توکن ربات BotFather: ' bot_token; echo
 read -r -p 'شناسه عددی ادمین تلگرام: ' admin_id
 [[ "$bot_token" =~ ^[0-9]+:[A-Za-z0-9_-]+$ && "$admin_id" =~ ^[0-9]+(,[0-9]+)*$ ]] || { echo 'توکن یا شناسه معتبر نیست.'; return 1; }
 export BOT_SETUP_TOKEN="$bot_token" BOT_SETUP_ADMIN="$admin_id"
 umask 077
 python3 -c 'import json,os; json.dump({"BOT_TOKEN":os.environ["BOT_SETUP_TOKEN"],"ADMIN_ID":os.environ["BOT_SETUP_ADMIN"]},open("/etc/vpn-ui-bot/config.json","w"))'
 unset BOT_SETUP_TOKEN BOT_SETUP_ADMIN bot_token
 cat > /etc/systemd/system/vpn-ui-telegram-bot.service <<UNIT
[Unit]
Description=VPN-UI Telegram bot
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
WorkingDirectory=$APP
ExecStart=$NODE $APP/server.mjs
Restart=on-failure
RestartSec=10
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=$DATA
[Install]
WantedBy=multi-user.target
UNIT
 systemctl daemon-reload
 systemctl enable --now vpn-ui-telegram-bot.service
 echo 'سرویس راه‌اندازی شد. اگر وب‌هوک این توکن قبلاً فعال است ابتدا گزینه ۷ را با آگاهی از قطع اتصال نسخه قبلی بزنید.'
 systemctl --no-pager --full status vpn-ui-telegram-bot.service || true
}
backup(){
 mkdir -p "$DATA/backups" && chmod 700 "$DATA/backups" || return 1
 [[ -f "$DATA/bot.sqlite" ]] || { echo 'دیتابیس پیدا نشد.'; return 1; }
 local out
 out=$(mktemp "$DATA/backups/bot-$(date +%Y%m%d-%H%M%S)-XXXXXX.sqlite") || return 1
 if ! python3 - "$DATA/bot.sqlite" "$out" <<'PY'
import sqlite3,sys
src=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True)
dst=sqlite3.connect(sys.argv[2]);src.backup(dst);dst.close();src.close()
PY
 then rm -f "$out"; return 1; fi
 chmod 600 "$out" || return 1
 echo "بکاپ: $out"
}
update_bot()(
 [[ -f "$CONF/config.json" && -f "$DATA/bot.sqlite" && -x "$NODE" && -d "$APP" ]] || { echo 'نصب کامل ربات پیدا نشد.'; return 1; }
 local tmp='' stage='' previous='' was_running=0 stop_requested=0 committed=0
 cleanup_update(){
   local result=$?
   trap - EXIT INT TERM HUP
   if (( stop_requested && !committed )); then
     if [[ -n "$previous" && -d "$previous" ]]; then
       if [[ -e "$APP" ]]; then rm -rf -- "$APP"; fi
       mv -- "$previous" "$APP" || echo 'بازگرداندن فایل‌های قبلی ناموفق بود؛ سرویس را بررسی کنید.' >&2
     fi
     systemctl start vpn-ui-telegram-bot.service || echo 'راه‌اندازی دوباره سرویس ناموفق بود؛ وضعیت آن را بررسی کنید.' >&2
   fi
   [[ -z "$tmp" || ! -d "$tmp" ]] || rm -rf -- "$tmp"
   [[ -z "$stage" || ! -d "$stage" ]] || rm -rf -- "$stage"
   return "$result"
 }
 trap cleanup_update EXIT
 trap 'exit 130' INT
 trap 'exit 143' TERM
 trap 'exit 129' HUP
 tmp=$(mktemp -d) || return 1
 stage=$(mktemp -d /opt/.vpn-ui-bot-stage.XXXXXX) || { rm -rf "$tmp"; return 1; }
 previous="${stage}.previous"
 echo 'دریافت و آماده‌سازی نسخه جدید...'
 if ! curl -fsSL --retry 2 https://codeload.github.com/admin6501/vpn-ui/tar.gz/refs/heads/main -o "$tmp/repo.tar.gz" ||
    ! tar -xzf "$tmp/repo.tar.gz" -C "$tmp" --wildcards 'vpn-ui-main/telegram-bot/*'; then
   echo 'دریافت نسخه جدید ناموفق بود؛ ربات تغییر نکرد.'; rm -rf "$tmp" "$stage"; return 1
 fi
 local source="$tmp/vpn-ui-main/telegram-bot"
 # The reviewed schema change adds an empty user directory and removes seed plans.
 # Other schema changes still require an explicit migration.
 if ! cmp -s "$APP/schema.sql" "$source/schema.sql"; then
   local old_hash new_hash
   old_hash=$(sha256sum "$APP/schema.sql" | awk '{print $1}')
   new_hash=$(sha256sum "$source/schema.sql" | awk '{print $1}')
   if [[ ( "$old_hash" != 586bc5f4900800bf85250e193800c338a7f0f4e46228b956f4c60116b6f2c1ae && "$old_hash" != 6f8ec662b729ec101df2e2dd968515a07f96e6c67cd5874dd32399752f7cb956 && "$old_hash" != 0c48c78842130427ad196612565634c87cf74f6b22c5c31d5fa8331c1296ea05 ) || "$new_hash" != c3507019edd74397a9ccba37a9f835bf6c91c71b3ba27283954db9c93228ebda ]]; then
     echo 'ساختار دیتابیس نسخه جدید تغییر کرده است؛ آپدیت متوقف شد و به روش مهاجرت نیاز دارد.'; rm -rf "$tmp" "$stage"; return 1
   fi
 fi
 if ! cp "$source"/{worker.js,qr.js,panel-config.js,wallet.js,server.mjs,plan-migration.mjs,schema.sql,package.json,package-lock.json,manage-server.sh} "$stage/" ||
    ! (cd "$stage" && PATH="/opt/vpn-ui-bot-node/bin:$PATH" npm ci --omit=dev --no-audit --no-fund && "$NODE" --check server.mjs && "$NODE" --check worker.js && bash -n manage-server.sh); then
   echo 'آماده‌سازی ناموفق بود؛ ربات تغییر نکرد.'; rm -rf "$tmp" "$stage"; return 1
 fi
 chmod 700 "$stage/manage-server.sh"
 rm -rf "$tmp"
 if systemctl is-active --quiet vpn-ui-telegram-bot.service; then
   was_running=1
   stop_requested=1
   if ! systemctl stop vpn-ui-telegram-bot.service; then rm -rf "$stage"; echo 'توقف سرویس ناموفق بود.'; return 1; fi
 fi
 if ! backup; then
   (( was_running == 0 )) || systemctl start vpn-ui-telegram-bot.service || true
   rm -rf "$stage"; echo 'بکاپ ناموفق بود؛ آپدیت انجام نشد.'; return 1
 fi
 if ! mv "$APP" "$previous"; then
   (( was_running == 0 )) || systemctl start vpn-ui-telegram-bot.service || true
   rm -rf "$stage"; echo 'جابجایی نسخه قبلی ناموفق بود.'; return 1
 fi
 if ! mv "$stage" "$APP"; then
   mv "$previous" "$APP"
   (( was_running == 0 )) || systemctl start vpn-ui-telegram-bot.service || true
   echo 'نصب نسخه جدید ناموفق بود.'; return 1
 fi
 if (( was_running )); then
   if ! systemctl start vpn-ui-telegram-bot.service || { sleep 2; ! systemctl is-active --quiet vpn-ui-telegram-bot.service; }; then
     echo 'نسخه جدید بالا نیامد؛ نسخه قبلی برگردانده می‌شود.'
     systemctl stop vpn-ui-telegram-bot.service || true
     rm -rf "$APP"
     mv "$previous" "$APP"
     systemctl start vpn-ui-telegram-bot.service || echo 'راه‌اندازی نسخه قبلی ناموفق بود؛ لاگ را بررسی کنید.'
     return 1
   fi
 fi
 committed=1
 rm -rf "$previous"
 echo 'آپدیت انجام شد. تنظیمات و دیتابیس نگه داشته شدند.'
)
database_counts(){
 [[ -f "$DATA/bot.sqlite" ]] || { echo 'دیتابیس پیدا نشد.'; return 1; }
 python3 - "$DATA/bot.sqlite" <<'PY'
import sqlite3, sys
from pathlib import Path
uri = Path(sys.argv[1]).resolve().as_uri() + '?mode=ro'
with sqlite3.connect(uri, uri=True) as db:
    for label, table in [('بسته‌ها','plans'),('سرویس‌های ثبت‌شده در ربات','services'),('سفارش‌ها','orders'),('ادمین‌های اضافه','admins'),('سرویس‌های تست ثبت‌شده','test_claims')]:
        print(f'{label}: {db.execute("SELECT COUNT(*) FROM " + table).fetchone()[0]}')
    print(f'کاربران دارای گردش کیف پول: {db.execute("SELECT COUNT(DISTINCT user_id) FROM wallet_entries").fetchone()[0]}')
PY
}
remove_bot(){
 echo 'سرویس و فایل‌های برنامه حذف می‌شوند. تنظیمات و دیتابیس ابتدا حفظ می‌شوند.'
 local answer
 read -r -p 'برای حذف ربات REMOVE را وارد کنید: ' answer
 [[ "$answer" == REMOVE ]] || { echo 'حذف لغو شد.'; return 0; }
 if systemctl is-active --quiet vpn-ui-telegram-bot.service && ! systemctl stop vpn-ui-telegram-bot.service; then
   echo 'توقف سرویس ناموفق بود؛ حذف لغو شد.'; return 1
 fi
 systemctl disable vpn-ui-telegram-bot.service 2>/dev/null || true
 rm -f /etc/systemd/system/vpn-ui-telegram-bot.service
 systemctl daemon-reload
 if [[ "$(readlink /usr/local/bin/vpn-ui-bot 2>/dev/null || true)" == "$APP/manage-server.sh" ]]; then rm -f /usr/local/bin/vpn-ui-bot; fi
 rm -rf "$APP" /opt/vpn-ui-bot-node
 echo 'ربات حذف شد. دیتابیس، بکاپ‌ها و تنظیمات حفظ شدند.'
 echo 'حذف این اطلاعات تمام سرویس‌ها، کیف پول‌ها، بکاپ‌ها و توکن ذخیره‌شده ربات را پاک می‌کند.'
 read -r -p 'برای حذف دائمی همه این داده‌ها DELETE را وارد کنید؛ برای نگه‌داشتن Enter بزنید: ' answer
 if [[ "$answer" == DELETE ]]; then rm -rf "$DATA" "$CONF"; echo 'داده‌ها و تنظیمات حذف شدند.'; else echo 'داده‌ها در /var/lib/vpn-ui-bot و تنظیمات در /etc/vpn-ui-bot باقی ماندند.'; fi
}
remove_webhook(){
 [[ -f "$CONF/config.json" ]] || { echo 'ربات نصب نشده.'; return 1; }
 echo 'این کار اتصال نسخه دیگر ربات به همین توکن را قطع می‌کند. پیام‌های در انتظار حذف نمی‌شوند.'
 read -r -p 'برای تایید کلمه YES را وارد کنید: ' answer
 [[ "$answer" == YES ]] || return 0
 "$NODE" --input-type=module -e 'import {readFileSync} from "node:fs";const {BOT_TOKEN}=JSON.parse(readFileSync("/etc/vpn-ui-bot/config.json"));const r=await fetch(`https://api.telegram.org/bot${BOT_TOKEN}/deleteWebhook`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({drop_pending_updates:false})});const o=await r.json();if(!o.ok)throw Error(o.description);console.log("وب‌هوک حذف شد.")'
 systemctl restart vpn-ui-telegram-bot.service
}
while true; do
 printf '\nمدیریت ربات VPN-UI\n1) نصب تازه\n2) وضعیت\n3) شروع\n4) توقف\n5) راه‌اندازی مجدد\n6) نمایش لاگ\n7) انتقال از وب‌هوک به سرور\n8) بکاپ دیتابیس\n9) آپدیت ربات\n10) حذف ربات\n11) آمار دیتابیس ربات\n0) خروج\n'
 read -r -p 'انتخاب: ' choice || exit 0
 case "$choice" in
  1) install_bot || echo 'نصب کامل نشد.';;
  2) systemctl --no-pager --full status vpn-ui-telegram-bot.service || true;;
  3) systemctl start vpn-ui-telegram-bot.service;; 4) systemctl stop vpn-ui-telegram-bot.service;;
  5) systemctl restart vpn-ui-telegram-bot.service;; 6) journalctl -u vpn-ui-telegram-bot.service -n 100 --no-pager;;
  7) remove_webhook;; 8) backup;; 9) update_bot || echo 'آپدیت کامل نشد.';; 10) remove_bot || echo 'حذف کامل نشد.'; exit 0;; 11) database_counts || echo 'خواندن آمار ناموفق بود.';; 0) exit 0;; *) echo 'گزینه نامعتبر';;
 esac
 pause
done
