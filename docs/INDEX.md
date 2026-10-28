# Documentation / مستندات v1.0.0

**First stable release of this fork / اولین نسخهٔ پایدار این فورک.**

## Installation in thirteen languages

[English](../README.md) | [فارسی](../README_FA.md) | [العربية](../README_AR.md) | [Español](../README_ES.md) | [Bahasa Indonesia](../README_ID.md) | [日本語](../README_JA.md) | [Português (Brasil)](../README_PT_BR.md) | [Русский](../README_RU.md) | [Türkçe](../README_TR.md) | [Українська](../README_UK.md) | [Tiếng Việt](../README_VI.md) | [简体中文](../README_ZH.md) | [繁體中文](../README_ZH_TW.md)

## Current operating guides

| Topic | Guide |
| --- | --- |
| Users, quota, subscriptions, branding and time | [Panel guide](PANEL-GUIDE.md), [راهنمای فارسی](PANEL-GUIDE.fa.md) |
| Manual renewal and reset permissions | [Client renewal](CLIENT-RENEWAL.md) |
| Dedicated locations, core/runtime controls and installer menu | [Nodes](NODES.md), [نودها](NODES.fa.md) |
| Local panel/agent files, checksums and optional apt step | [Offline installation](OFFLINE-INSTALL.md) |
| PasarGuard conversion | [Migration](../migration/INSTALL-MIGRATION.fa.md) |
| Management menus and recovery | [CLI](../management-cli-plan.md) |
| Database/account backfill and preserved state | [Upgrade guide](../accounts-upgrade-guide.md) |
| HTTP calls and endpoint inventory | [API reference](../api-reference.md), [route inventory](API-ROUTES.md), [راهنمای API نمایندگان و ربات فروش](REPRESENTATIVE-BOT-API.fa.md) |
| Data/control paths | [Architecture](ARCHITECTURE.md), [فارسی](../flowchart.md), [English](../flowchart_en.md) |
| Connection lifetimes | [Timeout reference](connection-lifetimes.fa.md) |
| Groups | [گروه‌ها](groups-fa.md) |
| Thirteen-language behavior and checks | [Translations](TRANSLATIONS.md) |
| Release scope and development history | [Stable notes](RELEASE-NOTES-v1.0.0.md), [تاریخچه](history-fa.md) |
| Host requirements and separate live VM matrix | [Targets](../table.md), [tests](../test_unit/README.md) |
| Certificates and security | [ACME](../build/acme/README.md), [CA pinning](TLS-CA-PIN-SECURITY.md), [security](../SECURITY.md), [dated audit](../SECURITY-AUDIT-FIXES.md) |

The panel, subscription page and bot support thirteen languages. Terminal installers remain English;
the migration wizard supports Persian and English. Vendored component guides describe their own upstreams.

## Feature references and historical records

The current section of each reference describes shipped behavior. Expand its historical section
for the original proposals, old defaults and dated development findings; those are not current
installation steps or newly performed live tests.

- [Stable account addressing](../account-slot-plan.md)
- [Accounts and memberships](../multi-inbound-client-plan.md)
- [Kernel WireGuard](../wireguard-plan.md)
- [AmneziaWG](../amneziawg-plan.md)
- [Device and session limits](../device-limit-plan.md)
- [Source-IP limiting](../ip-limiter-plan.md)
- [GRE operation](../gre-plan.md)
- [IKEv2 and shared IPsec](../ikev2-plan.md)
- [SSTP](../sstp-plan.md)
- [OpenConnect](../openconnect-plan.md)
- [OpenConnect diagnosis reference](../openconnect-remaining-bugs.md)
- [Built-in SSH proxy](../ssh-plan.md)
- [Representative administration](../reseller-plan.md)
- [Inbound and user pages](../inbound-list-plan.md)
- [Current interface](../redesign-plan.md)
- [Speed and reset behavior](../speed-limit-plan.md)
- [Tunnel carriers and routes](../tunnel-carrier-plan.md)
- [Current control plane](../control-plane-framework.md)
- [Management CLI](../management-cli-plan.md)

Offline installation asks before optional apt downloads; No or EOF retains local installation.
Kernel modules/packages remain host prerequisites. Release tests exercise focused behavior and both
native architectures; the privileged Incus protocol/distribution matrix is separate.
