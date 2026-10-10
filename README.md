# VPN-UI — admin6501

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [中文](README_ZH.md)

Multi-protocol VPN panel based on [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui), derived from [3X-UI](https://github.com/MHSanaei/3x-ui). Source, installation and updates for this fork use **admin6501/vpn-ui**. The release series starts at **v1.0.0** after the repository history reset; this includes the existing fork fixes.

## Features

- Xray protocols plus native AnyTLS, TUIC and NaiveProxy; portable VPN backends, Telegram integration and per-account limits.
- PasarGuard backup migration: users, administrator ownership, inbound assignments, hosts and existing subscription tokens. Nodes are not imported.
- Representative roles with add/edit/delete controls, scoped account visibility and administrator ownership transfer. Assigned roles cannot be deleted.
- Overview totals and online counts count unique accounts across inbounds.
- Persian/English migration prompts, Persian form and Telegram language fixes; no donation interface or representative external-proxy permission.
- Full release binary includes portable cores and all six geodata files: base, Iran and Russia geoip/geosite. Kernel modules and OS packages cannot all be embedded.

## Protocols and dependencies

The panel includes Xray VLESS, VMess, Trojan, Shadowsocks and native AnyTLS/TUIC/NaiveProxy, plus OpenVPN, OpenConnect, L2TP/IPsec, PPTP, SSTP, IKEv2, WireGuard/AmneziaWG and other bundled backends. Per-protocol host requirements are listed in the cores page; an embedded daemon alone does not supply TUN/PPP/IPsec/kernel support. [Architecture](docs/ARCHITECTURE.md) describes RADIUS and the accounting bridge.

## Installation

The published binary is **Linux amd64**, requires **root and systemd**, and installs under `/opt/vpn-ui`. Prefer Ubuntu 24.04 or Debian 12/13. The upstream targets additional distributions, but this fork has not completed the full VM/protocol matrix on every distribution. Ubuntu 22.04 is not officially supported upstream.

### Online

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

The installer prints credentials, port and web path. Open the exact URL with the correct HTTP/HTTPS scheme. Default new installations use HTTP; configure TLS separately. `0.0.0.0` is a listen address, not the address to enter in a browser. Other panels can coexist only if their listening ports do not collide.

### Offline

Download `vpn-ui-amd64`, `install-offline.sh` and optionally `SHA256SUMS` from [the latest release](https://github.com/admin6501/vpn-ui/releases/latest) on a connected machine. Transfer them into the same directory on the server:

```bash
sudo bash install-offline.sh ./vpn-ui-amd64
```

No panel, core or package download is performed by this installer. It installs the menu/service, generates credentials for a new database and preserves existing credentials on updates. It keeps a private rollback backup. Some protocols require preinstalled `nftables`, `iproute2`, kernel modules or Libreswan/DKMS; missing packages cannot be installed from a binary alone. See [offline installation and recovery](docs/OFFLINE-INSTALL.md).

## PasarGuard migration

Install the destination panel first. The destination must contain no VPN accounts or inbounds; existing web/TLS settings are preserved. Then:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/install.sh)
```

Choose Persian/English and enter the absolute backup ZIP path. The migration package contains scripts/config only, not another panel binary. It needs Python 3, unzip and the installed compatible panel. Existing PasarGuard administrator password hashes are imported: use the old password; the plaintext password cannot be recovered. Tokens remain unchanged, but the server domain/IP can change, so distribute the new subscription base URL when needed.

See [the detailed migration guide](migration/INSTALL-MIGRATION.fa.md). Conversion was verified with the supplied PostgreSQL/Timescale sample containing 319 users, six administrators and three hosts. This does not establish live VPN compatibility for every configuration or complete PasarGuard feature parity.

## Management and recovery

Run `sudo vpn-ui` for the menu. Back up the database and TLS/configuration before upgrading. Stop the service before copying SQLite files and include any WAL/SHM sidecars. The offline installer records its backup location. Uninstall is destructive:

```bash
sudo /opt/vpn-ui/vpn-ui-amd64 --uninstall
```

## Build and validation

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh
```

Building is an online development operation: use the Go version from `go.mod`, a C toolchain and the backend build dependencies (including Docker where the build scripts require it). Run `./build.sh --help` for options. Do not skip core/geodata/backend builds when preparing a full release. Git submodules pin the upstream core sources.

Release checks run in [the build workflow](.github/workflows/release.yml). The [Incus end-to-end harness](test_unit/README.md) requires its own VM host and outbound internet; its presence does not mean every matrix test passed for this release. [Architecture](docs/ARCHITECTURE.md) and [bundled ACME client](build/acme/README.md) explain the implementation. ACME issuance/renewal needs internet even though its client is bundled.

## License and attribution

[GPL-3.0](LICENSE). Original project and third-party copyright/license notices remain intact. Vendored/submodule READMEs document their respective upstream components, not this fork's release/install process.
