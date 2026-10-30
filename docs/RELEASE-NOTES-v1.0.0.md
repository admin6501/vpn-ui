# VPN-UI v1.0.0 — First stable release / اولین نسخهٔ پایدار

**v1.0.0 is the first stable release of the admin6501/vpn-ui fork**, based on Sir-MmD/vpn-ui
and 3X-UI. This release brings the central panel, dedicated location agent, installers and
migration tools together with a complete, current documentation set. This page describes the
stable release as a whole; older version numbers and dated live-test reports in retained design
records describe upstream development, not previous stable releases of this fork.


## Sales bot updates / به‌روزرسانی ربات فروش

The optional server bot supports multiple panels and category-bound plans, customer-selected names, wallet and card checkout, coupon limits, customer/admin messaging, subscription alerts, optional expired-service deletion, and backup/restore with scheduled Telegram backups. Transfer of bot-managed services requires a super-admin destination for importing recorded usage; unrecorded source usage can be missed. Additive renewal edits the existing client and preserves consumption, unlike the panel reset-style renewal. The bot UI is Persian; the panel's built-in Telegram management bot has separate language settings. [Bot guide](../telegram-bot/README-server.txt), [migration](BOT-LOCATION-MIGRATION.fa.md), [API for panel owners and representatives](REPRESENTATIVE-BOT-API.fa.md).

## تغییر جدید: حذف کاربر چندورودی

با حذف کاربر از یکی از چند ورودی، مصرف و IPهای مشترک تا حذف آخرین ورودی حفظ می‌شوند و بازپرداخت اعتبار نماینده بر اساس مصرف ثبت‌شده محاسبه می‌شود. تنظیمات سرویس پروتکل مربوط هم بازسازی می‌شود. ربات می‌تواند قابلیت پنل را تشخیص دهد، اما باید هر ورودی را جداگانه حذف و نتیجه را بررسی کند؛ در صورت قطع یا خطا، حذف ممکن است ناقص بماند و باید بررسی شود.

## Panel, protocols and presentation

- Xray outbound traffic stays within its column: upload/download use separate lines, column
  widths are balanced, and narrow tables scroll without widening the page. Successful tests
  remain successful when optional exit-IP metadata is unavailable.

- Manage Xray-native VLESS, VMess, Trojan, Shadowsocks, Hysteria2, AnyTLS, TUIC, NaiveProxy and
  native WireGuard, plus OpenVPN, OpenConnect, L2TP, PPTP, SSTP, IKEv2, kernel WireGuard,
  AmneziaWG, GRE, MTProto and the embedded SSH proxy.
- Full binaries carry the patched Xray core, portable backends, ACME client and all six base,
  Iran and Russia geoip/geosite files. OS packages and kernel/DKMS prerequisites remain host requirements.
- IPsec is selectable in central and node Cores dialogs and is a shared L2TP/IKEv2 dependency.
  Shared dependencies stay installed while another selected core needs them. GRE/IPsec changes
  preserve shared L2TP authentication/configuration; GRE peer public IPs can be edited and exported.
- Panel branding updates brand-aware login, Overview and subscription presentation. Modern and
  legacy inbound forms, responsive layouts, scoped menus and graphical Xray configuration remain available.
- External Proxy endpoints/configuration names are consistent between Info, copied links and
  subscription exports. Protocol profiles and device-specific configurations can be downloaded.
  Source-IP, session/device and download/upload speed controls are shown for supported protocols.

- WARP rejects failed or incomplete provider responses without replacing stored credentials or
  recording a failed license change. Requests have a timeout and bounded response size.

- Before a group client is created or a representative is charged, validate required connection fields for every selected managed inbound, including missing passwords, IDs, protocol-specific key formats and manually supplied device keys. Valid per-membership generated keys remain supported.

## Accounts, traffic, renewal and representatives

- Deleting an account by email from one of several inbounds now retains its shared
  traffic and saved IPs until the final membership is removed. The traffic row moves
  to a remaining inbound, so the final reseller refund uses the recorded consumption.
  Email-based deletion also reconciles the affected protocol daemon or WireGuard peers.
  `GET /panel/api/inbounds/multiDeleteCapability` lets bot clients detect this behavior;
  callers must still delete and verify each membership and handle partial failures.


- Users manages one account across several inbounds/locations with a central quota, expiry,
  credentials and subscription. Groups provide reusable assignments while preserving ownership
  and each membership's access switch. Shared Sub IDs may publish distinct accounts together;
  they do not merge those accounts or their separate quotas.
