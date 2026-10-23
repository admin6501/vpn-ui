# Security and deployment

## Stable v1.0.0 security scope

The first stable release retains authenticated sessions bound to account security state, protected owner operations, role/assignment checks, representative two-factor enrollment, verified node TLS/leases and private backups. The pinned toolchain/dependency checks are defined by [go.mod](go.mod) and the [release workflow](.github/workflows/release.yml), including dependency scanning. Per-request HTML language isolation prevents concurrent visitors changing each other's displayed locale. Subscription tokens and exported client profiles remain secrets.

Dated audit findings below are records of the examined source and fixes, not a guarantee that a running server is uncompromised. Database/configuration comparison alone cannot inspect every live runtime or host persistence mechanism. Do not disable TLS/origin/authentication protections to diagnose access issues. For backup and operating procedures see the [panel guide](docs/PANEL-GUIDE.md).

## Security update (2026-10-11)

This update fixes confirmed issues in the panel:

- Submitted login passwords no longer enter logs or Telegram login notifications.
- Signed login cookies are bound to current credentials and two-factor settings.
  Password changes and disabling an administrator revoke previous sessions;
  logout revokes every session for that account, including copied cookies.
  Existing WebSockets revalidate credentials before sending each update.
- Existing authenticators cannot be replaced using a new authenticator's code.
  First-time authenticator enrollment requires the current password.
- Login and credential-change requests have bounded bodies and rate limits.
  Untrusted forwarded IP headers cannot bypass the per-IP limit.
- Browser requests and WebSocket upgrades check the complete origin, including
  scheme and port. Config previews and connection logs display untrusted values
  as text rather than executable HTML.
- Database files, SQLite sidecars and newly created database backups use mode
  0600. New database directories use mode 0700.
- Known vulnerable Go dependencies were updated in both the panel and embedded
  Xray. The custom QUIC fork retains its transport features and includes the
  upstream HTTP/3 trailer decoding fix (GO-2026-5676).

Hysteria managed inbounds reject shared fallback authentication when their account
list is empty. Duplicate credentials cannot replace another account. Removed accounts
cannot open new TCP streams or continue sending UDP datagrams on an authenticated
QUIC connection; authentication state is synchronized with stream dispatch.

All users must log in again after installing this update. Update both the panel
and dedicated node agents. For each existing node, then request installation
of Xray again from the central panel's Nodes → Cores page. This refreshes the
on-disk Xray binary; updating the agent alone preserves existing installed cores.

Old logs, Telegram messages and backups are not erased by this update. If login
failures or incorrect two-factor codes were submitted on an older version,
rotate the affected panel passwords and restrict access to those old records.
Existing copies of backups may still have their old filesystem permissions.

## Browser navigation and domain checks

User-initiated top-level GET navigation to the registered panel HTML pages is allowed
through Fetch Metadata checks, including the existing-session redirect after opening
an external link. Login and role/permission guards still decide access. The exception
matches the configured base path and registered route exactly; it does not permit
cross-site API reads, POST requests, iframes, logout or WebSocket connections. Explicit
foreign Origin headers remain rejected.

Domain checks compare DNS names without case or a final root dot and normalize IP
literals; other domains and malformed authorities remain forbidden. Panel middleware
403 responses are not cached and include `X-VPN-UI-Error: origin-rejected` or
`domain-rejected` for diagnosis without exposing credentials or the private base path.

## HTTPS and reverse proxies

Use HTTPS for browser access. HTTP cannot protect passwords or cookies in transit.
Native panel TLS sets Secure login cookies automatically.

The default configuration trusts no reverse proxy headers. If HTTPS terminates
at a proxy, configure only its actual IP addresses or CIDRs using the service
variable `VPN_UI_TRUSTED_PROXIES`. For a local reverse proxy, for example:

```ini
# systemctl edit vpn-ui
[Service]
Environment="VPN_UI_TRUSTED_PROXIES=127.0.0.1,::1"
```

Then run `systemctl daemon-reload` and `systemctl restart vpn-ui`. The proxy must
preserve the public Host header and overwrite X-Forwarded-Proto and forwarded
client IP headers. Restrict direct access to the backend when using a proxy.
Do not trust 0.0.0.0/0 or ::/0.

## Administrative permissions

The historic first active administrator is recorded as the protected panel owner
on upgrade; new installs mark the initial account as owner. Ownership is independent
of the super-administrator switch. The active owner retains administrator and role
management, so it can restrict its operational permissions without losing recovery
access. Other super administrators cannot edit, disable or delete that account, or
change a role assigned to it.

Database export/restore, executable replacement, private authentication settings and
central daemon configuration writes additionally require the protected owner. These
operations could otherwise bypass account protection. Node management remains an
explicit delegated permission and applies to the remote node.

Administrator roles are live permission sets. Existing sessions use the current
role on each request; missing roles grant no operational permissions. Inbound access
is assigned per administrator. Roles never grant super-administrator status or panel
ownership. Assigned roles must be detached before deletion. The owner can repair a
missing role through administrator management.
Protect database backups: they contain credential hashes, VPN credentials and
subscription tokens. The panel's process and root account require protection.

## Verification and scope

Regression tests cover authentication, credential changes, copied-cookie
revocation, cross-origin requests, proxy spoofing, private database/backup modes,
role and ownership boundaries, and browser HTML injection. CI scans the panel
and embedded Xray with govulncheck and tests the backported QUIC trailer limit.
The final scan finds no affected symbols or imported packages. The unused
`golang.org/x/crypto/openpgp` package has an advisory without a fixed version;
neither the panel nor Xray imports it.

This review is not a guarantee that all vulnerabilities are absent. Other bundled
third-party VPN engines and live-server operating-system configuration have not had a
complete penetration test as part of this review.

Report suspected vulnerabilities privately to the repository owner before
posting credentials, tokens or exploit details publicly.
