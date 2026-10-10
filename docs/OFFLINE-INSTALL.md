# نصب آفلاین / Offline installation

## فارسی

باینری `vpn-ui-amd64` و اسکریپت `install-offline.sh` را از ریلیز رسمی **admin6501/vpn-ui** روی یک
دستگاه دارای اینترنت دانلود کنید و کنار هم روی سرور کپی کنید. `SHA256SUMS` اختیاری است، ولی برای
کنترل صحت انتقال پیشنهاد می‌شود. باینری باید از منبع مطمئن باشد: اسکریپت آن را با دسترسی root اجرا
می‌کند.

```bash
sudo bash install-offline.sh ./vpn-ui-amd64
```

زبان انگلیسی:

```bash
sudo bash install-offline.sh --lang en ./vpn-ui-amd64
```

Linux amd64 یا arm64، Bash، systemd فعال و ابزارهای پایهٔ سیستم (از جمله coreutils و awk) لازم‌اند. باینری
شامل Xray، هسته‌های قابل‌حمل و داده‌های عمومی، ایران و روسیه است. برای برخی پروتکل‌ها، nftables،
iproute2، هدرهای کرنل/DKMS یا ماژول‌های WireGuard/AmneziaWG/PPP باید از قبل فراهم شوند. نصب پنل با
دو فایل انجام می‌شود؛ این دو فایل جایگزین بسته‌های سیستم‌عامل نیستند. آن بسته‌ها را قبل از محدود شدن
اینترنت نصب کنید یا بسته‌های مناسب همان توزیع و تمام وابستگی‌ها را جداگانه آفلاین منتقل کنید. برای
راه‌اندازی هسته‌ها صفحهٔ هسته‌ها را بررسی کنید؛ شکست مرحلهٔ پیش‌نیاز را نادیده نگیرید.

اسکریپت پیش از مرحلهٔ apt یک سؤال Yes/No می‌پرسد. با No (پیش‌فرض، از جمله ورودی بسته/EOF) هیچ
دانلودی انجام نمی‌شود. با Yes مخازن apt به‌روزرسانی و پیش‌نیازهای میزبان نصب می‌شوند؛ این مرحله
اینترنت لازم دارد. اگر مخزن یا نصب بسته شکست بخورد، خطا گزارش و نصب محلی پنل ادامه پیدا می‌کند. این
مرحله قبل از توقف سرویس پنل است؛ بسته‌های apt جزو بازگردانی فایل‌های پنل نیستند.

`VPNUI_OFFLINE=1` در یک drop-in سرویس ثبت می‌شود تا نصب خودکار بسته‌ها، کشف IP از سرویس‌های اینترنتی
و به‌روزرسانی خودکار geodata غیرفعال شوند. این گزینه VPN را قطع نمی‌کند و فایروال مسدودکنندهٔ تمام
اینترنت هم نیست. عملیات دستی مانند آپدیت از منو، WARP یا صدور گواهی همچنان اینترنت می‌خواهند؛ در
سرور محدود آن‌ها را اجرا نکنید.

نصب جدید: رمز تصادفی و پورت/مسیر چاپ می‌شود و نسخهٔ خصوصی اطلاعات ورود در پوشهٔ بکاپ ذخیره می‌شود.
IP چاپ‌شده در حالت آفلاین ممکن است `127.0.0.1` باشد؛ در مرورگر IP عمومی سرور یا دامنه را جایگزین
کنید. برای چاپ آدرس عمومی دلخواه:

```bash
sudo env VPNUI_PUBLIC_IP=YOUR_SERVER_IP bash install-offline.sh ./vpn-ui-amd64
```

نصب تازه HTTP است. اگر TLS موجود دارید، به‌روزرسانی آن را حفظ می‌کند؛ صدور/تمدید ACME به دسترسی
اینترنت نیاز دارد.

به‌روزرسانی: رمز، دیتابیس و تنظیمات قبلی حفظ می‌شود. قبل از تغییر، سرویس متوقف و باینری، SQLite و
WAL/SHM، منو، unit و drop-in در `/opt/vpn-ui/offline-backup.*` با دسترسی خصوصی ذخیره می‌شوند. در
خطای نصب فایل‌های قبلی بازگردانده می‌شوند. اسکریپت برای مسیر استاندارد `/opt/vpn-ui` است؛ سرویس با
مسیر باینری یا دیتابیس سفارشی را رد می‌کند. برای چنین نصب‌هایی همان unit و متغیر `VPNUI_DB_FOLDER`
را نگه دارید، سرویس را متوقف کنید، از پوشهٔ دیتابیس و باینری بکاپ بگیرید، باینری را دستی جایگزین و
سرویس را دوباره اجرا کنید.

پس از نصب، `systemctl status vpn-ui` و ورود با مرورگر را بررسی کنید. برای خطا
`journalctl -u vpn-ui -n 100 --no-pager` را بخوانید. فعال بودن سرویس به معنی تست اتصال VPN نیست.