- Creating a listener does not require an existing group and does not create an account. Cipher
  changes, cloning and reused Add forms keep empty account rosters. Deleting and recreating the
  same listener name/port works; explicitly reattach the replacement to its group to restore users.
  Existing account credentials, subscription identity and current/lifetime usage are retained.
- Native restore and upgrade migrations retain shared account traffic even without a legacy
  home inbound, including Shadowsocks and parked accounts. Orphan cleanup preserves rows owned
  by an account. Removing an Xray outbound and rebuilding its configuration retain client usage.
- Overview counts unique accounts. Quota exhaustion and finite time expiry both affect depleted
  status. Subscription pages show effective state, actual quota, upload/download and available
  configurations, including representative-blocked accounts without mutating their records.
- New traffic is attributed to its inbound. Info displays measured inbound shares and any
  historical unattributed remainder separately. Old account-only records cannot reconstruct an
  exact historical split; updating retains those totals rather than inventing source attribution.
- **Renew**, in the client three-dot menu, clears current account/membership upload and download,
  starts the complete recorded package duration from now and enables the account. Traffic-only,
  time-only, combined and unlimited packages are supported. Legacy unknown durations ask for days
  and remember the answer. Quota, access, password, UUID, Sub ID, device keys and tunnel addresses remain.
- **Reset traffic**, also beside quota in the client editor, clears current usage without restarting
  expiry or enabling deliberately disabled access. Freeze/unfreeze preserves remaining time.
  Manual renewal is separate from automatic renewal. Lifetime representative accounting and
  cumulative node/core cursors remain, preventing refunds or double-billing from counter resets.
- Representative roles, own/all scopes, separate update/reset permissions and consumption-based
  limits apply throughout. Renewal requires both update and reset rights in the narrower scope.
  Existing prepaid accounts retain their billing rules. Exhausted representative policy still
  applies when a customer enabled switch is checked. Protected-owner operations remain separate.

## Dedicated location nodes

- A dedicated agent pairs to one central panel using outbound verified HTTPS. Initial installation
  activates no VPN core; install the selected cores and dependencies through the central node card.
  Account credentials, subscriptions and quota remain central. Multi-panel pairing is not included.
- Agent polling and the visible Nodes page refresh every second; host diagnostics normally refresh
  every five seconds separately. Unchanged snapshots avoid rewriting client tables, and changes
  reconcile affected cores. Commands show durable progress and remain pending until acknowledged.
- Central controls include start/stop/restart, core install/remove, runtime state and shared
  graphical/file configuration editing. Scoped node grants do not grant central owner operations.
- Cumulative sequenced reports survive updates and are accepted transactionally without billing
  retries twice. Online reports become stale after 35 seconds. A lost 90-second management lease
  stops the data plane while retaining counters; a fresh valid snapshot restores it.
- `install-node.sh` offers install/update, status, start, stop, restart, logs and confirmed removal.
  Existing pairing/counters survive updates; `--install` retains the direct scripted path.

## Thirteen languages and synchronized guides

English, Persian, Arabic, Spanish, Indonesian, Japanese, Brazilian Portuguese, Russian, Turkish,
Ukrainian, Vietnamese, Simplified Chinese and Traditional Chinese are included in the panel,
subscription browser page and built-in Telegram management bot. The built-in bot uses its configured language. The separate sales bot currently uses Persian. Terminal
installers remain English; the migration wizard supports Persian and English.

This stable update fixes concurrent HTML language mixing, regional-language/cookie fallback and
unnecessary first-page language reloads. Shared dialogs, pagination and calendars follow the web
language. Previously fixed English form explanations, tooltips, GRE instructions and validation
messages now use the thirteen catalogs. Technical enum values, units and configuration syntax remain intact.

There are now **13 complete root READMEs** with the same installation commands and release asset
names. The documentation index, new English/Persian operating guide, API/route reference, node,
offline, migration, renewal, security, architecture and feature guides match the stable source.
Historical proposals and dated test reports remain clearly identified and expandable.

## Installation and update

For a new central panel or an existing standard installation:

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

Back up first. Updates retain the database, credentials and settings. Use the exact scheme,
port and web path printed by installation; fresh installs use HTTP until TLS is configured.
Linux amd64/arm64, root, Bash and systemd are required. Ubuntu 24.04 and Debian 12/13 are
recommended targets; check the modules and packages needed by your deployed protocols.

