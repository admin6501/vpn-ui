# VPN-UI — نسخة admin6501

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [中文](README_ZH.md)

لوحة VPN مبنية على Sir-MmD/vpn-ui و3X-UI. تتضمن ترحيل مستخدمي PasarGuard وملكية المسؤولين والمضيفين ورموز الاشتراك، وإدارة أدوار الممثلين وحساب المستخدمين الفريدين. لا تُستورد العقد.

**Repository:** [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui) · **Release:** [v1.0.0 / latest](https://github.com/admin6501/vpn-ui/releases/latest)

## التثبيت

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

## التثبيت دون إنترنت

نزّل vpn-ui-amd64 وinstall-offline.sh وSHA256SUMS من الإصدار، وانقلها إلى المجلد نفسه. يتطلب Linux amd64 وroot وsystemd. قد تتطلب بعض البروتوكولات حزم النظام ووحدات النواة المثبتة مسبقاً.

```bash
sudo bash install-offline.sh ./vpn-ui-amd64
```

## المصدر والتوثيق

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh
```

[English documentation](README.md) · [Offline setup/recovery](docs/OFFLINE-INSTALL.md) · [Migration (فارسی)](migration/INSTALL-MIGRATION.fa.md) · [Architecture](docs/ARCHITECTURE.md) · [Tests](test_unit/README.md)

Ubuntu 24.04 / Debian 12–13 recommended. Full protocol/distribution coverage has not been verified for this fork. Certificate issuance/renewal requires internet. Updates preserve credentials; fresh installs print random credentials. PasarGuard subscription tokens remain unchanged, but a new server domain/IP needs a new base URL.

**License:** [GPL-3.0](LICENSE). Based on [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui) and [3X-UI](https://github.com/MHSanaei/3x-ui); original attribution is preserved.