برای خروج از حالت آفلاین در نصب استاندارد، بعد از بازگشت اینترنت:

```bash
sudo rm /etc/systemd/system/vpn-ui.service.d/90-offline.conf
sudo systemctl daemon-reload
sudo systemctl restart vpn-ui
```

اگر نام سرویس سفارشی است، `vpn-ui` را با نام آن جایگزین کنید. سپس پیش‌نیازهای هسته‌های ناقص را از
صفحهٔ هسته‌ها نصب کنید. با هر اجرای مجدد اسکریپت آفلاین، drop-in دوباره ساخته می‌شود.

## English

Transfer the release's `vpn-ui-amd64` and `install-offline.sh` into the same directory, optionally
with `SHA256SUMS`. Run:

```bash
sudo bash install-offline.sh --lang en ./vpn-ui-amd64
```

Requires Linux amd64 or arm64, Bash, an active systemd system and standard utilities (coreutils/awk). The
installer asks Yes/No before optional apt installation. No (the default, including EOF) keeps
installation local without downloads. Yes refreshes apt indexes and installs host prerequisites,
requiring connectivity. apt failures are reported and local installation continues. This happens
before stopping the panel; apt package changes are not part of panel-file rollback. Portable cores
and six base/IR/RU geodata files are embedded, but OS packages/kernel modules are not. Preinstall
the prerequisites for the protocols you use (nftables, iproute2, kernel headers/DKMS,
WireGuard/AmneziaWG/PPP as applicable). Missing packages are reported instead of attempting online
installation. Check the cores page after installing the panel.

A fresh database receives random credentials. Updates preserve credentials, TLS and database
settings. Private backups under `/opt/vpn-ui/offline-backup.*` include the stopped database and its
sidecars, binary, service configuration and menu; installation errors restore previous files. Custom
binary/database locations are rejected to avoid modifying the wrong database; back up and update
these manually, preserving their service environment.

The installer persists `VPNUI_OFFLINE=1` in the service drop-in `90-offline.conf`. This disables
package-install downloads, public-IP discovery and automatic geodata updates. It does not block VPN
networking or all manual internet operations. Manual updates, WARP and ACME issuance/renewal still
need connectivity. Printed localhost URLs must be changed to the actual server address; optionally
supply `VPNUI_PUBLIC_IP` for the installation report. New installs use HTTP.

To restore automatic online operations, remove
`/etc/systemd/system/vpn-ui.service.d/90-offline.conf`, run `systemctl daemon-reload` and restart
`vpn-ui` (substitute the actual unit name on custom units). Check service logs, browser access and
actual VPN traffic; a running service alone is not a connectivity test.

## بسته‌های apt / apt package list

| Purpose / کاربرد                         | Packages                                                       |
| ---------------------------------------- | -------------------------------------------------------------- |
| Certificates, download and archive tools | `ca-certificates`, `curl`, `unzip`, `openssl`                  |
| Conversion and database tools            | `python3`, `sqlite3`                                           |
| Host networking and modules              | `iproute2`, `nftables`, `kmod`                                 |
| Base utilities and scheduled tasks       | `coreutils`, `gawk`, `cron`                                    |
| Local kernel-module compilation          | `build-essential`, `dkms`, `pkg-config`                        |
| Running-kernel packages, when available  | `linux-headers-$(uname -r)`, `linux-modules-extra-$(uname -r)` |

باینری کامل، هسته‌های قابل‌حمل را فراهم می‌کند؛ apt ابزارهای میزبان را نصب می‌کند. اسکریپت کرنل
عمومی جدید را برای جایگزینی کرنل فعلی انتخاب نمی‌کند و خودش reboot نمی‌کند. اگر بستهٔ همان کرنل
موجود نباشد، آن را گزارش می‌کند؛ بررسی ماژول‌ها و راه‌اندازی پروتکل از صفحهٔ هسته‌ها همچنان لازم
است. در توزیع بدون apt این مرحله رد می‌شود و مدیر بستهٔ همان توزیع باید پیش‌نیازها را فراهم کند.

The full binary provides portable cores; apt supplies host utilities. Only matching running-kernel
package names are considered, when present in the refreshed repository metadata. The script does not
choose a replacement generic kernel or reboot the server. Check module availability and core setup
after installing. Non-apt distributions skip this optional step.

[فهرست مستندات / Documentation index](INDEX.md)

## ARM64 / aarch64

برای سرور ARM64 فایل `vpn-ui-arm64` را دانلود کنید و اجرا کنید:

```bash
sudo bash install-offline.sh ./vpn-ui-arm64
```

On ARM64, download `vpn-ui-arm64` instead of `vpn-ui-amd64`. The installed path remains
`/opt/vpn-ui/vpn-ui-amd64` for menu and migration compatibility; the executable and bundled cores
are native ARM64.
