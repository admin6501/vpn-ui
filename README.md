# VPN-UI v1.0.0 — First stable release

[English](README.md) | [فارسی](README_FA.md) | [العربية](README_AR.md) | [Español](README_ES.md) | [Bahasa Indonesia](README_ID.md) | [日本語](README_JA.md) | [Português (Brasil)](README_PT_BR.md) | [Русский](README_RU.md) | [Türkçe](README_TR.md) | [Українська](README_UK.md) | [Tiếng Việt](README_VI.md) | [简体中文](README_ZH.md) | [繁體中文](README_ZH_TW.md)

VPN-UI is a multi-protocol VPN management panel. **v1.0.0 is the first stable release of this fork.** Source, installers and updates come from [admin6501/vpn-ui](https://github.com/admin6501/vpn-ui), based on [Sir-MmD/vpn-ui](https://github.com/Sir-MmD/vpn-ui) and [3X-UI](https://github.com/MHSanaei/3x-ui).

## Thirteen languages

The panel, subscription browser page, Telegram bot and shared UI controls support English, Persian, Arabic, Spanish, Indonesian, Japanese, Brazilian Portuguese, Russian, Turkish, Ukrainian, Vietnamese, Simplified Chinese and Traditional Chinese. Choose the web language in the language selector; configure the bot language in Telegram settings. The bot uses the configured language, not a separate automatic preference for each Telegram chat.

Web requests keep their own language, including concurrent panel and subscription requests. Regional variants resolve to supported locales. Dialogs, calendars and pagination use the selected language. Protocol identifiers, configuration keys, command names and diagnostic output retain their original syntax. Terminal installers use English; the migration wizard offers Persian and English. See [docs/TRANSLATIONS.md](docs/TRANSLATIONS.md).

## Protocols and cores

| Runtime | Protocols and implementations |
| --- | --- |
| Bundled Xray | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, AnyTLS, TUIC, NaiveProxy, WireGuard |
| Portable VPN backends | OpenVPN (TCP/UDP), OpenConnect (ocserv), L2TP, PPTP, SSTP and IKEv2 |
| Kernel tunnels | WireGuard (`wg-c`), AmneziaWG (`awg`) and GRE, with optional IPsec or FOU where supported |
| Built-in relays | MTProto proxy and SSH proxy with per-account authentication |

IPsec is selectable in core installation and is also a shared dependency of L2TP/IKEv2. Required dependencies are retained while another installed core needs them. Kernel WireGuard and native Xray WireGuard are different implementations. The built-in SSH feature forwards VPN traffic; it does not provide a root shell.

The full binaries include portable backends, the patched Xray core, an ACME client and six geodata files: base, Iran and Russia geoip/geosite. OS packages, kernel modules, kernel headers and DKMS remain host requirements. The Cores page shows installation progress, missing prerequisites and actual service status. Plain GRE is unencrypted unless IPsec is enabled; PPTP is a legacy protocol. See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Users, groups and subscriptions

Manage customers in **Users**, infrastructure in **Inbounds**, and reusable inbound assignments in **Groups**. One account may use several compatible inbounds and locations with a shared quota and subscription. Group changes update assignments without replacing account credentials. Individual membership switches and the global account switch remain separate.

Traffic limits are stored in bytes despite the API field name `totalGB`. An account with zero traffic limit is unlimited in traffic; an expiry of zero means unlimited time. Delayed-start plans begin on first use. Overview totals count unique accounts, not one user for every membership. Sharing a subscription ID between distinct accounts publishes their configurations together; it does not merge their identities or turn their separate limits into one shared account quota.

The bundled core attributes new usage to its source inbound. Account Info shows measured shares and any historical unattributed remainder separately. Upgrading preserves existing usage; old account-only records cannot recreate a historical split. External Proxy endpoints are used consistently in account/configuration exports. OpenVPN profiles and WireGuard peer files can be downloaded from their corresponding configuration views.

### Renew, reset traffic and freeze

**Users → the client's three-dot menu → Renew** clears current upload/download across its memberships, starts the full package duration from now and enables the account. Traffic-only, time-only, combined and fully unlimited plans are supported. Quota, password, UUID, subscription ID, device keys, peer addresses and inbound access remain unchanged. Unknown legacy durations ask for the number of days once and remember it.

**Reset traffic** clears current usage without restarting expiry or enabling a manually disabled account. **Freeze** pauses remaining days; unfreezing restores the remainder. Renewal uses the full recorded package duration instead. Lifetime accounting and cumulative node counters are preserved. Representative limits and disabled memberships still apply. See [docs/CLIENT-RENEWAL.md](docs/CLIENT-RENEWAL.md) and [docs/PANEL-GUIDE.md](docs/PANEL-GUIDE.md).

### Speed and access limits

Download/upload overrides accept KB/s, MB/s or Mbps. Empty inherits the inbound setting; zero means unlimited in that direction. Storage uses 1024-byte KB/s, while Mbps means decimal megabits. The UI's MB/s uses 1024-based bytes: **3 MB/s = 24 Mib/s ≈ 25.17 Mbps**. Speed values are rounded to at least 1 KB/s when positive.

Source-IP limits count visible addresses, not physical devices behind one NAT. Session and peer limits depend on the protocol. WireGuard/AmneziaWG issue separate device configurations. Supported limits are shown in each form; node session/IP limits are local to each node.

## Administrators and representatives

The initial administrator is the protected panel owner. Administrator roles and custom grants determine operations; group/inbound assignments determine their scope. Owner-sensitive database, authentication and executable operations retain separate protection. An explicit panel-settings grant opens its complete permitted settings page, including Telegram, LDAP, certificates and service settings.

New representative accounts use consumption-based accounting and scoped roles. Existing prepaid accounts retain their own billing rules. Update, delete and usage-reset grants are separate. Manual renewal requires both update and reset permission with the narrower scope. Ownership transfer preserves customer credentials and is restricted to eligible owners. A representative whose allowance is exhausted remains subject to the configured disconnection policy.

Every signed-in account can manage its own authenticator in **Account security**. Enrollment requires the current password and a valid one-time code. Managers can require two-factor enrollment for representatives; pending enrollment permits only account security and logout. Secrets and subscription links must be treated as credentials.

## Multi-location nodes

One central panel manages dedicated location agents over outbound verified HTTPS. A node is paired with one panel. Install the agent on a separate location server, then select the required cores in the node's Cores dialog; initial agent installation activates no VPN core. Delegate node management with **Manage nodes and locations**. Representatives cannot obtain this infrastructure grant.

Agent polling and the visible Nodes page refresh every second. Host diagnostics refresh separately, normally every five seconds. Unchanged snapshots avoid rewriting client tables; changed configuration reconciles the affected cores. Core installation/removal, runtime controls and configuration edits show persistent progress and acknowledgments.

Accounts retain one central quota and subscription across locations. Accepted cumulative reports are not billed twice. Changes propagate asynchronously; in-flight traffic and simultaneous locations can briefly exceed a quota. Online node data becomes stale after 35 seconds. Losing the management lease for 90 seconds stops the VPN data plane while retaining counters; a fresh valid snapshot restores service. See [docs/NODES.md](docs/NODES.md) and [docs/NODES.fa.md](docs/NODES.fa.md).

## System requirements

Linux amd64 (x86-64) or arm64 (aarch64), root access, Bash and a running systemd system are required. Ubuntu 24.04 and Debian 12/13 are recommended deployment targets. Host networking privileges and protocol-specific modules are still necessary. A successful panel installation does not certify every protocol on every distribution, kernel or VPS. See [table.md](table.md).

The standard installation directory is `/opt/vpn-ui`. The installed executable keeps the compatibility name `/opt/vpn-ui/vpn-ui-amd64` on both architectures. Bundled core filenames follow their actual architecture. The node uses `/opt/vpn-ui-node` and its own systemd service.

## Install or update online

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/deploy.sh -o deploy.sh
sudo bash deploy.sh
```

The same command updates an existing standard installation while retaining its database and login settings. Back up first. Fresh installations print random credentials, port and web path. Open the exact URL with the printed HTTP/HTTPS scheme and replace a listen address such as `0.0.0.0` with the server's reachable address. Fresh installations use HTTP until TLS is configured.

For an existing paired node, download and run its installer; choose install/update from the menu. Existing pairing and counters are preserved:

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/install-node.sh -o install-node.sh
sudo bash install-node.sh
```

A new node must use the pairing command generated in the central panel. The node menu also provides status, start, stop, restart, recent logs and confirmed removal. `--install` keeps the direct non-menu update path. See [management-cli-plan.md](management-cli-plan.md).

## Offline installation and update

Download the architecture-matching binary, `install-offline.sh` and `SHA256SUMS` from the [stable release](https://github.com/admin6501/vpn-ui/releases/tag/v1.0.0). Transfer them together and verify the available files before installation:

```bash
sha256sum --ignore-missing -c SHA256SUMS
sudo bash install-offline.sh ./vpn-ui-amd64
# ARM64: sudo bash install-offline.sh ./vpn-ui-arm64
```

The optional apt prerequisite question defaults to **No**, including EOF. No keeps panel installation local; Yes requires reachable apt repositories. apt failures are reported and local installation continues. Preinstall or transfer the required OS packages and kernel modules separately. Updates preserve the database, credentials and TLS settings and keep private rollback backups.

The installer records `VPNUI_OFFLINE=1`, disabling automatic package downloads, public-IP lookup and geodata updates. VPN traffic still works; manual updates, WARP and ACME issuance/renewal still need internet. For offline node installation use the matching node binary and `install-node.sh` with its pairing options. See [docs/OFFLINE-INSTALL.md](docs/OFFLINE-INSTALL.md).

## PasarGuard backup migration

Install a compatible destination panel first. Its VPN accounts and inbounds must be empty; existing panel web/TLS settings are preserved. Run:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/migration/install.sh)
```

Choose Persian or English and supply the absolute backup ZIP path. The migration package contains scripts and configuration, not another panel binary. Python 3 and unzip are required. For offline conversion, transfer and extract `vpn-ui-auto-migration.zip`, then run its `migrate.sh` directly with the backup path.

Supported account identities, administrator ownership, groups/assignments, hosts, password hashes and subscription compatibility metadata are imported. Use the previous administrator password; hashes cannot reveal the plaintext password. PasarGuard nodes are not imported. A changed domain/IP requires updating the subscription base address and checking exported endpoints. Review converted configuration and test the protocols you actually use. See [migration/INSTALL-MIGRATION.fa.md](migration/INSTALL-MIGRATION.fa.md).

## Operation, backup and recovery

```bash
sudo vpn-ui
systemctl status vpn-ui --no-pager
journalctl -u vpn-ui -n 100 --no-pager
```

Use the printed private login details or the management menu to recover access. Stop the service before copying SQLite files and retain WAL/SHM sidecars, or make a consistent SQLite backup. Preserve TLS material, service overrides and node databases securely. A traffic reset or renewal is not a database restore. Uninstall removes data; inspect its confirmation before continuing. See [accounts-upgrade-guide.md](accounts-upgrade-guide.md) and [SECURITY.md](SECURITY.md).

## Release assets

| File | Purpose |
| --- | --- |
| `vpn-ui-amd64` | Full central panel for Linux x86-64 |
| `vpn-ui-arm64` | Full central panel for Linux aarch64 |
| `vpn-ui-node-amd64` | Dedicated location agent for Linux x86-64 |
| `vpn-ui-node-arm64` | Dedicated location agent for Linux aarch64 |
| `install-offline.sh` | Local panel installer/update with rollback support |
| `install-node.sh` | Node installation, update and management menu |
| `vpn-ui-auto-migration.zip` | PasarGuard conversion scripts and configuration |
| `SHA256SUMS` | SHA-256 checksums for the seven downloadable artifacts above |

## Build, testing and documentation

```bash
git clone --recurse-submodules https://github.com/admin6501/vpn-ui.git
cd vpn-ui
./build.sh --help
./build.sh
```

Use the Go toolchain from `go.mod` and the pinned submodules/security patches. A full release must build its cores, backends and all geodata; skipped bundles produce development binaries. Building requires online dependencies, a C toolchain and the backend build tools, including Docker where required. ACME needs internet even with its client bundled.

The release workflow validates both native architectures, focused accounting and permission behavior, real Xray/node data paths, offline install/update, and browser behavior in supported languages. The separate Incus protocol/distribution matrix requires a prepared privileged VM host and internet; its presence is not evidence that every matrix entry was rerun. See [test_unit/README.md](test_unit/README.md), [docs/INDEX.md](docs/INDEX.md), [api-reference.md](api-reference.md) and [.github/workflows/release.yml](.github/workflows/release.yml).

## License

[GPL-3.0](LICENSE). Original project and third-party notices remain intact. Vendored/submodule READMEs describe their own components; use this fork's guides for panel installation and operation.


## Telegram sales bot

An optional Telegram sales bot for VPN-UI can be installed on an Ubuntu/Debian server. It includes service management and a wallet. A fresh install creates a new local database; it does not import an existing Cloudflare bot. See [bot instructions](telegram-bot/README-server.txt).

```bash
curl -fsSL https://raw.githubusercontent.com/admin6501/vpn-ui/main/scripts/install-telegram-bot.sh -o install-telegram-bot.sh && sudo bash install-telegram-bot.sh
```
