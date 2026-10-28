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
 cp "$HERE"/{worker.js,qr.js,panel-config.js,wallet.js,server.mjs,schema.sql,package.json,package-lock.json,manage-server.sh} "$APP/"
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
 mkdir -p "$DATA/backups"; chmod 700 "$DATA/backups"
 [[ -f "$DATA/bot.sqlite" ]] || { echo 'دیتابیس پیدا نشد.'; return 1; }
 local out="$DATA/backups/bot-$(date +%Y%m%d-%H%M%S).sqlite"
 python3 - "$DATA/bot.sqlite" "$out" <<'PY'
import sqlite3,sys
src=sqlite3.connect('file:'+sys.argv[1]+'?mode=ro',uri=True)
dst=sqlite3.connect(sys.argv[2]);src.backup(dst);dst.close();src.close()
PY
 chmod 600 "$out"; echo "بکاپ: $out"
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
 printf '\nمدیریت ربات VPN-UI\n1) نصب تازه\n2) وضعیت\n3) شروع\n4) توقف\n5) راه‌اندازی مجدد\n6) نمایش لاگ\n7) انتقال از وب‌هوک به سرور\n8) بکاپ دیتابیس\n0) خروج\n'
 read -r -p 'انتخاب: ' choice || exit 0
 case "$choice" in
  1) install_bot || echo 'نصب کامل نشد.';;
  2) systemctl --no-pager --full status vpn-ui-telegram-bot.service || true;;
  3) systemctl start vpn-ui-telegram-bot.service;; 4) systemctl stop vpn-ui-telegram-bot.service;;
  5) systemctl restart vpn-ui-telegram-bot.service;; 6) journalctl -u vpn-ui-telegram-bot.service -n 100 --no-pager;;
  7) remove_webhook;; 8) backup;; 0) exit 0;; *) echo 'گزینه نامعتبر';;
 esac
 pause
done
