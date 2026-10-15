# تاریخچهٔ کامل VPN-UI

این گزارش تاریخچهٔ موجود در `admin6501/vpn-ui` را تا زمان دریافت مخزن در ۱۵ اکتبر ۲۰۲۶ ثبت می‌کند. مرجع بررسی: شاخهٔ `main`، کامیت `b376dcaf961071ee3d6be947919ebd9900b4151e`.

مخزن به‌صورت کامل دریافت شده و shallow نیست. تاریخچهٔ قابل‌دسترسی شامل **۵۵ کامیت** از ۱۰ تا ۱۴ اکتبر ۲۰۲۶، یک شاخهٔ راه‌دور (`main`) و برچسب `v1.0.0` است. تاریخچهٔ پروژه‌های بالادستی پیش از اولین کامیت این مخزن در آن ثبت نشده است. تاریخچهٔ داخلی زیرماژول‌ها جدا از این فهرست است.

## روند تغییرات

- **۱۰ اکتبر — ۱۱ کامیت:** انتشار اولیه با مهاجرت PasarGuard و نصب آفلاین؛ پشتیبانی AMD64 و ARM64؛ اصلاح نصب و بازیابی؛ حفظ حسابداری و اشتراک پس از حذف ورودی؛ افزودن نودهای چندمکانه و رفع بارگذاری صفحهٔ نود.
- **۱۱ اکتبر — ۱۸ کامیت:** مدیریت پایدار نودها و هسته‌ها؛ تکمیل ۱۳ زبان؛ هماهنگی مصرف و انقضای حساب‌ها؛ اصلاح هویت مشتری و اعمال سرعت؛ چرخهٔ عمر مستقل حساب؛ اصلاح نشت اطلاعات ورود، ابطال نشست و WebSocket؛ سازگاری TPROXY؛ اصلاح Hysteria2، HTTP3 و نمایش وضعیت اتصال.
- **۱۲ اکتبر — ۱۲ کامیت:** نمایش نشست WireGuard؛ نقش مدیر و مالک مستقل؛ اصلاح جزئیات اتصال و ثبت تأیید دومرحله‌ای؛ رفع خطای ۴۰۳ ناوبری؛ حذف و نصب مجدد نود و هسته با حفظ سرویس‌های همسایه؛ اصلاح انتخاب هسته و آزمون مرورگر.
- **۱۳ اکتبر — ۹ کامیت:** حفظ تاریخچهٔ شمارندهٔ نود؛ جداسازی محیط تست؛ اصلاح پذیرش reset و محدودیت سرعت؛ بازیابی شمارندهٔ WireGuard؛ دریافت فایل conf؛ اصلاح MPTCP/TPROXY؛ پروفایل‌های TCP/UDP در OpenVPN؛ انتخاب واحد سرعت ورودی.
- **۱۴ اکتبر — ۵ کامیت:** محدود کردن نماینده به عملیات مشتری و سهمیهٔ خودش؛ حفظ دسترسی مدیران واردشده؛ اصلاح شش ایراد امنیتی و آماده‌بودن listener؛ هماهنگی مجوز Overview نماینده؛ تکمیل مجوز تنظیمات مدیر تفویض‌شده.

## سابقهٔ مرتبط با این قابلیت

زیرساخت فعلی کد TOTP را روی رکورد هر حساب نگه می‌دارد و مسیر ورود همین مقدار را برای مدیر و نماینده بررسی می‌کند. کامیت `8963307` حفاظت از اطلاعات حساس و ابطال نشست را تقویت کرده؛ `abc2f37` نقش‌های مدیر و مالکیت مستقل را افزوده؛ `a0ae3fa` ثبت تأیید دومرحله‌ای را با تأیید سمت سرور اصلاح کرده است. در آخرین نسخه، مسیر تنظیمات 2FA پشت مجوز تنظیمات کل پنل قرار داشت و نماینده نمی‌توانست از رابط کاربری خودش آن را فعال کند. کامیت‌های اخیر دربارهٔ مرز دسترسی نماینده نیز در تغییر جدید حفظ شده‌اند.

## فهرست تمام کامیت‌ها، از ابتدا تا آخرین نسخه