For an existing paired node:

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/install-node.sh -o install-node.sh
sudo bash install-node.sh
```

Choose install/update. A new node must instead use its pairing command from the central panel.
Update the central panel and each existing agent to use all current node controls.

For offline panel installation/update, transfer the matching binary, `install-offline.sh` and
`SHA256SUMS` together, then run:

```bash
sha256sum --ignore-missing -c SHA256SUMS
sudo bash install-offline.sh ./vpn-ui-amd64
# ARM64: sudo bash install-offline.sh ./vpn-ui-arm64
```

The optional apt question defaults to No, including EOF. Yes needs reachable repositories;
package failures are reported and local installation continues. `VPNUI_OFFLINE=1` disables
background downloads, public-IP lookup and geodata updates. OS modules/packages remain separate;
ACME issuance/renewal, WARP and intentional online updates still require internet.

## Downloads

| Asset | Purpose |
| --- | --- |
| `vpn-ui-amd64` | Full Linux x86-64 central panel |
| `vpn-ui-arm64` | Full Linux aarch64 central panel |
| `vpn-ui-node-amd64` | Linux x86-64 location agent |
| `vpn-ui-node-arm64` | Linux aarch64 location agent |
| `install-offline.sh` | Local central installation/update and rollback support |
| `install-node.sh` | Agent installation/update and management menu |
| `vpn-ui-auto-migration.zip` | PasarGuard conversion scripts/configuration |
| `SHA256SUMS` | Checksums for the seven downloadable artifacts above |

The installed central executable retains `/opt/vpn-ui/vpn-ui-amd64` on both architectures for
compatibility. Core filenames follow their real architecture. PasarGuard migration requires a
compatible installed destination with empty VPN accounts/inbounds; it preserves web/TLS settings
and supported identities, ownership, groups and subscription metadata. Source nodes are not imported.

## Verification and practical limits

The release workflow builds both architectures natively and checks accounting, renewal/access
preservation, representative billing/permissions, authentication/sessions, TLS/CA pinning,
node data paths, shared dependencies, offline install/update and rollback. It renders all catalog
messages and configuration literals, tests concurrent HTML locale isolation and mocked Telegram
commands, and exercises real browser UI in all thirteen languages. Go/dependency versions and
vulnerability scans are pinned in the build workflow; published binaries and checksums must
match the released source revision.

This is not certification of every protocol on every VPS/kernel/distribution. The separate
privileged Incus live-protocol matrix needs a prepared host. Node changes propagate asynchronously;
in-flight/concurrent traffic can briefly exceed a quota, and node session/IP limits are local.
A prolonged central outage interrupts node service after the lease expires. Plain GRE is
unencrypted without IPsec; PPTP is legacy; the embedded SSH proxy does not expose a root shell.
Keep backups, subscription tokens, credentials, TLS keys and node databases private.

Documentation: [all guides](https://github.com/admin6501/vpn-ui/blob/main/docs/INDEX.md),
[operating guide](https://github.com/admin6501/vpn-ui/blob/main/docs/PANEL-GUIDE.md),
[renewal](https://github.com/admin6501/vpn-ui/blob/main/docs/CLIENT-RENEWAL.md),
[nodes](https://github.com/admin6501/vpn-ui/blob/main/docs/NODES.md),
[offline setup](https://github.com/admin6501/vpn-ui/blob/main/docs/OFFLINE-INSTALL.md),
[API](https://github.com/admin6501/vpn-ui/blob/main/api-reference.md),
[translations](https://github.com/admin6501/vpn-ui/blob/main/docs/TRANSLATIONS.md).

## توضیح نسخه به فارسی

در جدول خروجی‌های Xray، آپلود و دانلود در دو خط و داخل ستون ترافیک نمایش داده می‌شوند؛
عرض ستون‌ها متعادل شده و جدول در صفحهٔ باریک اسکرول می‌شود. تست موفق بدون اطلاعات اختیاری
IP خروجی، دیگر هم‌زمان برچسب خطا نشان نمی‌دهد.

**این اولین نسخهٔ پایدار این فورک با شمارهٔ v1.0.0 است.** پنل مرکزی، عامل نود، نصب آنلاین/آفلاین
و ابزار مهاجرت همراه راهنماهای هماهنگ منتشر می‌شوند. پنل، ساب و ربات مدیریتی داخلی ۱۳ زبان دارند؛ رابط ربات فروش مستقل فارسی است؛ شش README
جاافتاده اضافه و هر ۱۳ راهنما هم‌ساختار شده‌اند. تداخل زبان درخواست‌های هم‌زمان، تشخیص زبان منطقه‌ای
و متن‌های ثابت فرم‌ها، هشدارها و راهنمای GRE اصلاح شده‌اند. اسکریپت ترمینال انگلیسی و مهاجرت فارسی/انگلیسی است.

ساخت اینباند به وجود گروه نیاز ندارد. تغییر رمزنگاری Shadowsocks، کلون یا فرمِ دوباره‌استفاده‌شده
کاربر تصادفی اضافه نمی‌کند. پس از حذف، ساخت دوباره با همان نام و پورت ممکن است؛ اینباند جدید را
دوباره به گروه قبلی اضافه کنید. حساب، ساب، رمز و مصرف کاربر حفظ می‌شوند.

پاک‌سازی قدیمی هنگام ریستور یا مهاجرت، مصرف حساب‌هایی را که اینباند اصلی نداشتند اشتباهی حذف
می‌کرد؛ این مسیر برای حساب‌های مرکزی، Shadowsocks و حساب‌های بدون عضویت اصلاح شده است.
حذف اوت‌باند و بازسازی تنظیمات Xray مصرف کاربران را حفظ می‌کند. مصرفی که قبلاً حذف شده، برای
بازیابی دقیق به بکاپ معتبر نیاز دارد؛ نصب نسخهٔ اصلاح‌شده به‌تنهایی اعداد گذشته را برنمی‌گرداند.

کاربران و گروه‌ها سهمیه و دسترسی مرکزی دارند. مصرف جدید بر اساس اینباند ثبت می‌شود و باقیماندهٔ
تاریخیِ بدون اطلاعات مبدأ جدا نشان داده می‌شود. تیک فعال به‌تنهایی محدودیت حجم، زمان، نماینده یا
عضویت را لغو نمی‌کند. دو حساب با Sub ID یکسان در ساب کنار هم‌اند ولی هویت و سهمیه‌شان یکی نمی‌شود.

دکمهٔ **تمدید** در منوی سه‌نقطه، مصرف جاری را صفر، مدت کامل بسته را از الان شروع و حساب را فعال
می‌کند؛ رمز، UUID، ساب، حجم بسته، کلید دستگاه، IP و دسترسی حفظ می‌شوند. مدت نامعلومِ قدیمی پرسیده
می‌شود. **ریست ترافیک** فقط مصرف را صفر می‌کند و زمان را از اول شروع نمی‌کند. فریز، روزهای باقی‌مانده
را نگه می‌دارد. شمارندهٔ مادام‌العمر نماینده و نود پاک نمی‌شود و قواعد نمایندگی برقرار می‌مانند.

نودها هر یک ثانیه همگام می‌شوند؛ اطلاعات تشخیصی معمولاً هر پنج ثانیه است. مراحل فرمان تا تأیید
عامل در انتظار می‌ماند. نصب اولیهٔ نود هیچ هسته‌ای را فعال نمی‌کند. IPsec انتخاب مستقل و وابستگی
مشترک L2TP/IKEv2 است. منوی اسکریپت نود نصب/آپدیت، وضعیت، کنترل سرویس، لاگ و حذف دارد.
هر نود به یک پنل متصل است؛ مجوز مدیریتیِ قطع‌شده پس از ۹۰ ثانیه سرویس VPN نود را متوقف می‌کند.

برای آپدیت پنل از فرمان `deploy.sh` بالا و برای نود موجود از `install-node.sh` استفاده کنید.
اول بکاپ بگیرید؛ اطلاعات موجود در آپدیت استاندارد حفظ می‌شوند. فایل مناسب معماری را دانلود و هش‌ها
را بررسی کنید. انتخاب No در سؤال apt نصب را محلی نگه می‌دارد؛ ماژول و بستهٔ سیستم‌عامل جدا لازم‌اند.
نصب موفق، تضمین اجرای همهٔ پروتکل‌ها روی هر میزبان نیست؛ بعد از آپدیت اتصال واقعی و ساب خودتان را تست کنید.

[راهنمای فارسی نصب](https://github.com/admin6501/vpn-ui/blob/main/README_FA.md) ·
[راهنمای فارسی کار با پنل](https://github.com/admin6501/vpn-ui/blob/main/docs/PANEL-GUIDE.fa.md) ·
[راهنمای فارسی نود](https://github.com/admin6501/vpn-ui/blob/main/docs/NODES.fa.md).

اصلاح مدیریت خطای WARP: پاسخ خطا یا ناقص سرویس باعث ذخیرهٔ اطلاعات نامعتبر یا ثبت موفقیت کاذب لایسنس نمی‌شود؛ زمان انتظار و اندازهٔ پاسخ محدود شده‌اند.

در ساخت کاربر گروهی، مشخصات اتصال همهٔ ورودی‌های انتخاب‌شده پیش از ساخت و کسر اعتبار بررسی می‌شوند و در صورت نقص یا مقدار نامعتبر، درخواست با نام ورودی و فیلد رد می‌شود.
