# VPN-UI — admin6501 sürümü

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) |
[Русский](README_RU.md) | [Türkçe](README_TR.md) | [中文](README_ZH.md)

Sir-MmD/vpn-ui ve 3X-UI tabanlı panel. PasarGuard kullanıcıları, yönetici sahipliği, hostlar ve
abonelik tokenlarını taşır; temsilci rollerini yönetir ve benzersiz kullanıcıları sayar. Düğümler
aktarılmaz.

**Repository:** [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui) · **Release:**
[v1.0.0 / latest](https://github.com/admin6501/vpn-ui/releases/latest)

## Kurulum

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

## Çevrimdışı kurulum

Sürümden vpn-ui-amd64, install-offline.sh ve SHA256SUMS dosyalarını indirip aynı dizine aktarın.
Linux amd64 / arm64, root ve systemd gerekir. Bazı protokoller önceden kurulmuş sistem paketleri ve çekirdek
modülleri gerektirir.

```bash
sudo bash install-offline.sh ./vpn-ui-amd64
```

Betik, apt ile ön koşulları kurmadan önce Yes/No sorar. No yerel kurulumu sürdürür; Yes internet
gerektirir. apt başarısız olursa hata bildirilir ve yerel panel kurulumu devam eder.

[Documentation index](docs/INDEX.md)

## Kaynak ve belgeler

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh
```

[English documentation](README.md) · [Offline setup/recovery](docs/OFFLINE-INSTALL.md) ·
[Migration (فارسی)](migration/INSTALL-MIGRATION.fa.md) · [Architecture](docs/ARCHITECTURE.md) ·
[Tests](test_unit/README.md)

Ubuntu 24.04 / Debian 12–13 recommended. Full protocol/distribution coverage has not been verified
for this fork. Certificate issuance/renewal requires internet. Updates preserve credentials; fresh
installs print random credentials. PasarGuard subscription tokens remain unchanged, but a new server
domain/IP needs a new base URL.

**License:** [GPL-3.0](LICENSE). Based on [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui) and
[3X-UI](https://github.com/MHSanaei/3x-ui); original attribution is preserved.

ARM64 (aarch64) için `vpn-ui-arm64` indirin; çevrimiçi kurucu mimariyi otomatik algılar.
Uyumluluk için kurulum yolu her iki mimaride `/opt/vpn-ui/vpn-ui-amd64` olarak kalır.