| ردیف | تاریخ | کامیت | شرح ثبت‌شده |
|---:|---|---|---|
| 1 | 2026-10-10 | [`00f842e`](https://github.com/admin6501/vpn-ui/commit/00f842e19cc7fea7696ff370f11933d40b113ffc) | Initial release: VPN-UI with PasarGuard migration and offline installation |
| 2 | 2026-10-10 | [`ddaba61`](https://github.com/admin6501/vpn-ui/commit/ddaba61007f53300e52e2dad4e6d48c04e0ddede) | Reject unsupported architectures before deployment |
| 3 | 2026-10-10 | [`99646f5`](https://github.com/admin6501/vpn-ui/commit/99646f503ab753984f87aa0d451297865de099bc) | Build and publish native ARM64 panel with bundled VPN cores |
| 4 | 2026-10-10 | [`0544294`](https://github.com/admin6501/vpn-ui/commit/0544294409ec4aa8476e50381f1edf14c45b9106) | Clarify ARM64 installation in localized documentation |
| 5 | 2026-10-10 | [`d2ae9d3`](https://github.com/admin6501/vpn-ui/commit/d2ae9d3141baae06801a6bf1d6ae0916e25c584d) | Recover incomplete installs with an incompatible old executable |
| 6 | 2026-10-10 | [`e793606`](https://github.com/admin6501/vpn-ui/commit/e79360674dd5bddb08c004938e8bb3ff63f15f84) | Allow replacing a non-runnable previous binary during deployment |
| 7 | 2026-10-10 | [`bb102ea`](https://github.com/admin6501/vpn-ui/commit/bb102ea919a5afccb96afbb69279b094ac1f303f) | Allow ARM64 in automatic migration bootstrap |
| 8 | 2026-10-10 | [`336a906`](https://github.com/admin6501/vpn-ui/commit/336a9066aa5a9f12d5c7fb33be83b0c0747561c5) | Preserve shared usage on inbound deletion and refresh overview and subscription links |
| 9 | 2026-10-10 | [`ad85dfd`](https://github.com/admin6501/vpn-ui/commit/ad85dfd1cc32193f37934b85e60b573bcc1fb3be) | Repair imported accounting and subscription display after inbound deletion |
| 10 | 2026-10-10 | [`165a94b`](https://github.com/admin6501/vpn-ui/commit/165a94ba06d8ba212d45c87ade355e4851f3af43) | Add dedicated multi-location agents with central core installation |
| 11 | 2026-10-10 | [`3ee7a5e`](https://github.com/admin6501/vpn-ui/commit/3ee7a5eb79f3402d4b15db87041bfcf8f33060a0) | Fix blank node page and browser form submissions |
| 12 | 2026-10-11 | [`dd772f0`](https://github.com/admin6501/vpn-ui/commit/dd772f017f9fbc249546654e97463afada36e48b) | Add durable node and core controls with shared configuration editors |
| 13 | 2026-10-11 | [`f13307c`](https://github.com/admin6501/vpn-ui/commit/f13307c4c407f9cb425672fb7b1dfe95b10ddb97) | Complete all thirteen locales and localize shared UI components |
| 14 | 2026-10-11 | [`4ec8a50`](https://github.com/admin6501/vpn-ui/commit/4ec8a5017aa5a302e87d8f2c7ebcfeed52943681) | Use English for every dedicated node installer prompt |
| 15 | 2026-10-11 | [`7346f5b`](https://github.com/admin6501/vpn-ui/commit/7346f5b6be301e668ecb81536ab4e645daa9b64e) | Keep node command-line root requirement in English |
| 16 | 2026-10-11 | [`ffbe566`](https://github.com/admin6501/vpn-ui/commit/ffbe5667a61cff6961ad5641c7d92c56590232ad) | Use English for agent enrollment output and runtime errors |
| 17 | 2026-10-11 | [`fa292c0`](https://github.com/admin6501/vpn-ui/commit/fa292c0318db628e6433b89b247dff40108dadd4) | Unify scoped account totals and time expiry across Overview and Inbounds |
| 18 | 2026-10-11 | [`a6de1cd`](https://github.com/admin6501/vpn-ui/commit/a6de1cdbbf07bcac351519397b03cb474225af35) | Refresh account expiration counters even without traffic updates |
| 19 | 2026-10-11 | [`004ba68`](https://github.com/admin6501/vpn-ui/commit/004ba68e15cfcf028a41448e189f1074a3a7b18e) | Show final node configuration and fix Overview health and installed TLS visibility |
| 20 | 2026-10-11 | [`28a3840`](https://github.com/admin6501/vpn-ui/commit/28a3840cecaec7ceeac62ddf534242bfeca0098e) | Fix client identity editing, name sorting, quota units and node rate delivery |
| 21 | 2026-10-11 | [`0d66611`](https://github.com/admin6501/vpn-ui/commit/0d6661162361cf8436ae44888d83a96dff40c0cf) | Compact client protocol details and comments; audit device and IP limit capabilities |
| 22 | 2026-10-11 | [`233a445`](https://github.com/admin6501/vpn-ui/commit/233a4457938fe401ccda5ce80e0b7f2d2f897b45) | Keep compact comment arrows reachable beside long client names |
| 23 | 2026-10-11 | [`e4cad77`](https://github.com/admin6501/vpn-ui/commit/e4cad77c2616ddf54b251a2d883db36c74470172) | Add client speed units and independent account lifecycle |
| 24 | 2026-10-11 | [`8963307`](https://github.com/admin6501/vpn-ui/commit/8963307c677737a41163e4c10ab0a84a0adea492) | Fix credential leakage, session revocation and browser security boundaries |
| 25 | 2026-10-11 | [`bcff9db`](https://github.com/admin6501/vpn-ui/commit/bcff9db4654c8ba4fdc9acd32a176114739358cd) | Clarify how existing nodes receive the security-patched Xray core |
| 26 | 2026-10-11 | [`5688762`](https://github.com/admin6501/vpn-ui/commit/5688762af9e71b517a7a5cf40b8afa65e3f6419d) | Revoke open WebSockets before sending data after credential changes |
| 27 | 2026-10-11 | [`53ce456`](https://github.com/admin6501/vpn-ui/commit/53ce456040c52496b40fc2a086ff0090d079cb8f) | Keep TPROXY socket setup working when TCP_USER_TIMEOUT is unsupported |
| 28 | 2026-10-11 | [`dfa11e2`](https://github.com/admin6501/vpn-ui/commit/dfa11e242ec57fb2e594dc03bd7a32723bdb2a3c) | Fix Hysteria2 authentication and HTTP3 profiles; sort accounts by traffic and days |
| 29 | 2026-10-11 | [`85e050e`](https://github.com/admin6501/vpn-ui/commit/85e050ec65f78db7e05c0f65f7eb74ea87c716ee) | Fix client info polling, expired tooltips and exact protocol liveness |
| 30 | 2026-10-12 | [`2fcc584`](https://github.com/admin6501/vpn-ui/commit/2fcc584c1eff5f435acffa9c8db786ea8ed549d3) | Show WireGuard handshake sessions online and delegate node management |
| 31 | 2026-10-12 | [`abc2f37`](https://github.com/admin6501/vpn-ui/commit/abc2f37434ee244c958d18d2fa2a129e55826a13) | Add administrator roles and protect independent panel ownership |
| 32 | 2026-10-12 | [`a0ae3fa`](https://github.com/admin6501/vpn-ui/commit/a0ae3fa62d5ecb7fb6f3f114aa4c8d1d0a5dba5f) | Fix client endpoints and SSH details, 2FA enrollment and Hysteria obfs form |
| 33 | 2026-10-12 | [`b20c6d2`](https://github.com/admin6501/vpn-ui/commit/b20c6d20e5f35ad886fd5b8cc1e600ed75038e51) | Fix SSH credentials and external endpoints in Account Info expanders |
| 34 | 2026-10-12 | [`8c66639`](https://github.com/admin6501/vpn-ui/commit/8c66639af8c58e97dff64b6f14b7598fe477eeca) | Fix intermittent panel navigation 403 while preserving API origin protection |
| 35 | 2026-10-12 | [`56d6daf`](https://github.com/admin6501/vpn-ui/commit/56d6daf1abbe3c0db925608f09b2544718ec1611) | Add confirmed dedicated node uninstall with ownership-aware host cleanup |
| 36 | 2026-10-12 | [`4cb223c`](https://github.com/admin6501/vpn-ui/commit/4cb223cf12b3b8c59c67b8561a29320ad8a9fa28) | Clear node installer RETURN trap before leaving its local scope |
| 37 | 2026-10-12 | [`e8e413d`](https://github.com/admin6501/vpn-ui/commit/e8e413dddf693fef64eefd1f32782bd1c13e3596) | Keep node services running during additional core setup and streamline Amnezia prerequisites |
| 38 | 2026-10-12 | [`f9c393d`](https://github.com/admin6501/vpn-ui/commit/f9c393d300e4d8eac7387fc48c5b13d2847175c3) | Activate reinstalled node cores after provisioning completes |
| 39 | 2026-10-12 | [`867028f`](https://github.com/admin6501/vpn-ui/commit/867028ffab29a9fc6757a2dcf667f4ba3e80c398) | Remove node cores without stopping sibling services and classify sync failures |
| 40 | 2026-10-12 | [`9e25843`](https://github.com/admin6501/vpn-ui/commit/9e2584351ee070ee92d83005cb558dcac725a20b) | Show one descriptive Xray option in the node core installer |
| 41 | 2026-10-12 | [`5687972`](https://github.com/admin6501/vpn-ui/commit/568797282f28376b5b32b96c5366a5d3e79f816f) | Target the node core picker explicitly in browser regression coverage |
| 42 | 2026-10-13 | [`64ef8ee`](https://github.com/admin6501/vpn-ui/commit/64ef8ee1e8f2584688aa970f642a62602358ff2e) | Repair reused node inbound tags and retain scoped counter history |
| 43 | 2026-10-13 | [`d9a91a7`](https://github.com/admin6501/vpn-ui/commit/d9a91a78c787c03894a48ddb45b27e0589624aac) | Isolate integration node and client working directories |
| 44 | 2026-10-13 | [`eef74a6`](https://github.com/admin6501/vpn-ui/commit/eef74a6eac31b9213fc6b0b43cc7cf3978d369ed) | Restore reset admission and complete shared Xray speed thresholds |
| 45 | 2026-10-13 | [`b6a89bb`](https://github.com/admin6501/vpn-ui/commit/b6a89bbe548466d2e89e226e74d6a4f3bbe9d056) | Scope WireGuard rate buckets and recover live counters after edits |
| 46 | 2026-10-13 | [`560d06a`](https://github.com/admin6501/vpn-ui/commit/560d06add6d14f05ebf24bdfa0988a4ed98b0a6e) | Download managed WireGuard peer files as raw conf configurations |
| 47 | 2026-10-13 | [`aff57d6`](https://github.com/admin6501/vpn-ui/commit/aff57d69a4360b3729a63eef54ef7ebec6c04665) | Exercise inbound counter publication instead of matching variable names |
| 48 | 2026-10-13 | [`1859181`](https://github.com/admin6501/vpn-ui/commit/185918161f5ed8a6e9933ea93b2cd7bbdb14e8dc) | Fix implicit MPTCP listeners breaking native VPN TPROXY |
| 49 | 2026-10-13 | [`dc50357`](https://github.com/admin6501/vpn-ui/commit/dc50357062230c25eacd46974cad280adb11b00d) | Expose OpenVPN TCP and UDP profiles in client membership details |
| 50 | 2026-10-13 | [`05e3580`](https://github.com/admin6501/vpn-ui/commit/05e35807a86558d03aba9e9ad3591343473ca7ff) | Add selectable speed units to both inbound edit forms |
| 51 | 2026-10-14 | [`73afd93`](https://github.com/admin6501/vpn-ui/commit/73afd93351a511b33b664084ada86a1a9f9cb29a) | Restrict representatives to client operations and show their own quota |
| 52 | 2026-10-14 | [`67cb5d4`](https://github.com/admin6501/vpn-ui/commit/67cb5d40a85764a28fb26e3f7c227486d75e3242) | Preserve administrator inbound access with imported role IDs |
| 53 | 2026-10-14 | [`016eb6a`](https://github.com/admin6501/vpn-ui/commit/016eb6abdcea43b6317039ceeab46b0a4c3b5857) | Fix six audited security and listener readiness issues |
| 54 | 2026-10-14 | [`c9eca2d`](https://github.com/admin6501/vpn-ui/commit/c9eca2de24ba5683ebfd49a3d866e559220e75d1) | Align representative Overview grants and retain inbound speed display units |
| 55 | 2026-10-14 | [`b376dca`](https://github.com/admin6501/vpn-ui/commit/b376dcaf961071ee3d6be947919ebd9900b4151e) | Honor complete delegated panel settings permissions |

## روش بازتولید

```sh
git fetch origin --tags
git rev-parse --is-shallow-repository
git log origin/main --reverse --format=fuller --stat
git log origin/main -- web/controller/setting.go web/service/user.go web/session/session.go database/model/model.go
git branch -r
git tag
```

این گزارش نسخهٔ پیش از تغییر حاضر را مستند می‌کند؛ کامیت قابلیت جدید در جدول بالا شمرده نشده است.
