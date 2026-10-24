# Operating VPN-UI v1.0.0

This guide describes the first stable release of `admin6501/vpn-ui`. Start with the
[installation guide](../README.md), then use the exact URL and credentials printed by the installer.
The [documentation index](INDEX.md) links protocol-specific references and historical designs.

## First login and branding

Use a publicly trusted HTTPS certificate or a trusted private CA before pairing remote nodes.
Set your own password and enroll your authenticator in **Account security**. Preserve recovery
material privately. Initial random credentials, the listen port and web base path are independent.
A browser 403 does not by itself prove intrusion: inspect reverse-proxy rules, request origin and
server logs rather than disabling authentication or origin checks.

In **Panel settings**, set the brand name. Panel titles, login/Overview, subscription presentation
and other brand-aware controls use that value; protocol identifiers, executable names, download
filenames and the project's attribution keep their technical names. Choose one of thirteen web
languages. Telegram has its own configured bot language. Installers are English; migration offers
Persian and English. See [translation behavior](TRANSLATIONS.md).

## Inbounds, users and groups

1. Install the required core on the central server or selected node. IPsec is selectable and is a
   shared dependency for L2TP/IKEv2. Kernel protocols also need suitable host modules.
2. Add an inbound with its listener, port/transport, external endpoint and location. A group does not need to exist yet; the listener starts with no accounts, including after a cipher change or clone. Verify port
   availability and firewall rules. Moving an existing inbound between hosts is not supported.
3. Create an account in **Users** and assign its groups/inbounds. Accounts have one identity and
   shared quota across their memberships; memberships have separate access switches.
4. Open **Info** or the subscription page, download the appropriate client profile and test an
   actual connection. For External Proxy, verify the advertised endpoint, port and label.

Deleting an inbound removes its group links; it keeps accounts, subscription identities and their
usage. Recreating the same name/port creates a new listener identity. Add that replacement to the
existing group explicitly to restore its users; matching a name never grants access automatically.

Groups are reusable access assignments. Edit group membership instead of duplicating credentials
for every location. Administrator scope and representative ownership still restrict assignments.
Two distinct accounts can share a Sub ID and appear in one subscription, but remain distinct
identities with separate quota/state. Use a single account with multiple memberships for a shared
quota. A subscription token is a credential: never publish screenshots containing complete links,
UUIDs, passwords, private keys or QR codes.

## Reading status and usage

A checked **Enabled** switch records the administrator's intent. Effective access additionally
requires an enabled membership/inbound, available quota, a valid expiry, permitted owner policy
and an available runtime/location. Thus an account can be enabled but its subscription unavailable.
When a representative runs out of allowance, the configured blocking policy still applies; a
customer switch does not automatically override the representative's allowance.

Overview counts accounts, while inbound lists describe memberships/services. They need not show
the same count. Expired/exhausted views include accounts whose finite time or traffic is finished.
Usage is upload plus download at the panel's accounting layer. Tunnel/interface/provider meters
can also include encryption, encapsulation, retransmissions and both legs of a forwarded flow.
A discrepancy with a tunnel menu alone is not proof of a panel accounting loss.

Info separates measured per-inbound traffic from any historical unattributed remainder. Upgrading
cannot reconstruct source inbounds from old account-only totals. This remainder can occur with
any protocol; it is not another hidden user's traffic. Preserve a consistent backup before
repairing records, and compare newly measured traffic separately from historical consumption.
Lifetime/representative accounting and current-period traffic are different counters.

Native restore and upgrade preserve shared account counters, including accounts without a
legacy home inbound or with no memberships. Deleting an Xray outbound changes its configuration
without resetting client traffic. If an older migration already deleted a shared counter, compare
a matching backup before restoring usage; an upgrade alone cannot recreate the lost history.

## Renewing and resetting a customer

Use **Users → three-dot menu → Renew**. This clears current upload/download for the account and
its memberships, restarts the complete recorded package duration from now and enables the account.
Unknown old package durations prompt for days. A time-unlimited account stays time-unlimited;
a traffic-unlimited account keeps that property. Credentials, quota, access assignments, device
keys, peer addresses and the subscription URL are preserved. Lifetime/node billing cursors remain.
See [full renewal and permission rules](CLIENT-RENEWAL.md).

**Reset traffic**, also available beside quota in the client editor, clears current usage only;
it does not restart expiry or deliberately disabled access. **Freeze** pauses remaining days;
unfreeze restores the remaining time rather than a fresh full package. Automatic renewal is a
separate policy and is not switched on by manual Renew.

## Speed, addresses and devices

Client download/upload overrides inherit the inbound value when empty; zero is unlimited.
Positive rates are stored in 1024-byte KB/s. MB/s uses 1024-based bytes; Mbps is decimal bits.
For example, 3 MB/s is 24 Mib/s, approximately 25.17 Mbps. The restrictive active policy governs
shared account pacing. Threshold-based throttling follows current-period usage and resets with it.

Source-IP limits count visible addresses, not physical devices behind NAT. Device/session limits
vary by protocol and appear in its form. WireGuard and AmneziaWG assign individual device keys
and profiles; copying the same profile shares the same peer. Keep a GRE router's outer endpoint
reachable through its original gateway before routing the default through the tunnel. Enter each
peer's public IP in the client editor when pinning it. Plain GRE is not encrypted.

## Representatives, administrators and nodes

The initial administrator is the protected owner. Roles/grants govern operations and their
scope; explicit panel-settings access includes its permitted settings subsections. Representatives
have separate user-update and usage-reset rights; manual renewal needs both. Consumption-based
accounts retain lifetime consumption, while existing prepaid billing follows its package rules.
Two-factor requirements are enforced per signed-in account, including required enrollment.

Nodes are dedicated agents paired to one central panel. Their initial installation enables no
VPN core. Install selected cores from the node card, then add location inbounds and assignments.
Poll/page refresh is one second; diagnostics normally refresh every five seconds. Commands remain
pending until acknowledged. Counters survive updates. Online reports are stale after 35 seconds;
a management lease lost for 90 seconds stops the data plane. See [Nodes](NODES.md).

## Time, backups and recovery

In the panel's time-location setting, use the IANA name `Asia/Tehran`. The default `Local` means
the server process's local zone. The server OS, panel scheduler and browser calendar can use
different time zones; check each clock instead of assuming that changing one rewrites stored
Unix timestamps. On the host, `timedatectl` shows OS configuration. A Unix expiry stores an instant,
not a timezone; changing its display zone must not add extra subscription time.

Use a consistent SQLite backup or stop the service before copying database files; retain relevant
WAL/SHM sidecars. Back up TLS files, systemd overrides and node databases privately. Do not restore
an old node counter database onto an already accepted central record. After an update, verify
login, settings, one real connection per deployed protocol, subscription rendering and node status.

For diagnosis use `systemctl status vpn-ui --no-pager` and
`journalctl -u vpn-ui -n 100 --no-pager`; node service names end in `-node`.
The [unlisted-user checker](../scripts/check-unlisted-xray-users.py) compares configured identities;
a zero result is not a complete host compromise assessment. Inspect live runtime and host access
separately. No database-only check can certify an entire server clean.
