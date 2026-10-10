# Documentation index / فهرست مستندات

## راهنماهای اجرایی / Operational guides

| Topic                                                    | Guide                                                                                        |
| -------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Installation, migration and features                     | [English README](../README.md), [راهنمای فارسی](../README_FA.md)                             |
| Local binary installation and optional apt prerequisites | [Offline installation](OFFLINE-INSTALL.md)                                                   |
| PasarGuard ZIP conversion                                | [Migration guide](../migration/INSTALL-MIGRATION.fa.md)                                      |
| Menu and binary CLI commands                             | [Management CLI](../management-cli-plan.md)                                                  |
| Account/membership upgrade and recovery                  | [Accounts upgrade](../accounts-upgrade-guide.md)                                             |
| API calls, roles and ownership transfer                  | [API reference](../api-reference.md)                                                         |
| Distribution targets and release architecture            | [Distribution reference](../table.md)                                                        |
| Automated unit/release checks and separate VM testing    | [Test harness](../test_unit/README.md), [release workflow](../.github/workflows/release.yml) |
| ACME client and network requirements                     | [Bundled ACME client](../build/acme/README.md)                                               |

نصب آفلاین پیش از مرحلهٔ اختیاری apt فقط یک سؤال Yes/No می‌پرسد. با No پنل از فایل محلی نصب می‌شود؛
Yes برای نصب پیش‌نیازهای میزبان به مخازن apt نیاز دارد. اگر apt شکست بخورد، خطا گزارش می‌شود و نصب
محلی پنل ادامه دارد. برای برخی کرنل‌ها هدر یا ماژول مناسب در مخزن موجود نیست؛ وضعیت پروتکل را جدا
بررسی کنید.

The offline installer asks before an optional online apt step. No keeps the panel installation
local; Yes installs host prerequisites if repositories are reachable. A package failure is reported
and does not block the local panel installation.

## معماری / Architecture

- [Protocol architecture](ARCHITECTURE.md)
- [مهلت اتصال‌ها و مسیر بومی TCP](connection-lifetimes.fa.md)
- [نمودار و توضیح فارسی](../flowchart.md)
- [English data/control-plane diagram](../flowchart_en.md)

These are implementation explanations, not proof of live connectivity for every protocol or host.
PasarGuard nodes are not part of this fork.

## یادداشت‌های طراحی / Design records

The following root documents now begin with a current-source summary and links. Their historical
sections retain upstream proposals, old versions, dated live-box results and development decisions.
Those records are not current release test results or instructions to implement features that are
already present.

| Area                                                       | Records                                                                                                                                                    |
| ---------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Account identity, addressing and multi-inbound memberships | [Stable slots](../account-slot-plan.md), [Multi-inbound accounts](../multi-inbound-client-plan.md)                                                         |
| Representative roles and consumption accounting            | [Representatives](../reseller-plan.md)                                                                                                                     |
| Speed, source-IP and device/session limits                 | [Speed](../speed-limit-plan.md), [IP limits](../ip-limiter-plan.md), [Device identity](../device-limit-plan.md)                                            |
| Kernel VPNs                                                | [WireGuard](../wireguard-plan.md), [AmneziaWG](../amneziawg-plan.md), [GRE](../gre-plan.md)                                                                |
| Daemon-backed VPNs                                         | [IKEv2](../ikev2-plan.md), [SSTP](../sstp-plan.md), [OpenConnect](../openconnect-plan.md), [Dated OpenConnect diagnosis](../openconnect-remaining-bugs.md) |
| Relays and carriers                                        | [SSH](../ssh-plan.md), [Tunnel carriers](../tunnel-carrier-plan.md)                                                                                        |
| Control-plane research                                     | [Framework proposal](../control-plane-framework.md)                                                                                                        |
| UI research                                                | [Redesign](../redesign-plan.md), [Inbound list](../inbound-list-plan.md)                                                                                   |

## Scope of verification

Release checks cover focused behavior and systemd installation/update/rollback. The optional apt
helper is tested with fake apt executables so tests do not install host packages. Incus
live-protocol testing needs a separately prepared VM host and network access. Historical “green”
reports are attributed to their original development sessions, not rerun as part of a documentation
update.

Vendored/submodule READMEs describe their own upstream components. Their original attribution and
licensing remain intact; use this index for this fork’s operating instructions.
