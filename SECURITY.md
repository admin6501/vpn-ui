# Security and deployment

## Security update (2026-10-11)

This update fixes confirmed issues in the panel:

- Submitted login passwords no longer enter logs or Telegram login notifications.
- Signed login cookies are bound to current credentials and two-factor settings.
  Password changes and disabling an administrator revoke previous sessions;
  logout revokes every session for that account, including copied cookies.
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

All users must log in again after installing this update. Update both the panel
and dedicated node agents to refresh their embedded Xray binaries.

Old logs, Telegram messages and backups are not erased by this update. If login
failures or incorrect two-factor codes were submitted on an older version,
rotate the affected panel passwords and restrict access to those old records.
Existing copies of backups may still have their old filesystem permissions.

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

The overview management permission includes database export/restore and binary
replacement. Grant it only to fully trusted administrators. Those operations
intentionally provide access to sensitive data and control over the service.
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

This review is not a guarantee that all vulnerabilities are absent. Third-party
non-Go VPN engines and live-server operating-system configuration have not had a
complete penetration test as part of this review.

Report suspected vulnerabilities privately to the repository owner before
posting credentials, tokens or exploit details publicly.
