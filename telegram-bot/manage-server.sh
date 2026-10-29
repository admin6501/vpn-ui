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
 cp "$HERE"/{worker.js,qr.js,panel-config.js,wallet.js,coupons.js,messages.js,migration.js,migration-journal.js,migration-commit.js,bulk-migration.js,server.mjs,plan-migration.mjs,schema.sql,package.json,package-lock.json,manage-server.sh} "$APP/"
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
 BACKUP_PATH="$out"
 echo "بکاپ: $out"
}
restore_backup()(
 [[ -f "$DATA/bot.sqlite" && -f "$CONF/config.json" ]] || { echo 'نصب ربات پیدا نشد.'; return 1; }
 local source stage='' answer was_running=0 replaced=0 committed=0 previous=''
 read -r -p 'مسیر کامل فایل بکاپ دیتابیس ربات: ' source
 [[ -f "$source" && -r "$source" ]] || { echo 'فایل بکاپ قابل خواندن نیست.'; return 1; }
 stage=$(mktemp "$DATA/.restore-XXXXXX.sqlite") || return 1
 cleanup_restore(){
   local code=$?; trap - EXIT INT TERM HUP
   if (( replaced && !committed )); then
     if ! systemctl stop vpn-ui-telegram-bot.service; then
       echo 'توقف سرویس برای بازگردانی ناموفق بود؛ دیتابیس را دستی بررسی کنید.' >&2
       [[ -z "$stage" || ! -f "$stage" ]] || rm -f -- "$stage"
       return 1
     fi
     rm -f "$DATA/bot.sqlite-wal" "$DATA/bot.sqlite-shm"
     if [[ -n "$previous" && -f "$previous" ]]; then
       cp -- "$previous" "$DATA/bot.sqlite" && chmod 600 "$DATA/bot.sqlite" || echo 'بازگرداندن دیتابیس قبلی ناموفق بود؛ بکاپ قبلی را بررسی کنید.' >&2
     fi
   fi
   if (( was_running && !committed )); then systemctl start vpn-ui-telegram-bot.service || echo 'راه‌اندازی دوباره ربات ناموفق بود.' >&2; fi
   [[ -z "$stage" || ! -f "$stage" ]] || rm -f -- "$stage"
   return "$code"
 }
 trap cleanup_restore EXIT
 trap 'exit 130' INT; trap 'exit 143' TERM; trap 'exit 129' HUP
 if ! python3 - "$source" "$stage" <<'PY'
import sqlite3,sys
from pathlib import Path
try:
    src=sqlite3.connect(Path(sys.argv[1]).resolve().as_uri()+'?mode=ro',uri=True,timeout=10)
    src.execute('PRAGMA trusted_schema=OFF')
    dst=sqlite3.connect(sys.argv[2]);src.backup(dst);src.close()
    dst.execute('PRAGMA trusted_schema=OFF')
    if dst.execute('PRAGMA integrity_check').fetchall()!=[('ok',)]: raise ValueError('integrity')
    required={'settings':{'key','value'},'services':{'id','user_id','name','panel_email','sub_id'},'orders':{'id','user_id','status','service_name'},'plans':{'key','gb','price'},'test_claims':{'user_id','created_at'}}
    for name,columns in required.items():
        actual={r[1] for r in dst.execute('PRAGMA table_info("'+name+'")')}
        if not columns.issubset(actual): raise ValueError('schema')
    dst.close()
except Exception:
    print('فایل سالم یا بکاپ سازگار دیتابیس ربات نیست.',file=sys.stderr);sys.exit(1)
PY
 then return 1; fi
 echo 'ریستور دیتابیس فعلی ربات را جایگزین می‌کند؛ سرویس‌های پنل یا پرداخت‌های بیرونی به عقب برنمی‌گردند.'
 echo 'تنظیمات و توکن فعلی حفظ می‌شود؛ اتصال رمزگذاری‌شده پنل ممکن است با توکن ربات دیگری باز نشود.'
 read -r -p 'برای تأیید RESTORE را وارد کنید: ' answer
 [[ "$answer" == RESTORE ]] || { echo 'ریستور لغو شد.'; return 0; }
 if systemctl is-active --quiet vpn-ui-telegram-bot.service; then
   was_running=1
   if ! systemctl stop vpn-ui-telegram-bot.service; then echo 'توقف ربات ناموفق بود.'; return 1; fi
 fi
 if ! backup; then echo 'بکاپ قبل از ریستور ناموفق بود.'; return 1; fi
 previous="$BACKUP_PATH"
 replaced=1
 rm -f "$DATA/bot.sqlite-wal" "$DATA/bot.sqlite-shm"
 chmod 600 "$stage" && mv -f -- "$stage" "$DATA/bot.sqlite" || return 1
 if (( was_running )); then
   if ! systemctl start vpn-ui-telegram-bot.service || { sleep 2; ! systemctl is-active --quiet vpn-ui-telegram-bot.service; }; then
     echo 'ربات پس از ریستور بالا نیامد؛ دیتابیس قبلی برگردانده می‌شود.'; return 1
   fi
 fi
 committed=1
 echo "ریستور انجام شد. بکاپ دیتابیس قبل از ریستور: $previous"
)
automatic_backup()(
 [[ -f "$DATA/bot.sqlite" && -f "$CONF/config.json" ]] || { echo 'دیتابیس یا تنظیمات ربات پیدا نشد.'; return 1; }
 local snapshot; snapshot=$(mktemp "$DATA/.telegram-backup-XXXXXX.sqlite") || return 1
 trap 'rm -f -- "$snapshot"' EXIT
 chmod 600 "$snapshot"
 python3 - "$DATA/bot.sqlite" "$snapshot" "$CONF/config.json" <<'PY'
import json,os,re,sqlite3,sys,urllib.request,uuid
from pathlib import Path
try:
    src=sqlite3.connect(Path(sys.argv[1]).resolve().as_uri()+'?mode=ro',uri=True,timeout=10)
    dst=sqlite3.connect(sys.argv[2]);src.backup(dst);dst.close();src.close()
    cfg=json.loads(Path(sys.argv[3]).read_text());token=cfg['BOT_TOKEN'];admins=str(cfg['ADMIN_ID'])
    if not re.fullmatch(r'\d+:[A-Za-z0-9_-]+',token) or not re.fullmatch(r'\d+(,\d+)*',admins): raise ValueError('config')
    data=Path(sys.argv[2]).read_bytes()
    if len(data)>50_000_000:
        print('دیتابیس بیش از سقف ارسال فایل تلگرام است.',file=sys.stderr);sys.exit(1)
    failed=[]
    for admin in dict.fromkeys(admins.split(',')):
        boundary='----vpnui'+uuid.uuid4().hex
        body=(f'--{boundary}\r\nContent-Disposition: form-data; name="chat_id"\r\n\r\n{admin}\r\n--{boundary}\r\nContent-Disposition: form-data; name="document"; filename="vpn-ui-bot-backup.sqlite"\r\nContent-Type: application/octet-stream\r\n\r\n'.encode()+data+f'\r\n--{boundary}--\r\n'.encode())
        try:
            request=urllib.request.Request('https://api.telegram.org/bot'+token+'/sendDocument',data=body,headers={'Content-Type':'multipart/form-data; boundary='+boundary})
            with urllib.request.urlopen(request,timeout=45) as response: result=json.load(response)
            if not result.get('ok'): failed.append(admin)
        except Exception: failed.append(admin)
    if failed: print('ارسال بکاپ به بعضی ادمین‌ها ناموفق بود؛ مطمئن شوید ربات را استارت کرده‌اند.',file=sys.stderr);sys.exit(1)
    print('بکاپ برای ادمین‌های اصلی ارسال شد.')
except Exception:
    print('ساخت یا ارسال بکاپ خودکار ناموفق بود؛ دیتابیس، تنظیمات و اتصال تلگرام را بررسی کنید.',file=sys.stderr);sys.exit(1)
PY
)
configure_auto_backup(){
 [[ -f "$CONF/config.json" && -f "$DATA/bot.sqlite" ]] || { echo 'ابتدا ربات را نصب کنید.'; return 1; }
 local choice interval unit seconds
 echo '1) فعال‌سازی یا تغییر فاصله  2) غیرفعال‌سازی  3) وضعیت  4) ارسال آزمایشی همین حالا  0) بازگشت'
 read -r -p 'انتخاب: ' choice
 case "$choice" in
  1)
   read -r -p 'عدد فاصله بکاپ: ' interval
   [[ "$interval" =~ ^[0-9]{1,6}$ ]] && (( 10#$interval > 0 )) || { echo 'فاصله باید عدد مثبت باشد.'; return 1; }
   read -r -p 'واحد: 1 دقیقه، 2 ساعت: ' unit
   case "$unit" in 1) seconds=$((10#$interval*60));; 2) seconds=$((10#$interval*3600));; *) echo 'واحد نامعتبر.'; return 1;; esac
   (( seconds <= 2592000 )) || { echo 'حداکثر فاصله ۳۰ روز است.'; return 1; }
   [[ -x "$APP/manage-server.sh" ]] || { echo 'اسکریپت مدیریت نصب نشده است.'; return 1; }
   cat > /etc/systemd/system/vpn-ui-bot-backup.service <<UNIT
[Unit]
Description=VPN-UI bot database backup to Telegram
Wants=network-online.target
After=network-online.target
[Service]
Type=oneshot
ExecStart=$APP/manage-server.sh --auto-backup
UNIT
   cat > /etc/systemd/system/vpn-ui-bot-backup.timer <<UNIT
[Unit]
Description=Schedule VPN-UI bot backups
[Timer]
OnActiveSec=${seconds}s
OnUnitActiveSec=${seconds}s
Unit=vpn-ui-bot-backup.service
[Install]
WantedBy=timers.target
UNIT
   systemctl daemon-reload && systemctl enable vpn-ui-bot-backup.timer && systemctl restart vpn-ui-bot-backup.timer || { echo 'فعال‌سازی زمان‌بندی ناموفق بود.'; return 1; }
   echo 'بکاپ خودکار فعال شد. ادمین‌های اصلی باید قبلاً ربات را /start کرده باشند.';;
  2) systemctl disable --now vpn-ui-bot-backup.timer || return 1; echo 'بکاپ خودکار غیرفعال شد.';;
  3) systemctl --no-pager --full status vpn-ui-bot-backup.timer || true;;
  4) automatic_backup;;
  0) return 0;; *) echo 'گزینه نامعتبر'; return 1;;
 esac
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
 # The reviewed schema change adds the reviewed bot user, plan and discount-code tables.
 # Other schema changes still require an explicit migration.
 if ! cmp -s "$APP/schema.sql" "$source/schema.sql"; then
   local old_hash new_hash
   old_hash=$(sha256sum "$APP/schema.sql" | awk '{print $1}')
   new_hash=$(sha256sum "$source/schema.sql" | awk '{print $1}')
   if [[ ( "$old_hash" != 586bc5f4900800bf85250e193800c338a7f0f4e46228b956f4c60116b6f2c1ae && "$old_hash" != 6f8ec662b729ec101df2e2dd968515a07f96e6c67cd5874dd32399752f7cb956 && "$old_hash" != 0c48c78842130427ad196612565634c87cf74f6b22c5c31d5fa8331c1296ea05 && "$old_hash" != c3507019edd74397a9ccba37a9f835bf6c91c71b3ba27283954db9c93228ebda ) || "$new_hash" != fcf4503c510e1ff9897b7002b1f75396b03fa389e5f7a43db04675c346c95b04 ]]; then
     echo 'ساختار دیتابیس نسخه جدید تغییر کرده است؛ آپدیت متوقف شد و به روش مهاجرت نیاز دارد.'; rm -rf "$tmp" "$stage"; return 1
   fi
 fi
 if ! cp "$source"/{worker.js,qr.js,panel-config.js,wallet.js,coupons.js,messages.js,migration.js,migration-journal.js,migration-commit.js,bulk-migration.js,server.mjs,plan-migration.mjs,schema.sql,package.json,package-lock.json,manage-server.sh} "$stage/" ||
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
 systemctl disable --now vpn-ui-bot-backup.timer 2>/dev/null || true
 rm -f /etc/systemd/system/vpn-ui-bot-backup.timer /etc/systemd/system/vpn-ui-bot-backup.service
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
if [[ "${1:-}" == --auto-backup ]]; then automatic_backup; exit $?; fi
if [[ "${1:-}" == --update ]]; then update_bot; exit $?; fi
while true; do
 printf '\nمدیریت ربات VPN-UI\n1) نصب تازه\n2) وضعیت\n3) شروع\n4) توقف\n5) راه‌اندازی مجدد\n6) نمایش لاگ\n7) انتقال از وب‌هوک به سرور\n8) بکاپ دیتابیس\n9) آپدیت ربات\n10) حذف ربات\n11) آمار دیتابیس ربات\n12) ریستور بکاپ\n13) بکاپ خودکار تلگرام\n0) خروج\n'
 read -r -p 'انتخاب: ' choice || exit 0
 case "$choice" in
  1) install_bot || echo 'نصب کامل نشد.';;
  2) systemctl --no-pager --full status vpn-ui-telegram-bot.service || true;;
  3) systemctl start vpn-ui-telegram-bot.service;; 4) systemctl stop vpn-ui-telegram-bot.service;;
  5) systemctl restart vpn-ui-telegram-bot.service;; 6) journalctl -u vpn-ui-telegram-bot.service -n 100 --no-pager;;
  7) remove_webhook;; 8) backup;; 9) update_bot || echo 'آپدیت کامل نشد.';; 10) remove_bot || echo 'حذف کامل نشد.'; exit 0;; 11) database_counts || echo 'خواندن آمار ناموفق بود.';;
  12) restore_backup || echo 'ریستور کامل نشد.';; 13) configure_auto_backup || echo 'تنظیم بکاپ خودکار کامل نشد.';; 0) exit 0;; *) echo 'گزینه نامعتبر';;
 esac
 pause
done
