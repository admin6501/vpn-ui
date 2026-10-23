# Security audit fixes — 2026-10-14

## Stable v1.0.0 security scope

The first stable release retains authenticated sessions bound to account security state, protected owner operations, role/assignment checks, representative two-factor enrollment, verified node TLS/leases and private backups. The pinned toolchain/dependency checks are defined by [go.mod](go.mod) and the [release workflow](.github/workflows/release.yml), including dependency scanning. Per-request HTML language isolation prevents concurrent visitors changing each other's displayed locale. Subscription tokens and exported client profiles remain secrets.

Dated audit findings below are records of the examined source and fixes, not a guarantee that a running server is uncompromised. Database/configuration comparison alone cannot inspect every live runtime or host persistence mechanism. Do not disable TLS/origin/authentication protections to diagnose access issues. For backup and operating procedures see the [panel guide](docs/PANEL-GUIDE.md).

Delegated settings follow-up: the explicitly granted panel-settings permission
now opens and manages the complete settings page through roles or custom grants,
including Telegram, LDAP, certificates and the systemd service. Settings API
routes use this grant rather than an additional owner-only check. Per-account
password/2FA operations still target only the caller, and the retired global 2FA
secret is never returned. Protected owner-account edits, database access and
executable replacement retain their separate guards. API tests cover manual and
role grants, absent/revoked permissions and input validation; Chromium verifies
load/save/reload in English and Persian.

These fixes address the six confirmed findings reviewed against
`67cb5d40a85764a28fb26e3f7c227486d75e3242`. Update both the central panel and
location agents to receive the embedded Xray fix.

| Finding | Change | Compatibility checks |
| --- | --- | --- |
| A1: OpenVPN directive injection | Validate DNS as IP addresses and reject control characters in certificate paths, both on writes and configuration generation. Quote paths using OpenVPN token escaping. | IPv4/IPv6 DNS, paths containing spaces and quotes, inline certificates, optional TLS-Crypt, and both TCP/UDP exports. |
| A2: host file transfer through node snapshots | Read only active, protocol-specific certificate fields. Require regular, bounded files with valid certificate/key/static-key material. Stage bundles before publishing them. | Certificate chains, managed renewal symlinks, native OpenVPN static keys and ordinary node synchronization. |
| A3: SSH host key exposure | Redact the singular `hostKey` from representative inbound reads, including the shared WebSocket read model. | Keep the representative's own SSH credentials and retain the stored server identity. |
| A4: incomplete socket setup reported as ready | Propagate listener controller and configured socket-option failures. Preserve the existing unsupported optional TCP timeout fallback and explicit MPTCP behavior. | Actual TCP/UDP TPROXY sockets, socket marks, normal TCP and existing native VPN data-plane tests. |
| A5: overview status permission bypass | Apply Overview permission checks to server status/history endpoints and revalidate live roles before WebSocket status delivery. Remove global traffic from representative status and hide its graph. | Assigned client operations, UUID generation, own representative quota and client refreshes after Overview revocation. |
| A6: Telegram backup without owner authorization | Require the panel owner and Overview management on `POST /panel/api/backuptotgbot`; GET cannot trigger a backup. | Owner access and rejection of delegated managers and non-owner super administrators; tests never send a Telegram message. |

## Certificate path permissions

A delegated inbound administrator can keep an existing certificate binding, use
inline certificate material, or choose a certificate from SSL Manager. Only the
panel owner can introduce a new external file path through inbound creation,
editing or import. Existing certificate paths and renewal links remain in place;
this change does not move certificates or replace client keys.

## Existing SSH host keys

The redaction fix does not rotate server host keys, which would invalidate
clients' existing host-key pins. If an untrusted representative previously had
access to an SSH inbound, the owner should rotate that inbound's host key and
update affected clients' trusted fingerprints.

## Verification scope

Regression tests cover the six reported cases and corresponding allowed
behavior. The release workflow also exercises both CPU architectures, native VPN
integration, node billing and synchronization, subscriptions, browser navigation,
installation/rollback and dependency scans. These checks do not replace testing
an operator's own firewall, kernel and client networks.

## Representative Overview permission follow-up

An imported or default representative role can contain `system.read=true` while
that representative's explicit Overview switch is off. The switch now governs
navigation, direct page access, server APIs and live WebSocket status consistently.
It also governs Overview management separately. Removing access hides an already
open dashboard and navigates back through the server's current page permissions;
client operations remain available. Host status/Xray events are withheld without
the required view permission, and Overview responses use `Cache-Control: no-store`.
