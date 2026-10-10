# VPN-UI — Версия admin6501

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) |
[Русский](README_RU.md) | [Türkçe](README_TR.md) | [中文](README_ZH.md)

Панель на основе Sir-MmD/vpn-ui и 3X-UI. Поддерживает перенос пользователей PasarGuard,
владельцев-администраторов, хостов и токенов подписки, управление ролями и подсчёт уникальных
пользователей. Узлы не импортируются.

**Repository:** [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui) · **Release:**
[v1.0.0 / latest](https://github.com/admin6501/vpn-ui/releases/latest)

## Установка

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

## Офлайн-установка

Загрузите vpn-ui-amd64, install-offline.sh и SHA256SUMS из релиза и перенесите в одну папку.
Требуются Linux amd64, root и systemd. Некоторым протоколам нужны заранее установленные системные
пакеты и модули ядра.

```bash
sudo bash install-offline.sh ./vpn-ui-amd64
```

Скрипт спрашивает Yes/No перед установкой зависимостей через apt. No сохраняет локальный режим, Yes
требует интернета. При ошибке apt выводится сообщение, а установка панели из локального файла
продолжается.

[Documentation index](docs/INDEX.md)

## Исходный код и документация

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

ARM64 (aarch64): download `vpn-ui-arm64`; the online installer selects the architecture automatically.
The installed executable keeps the compatibility path `/opt/vpn-ui/vpn-ui-amd64` on both architectures.
