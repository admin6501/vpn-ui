# VPN-UI — Versión admin6501

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [中文](README_ZH.md)

Panel basado en Sir-MmD/vpn-ui y 3X-UI. Incluye migración de usuarios de PasarGuard, propiedad por administrador, hosts y tokens de suscripción, gestión de roles y recuentos de usuarios únicos. No importa nodos.

**Repository:** [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui) · **Release:** [v1.0.0 / latest](https://github.com/admin6501/vpn-ui/releases/latest)

## Instalación

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

## Instalación sin conexión

Descarga vpn-ui-amd64, install-offline.sh y SHA256SUMS de la publicación y transfiérelos al mismo directorio. Requiere Linux amd64, root y systemd. Algunos protocolos necesitan paquetes del sistema y módulos del kernel previamente instalados.

```bash
sudo bash install-offline.sh ./vpn-ui-amd64
```

## Código y documentación

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh
```

[English documentation](README.md) · [Offline setup/recovery](docs/OFFLINE-INSTALL.md) · [Migration (فارسی)](migration/INSTALL-MIGRATION.fa.md) · [Architecture](docs/ARCHITECTURE.md) · [Tests](test_unit/README.md)

Ubuntu 24.04 / Debian 12–13 recommended. Full protocol/distribution coverage has not been verified for this fork. Certificate issuance/renewal requires internet. Updates preserve credentials; fresh installs print random credentials. PasarGuard subscription tokens remain unchanged, but a new server domain/IP needs a new base URL.

**License:** [GPL-3.0](LICENSE). Based on [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui) and [3X-UI](https://github.com/MHSanaei/3x-ui); original attribution is preserved.
