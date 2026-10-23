# تاریخچهٔ توسعه و اولین انتشار پایدار

`v1.0.0` اولین نسخهٔ پایدار این فورک است؛ شماره‌نسخه‌ها و نتایج قدیمیِ داخل طرح‌ها، انتشار پایدار قبلی این فورک محسوب نمی‌شوند.
مرجع تاریخچهٔ زیر، کامیت `42c92d6a0c8d2349654e3a7338608c1fedb45dcd` پیش از هماهنگ‌سازی مستندات و ترجمه‌های نسخهٔ پایدار است.
تعداد کامیت‌های قابل دسترسی در آن نقطه: **40**. برای وضعیت منتشرشده، [یادداشت نسخه](RELEASE-NOTES-v1.0.0.md) و [ریلیز GitHub](https://github.com/admin6501/vpn-ui/releases/tag/v1.0.0) مرجع‌اند.

عنوان‌های جدول، پیام واقعی کامیت‌ها هستند و ادعای اجرای دوبارهٔ آزمون‌های تاریخی ندارند. ۱۰۰ کامیت آخر:

| کامیت | تاریخ | تغییر ثبت‌شده |
| --- | --- | --- |
| 42c92d6 | 2026-10-21 | Merge pull request #28 from admin6501/feat/client-renew |
| ce5be79 | 2026-10-21 | Add client renewal preserving quota, credentials and inbound access |
| 466fc1a | 2026-10-21 | Merge pull request #27 from admin6501/fix/go1273-xray-ca-pinning |
| 4bc3e33 | 2026-10-21 | Fix Go certificate distrust and Xray CA pinning release vulnerabilities |
| a2bd114 | 2026-10-21 | Merge pull request #26 from admin6501/fix/node-menu-ci-portability |
| d0c5675 | 2026-10-21 | Keep node menu regression tests portable on release runners |
| 536b851 | 2026-10-21 | Merge pull request #25 from admin6501/fix/node-sync-ipsec-menu |
| 41efe7b | 2026-10-21 | Speed up node synchronization and add IPsec selection and node menu |
| 8ea9ad8 | 2026-10-19 | Merge pull request #24 from admin6501/fix/gre-peer-public-ip-editor |
| 0d0632c | 2026-10-19 | Allow editing GRE peer router public IPs from setup |
| 1277d76 | 2026-10-19 | Merge pull request #23 from admin6501/fix/brand-reset-browser-workflow |
| a7288f0 | 2026-10-19 | Run branding and client reset browser checks with Playwright setup |
| 21249ca | 2026-10-19 | Merge pull request #22 from admin6501/fix/gre-l2tp-client-tools-brand |
| a279a30 | 2026-10-19 | Fix GRE/L2TP coexistence and add edit reset and panel branding |
| fea02c3 | 2026-10-19 | Merge pull request #21 from admin6501/fix/blocked-client-disable |
| f0df53c | 2026-10-19 | Fix manual client disable while representative is blocked |
| 0cb6722 | 2026-10-18 | Merge pull request #20 from admin6501/fix/subscription-browser-page |
| 92d6387 | 2026-10-18 | fix: preserve subscription page data during representative suspension |
| da21a93 | 2026-10-18 | Merge pull request #19 from admin6501/fix/preserve-representative-username-case |
| 0b1f622 | 2026-10-18 | fix: preserve administrator and representative username spelling |
| 4d149ba | 2026-10-16 | Merge pull request #18 from admin6501/fix/expired-includes-exhausted-quota |
| 2390b1c | 2026-10-16 | Count quota-exhausted accounts as expired |
| bb7e673 | 2026-10-16 | Merge pull request #14 from admin6501/fix/first-navigation-and-config-names |
| 0a5a2f4 | 2026-10-16 | Fix initial panel navigation and preserve inbound names in shared configs |
| c4f5db4 | 2026-10-16 | Merge pull request #13 from admin6501/fix/panel-refresh-performance |
| d777831 | 2026-10-16 | Reduce traffic write contention and repair panel refresh lifecycle |
| 5a24db0 | 2026-10-16 | Merge pull request #12 from admin6501/fix/exact-inbound-traffic |
| acc015f | 2026-10-16 | Record exact Xray membership traffic and retain concurrent meters |
| 663ec8c | 2026-10-16 | Merge pull request #11 from admin6501/fix/account-info-shared-traffic |
| c827618 | 2026-10-15 | Run account usage regressions before publishing release assets |
| 2c3a965 | 2026-10-15 | Fix duplicated shared traffic in account info breakdown |
| 95acc0c | 2026-10-15 | Merge pull request #10 from admin6501/fix/repeated-native-backup-restore |
| 262da51 | 2026-10-15 | Complete repeated native restores within the original authorized request |
| dc958ee | 2026-10-15 | Merge pull request #9 from admin6501/fix/representative-role-group-defaults |
| f0dd663 | 2026-10-15 | Preselect allowed representative groups when choosing a role |
| 8655c5c | 2026-10-15 | Merge pull request #8 from admin6501/fix/admin-group-permissions-and-access |
| 83500c2 | 2026-10-15 | Assign administrator groups and delegate independent group permissions |
| a043ac9 | 2026-10-15 | Merge pull request #7 from admin6501/fix/representative-overview-connection-counts |
| f764a80 | 2026-10-15 | Preserve representative Overview TCP and UDP connection counts |
| 5fd5940 | 2026-10-15 | Merge pull request #6 from admin6501/fix/imported-groups-representative-access |

[راهنمای فعلی پنل](PANEL-GUIDE.fa.md) · [فهرست مستندات](INDEX.md)
