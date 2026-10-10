# Dedicated VPN-UI nodes

One central panel can manage separate location servers. Accounts, owners, quotas,
subscription tokens and representative limits stay in the central database.
Assign the same account to inbounds on several nodes to publish those locations in
one subscription and use one shared quota.

## Setup

1. Update the central panel using `vpn-ui` → Update, or run `deploy.sh` from this repository.
2. Use an HTTPS panel URL with its exact web path. A publicly trusted certificate
   works without Cloudflare; DNS validation is optional. For a private certificate,
   pass its trusted CA certificate with `--ca-file` on the node.
3. As the main administrator, open **Nodes / locations**, add a name and the
   node's public IP or hostname, and copy its installation command.
4. Run the copied command as root on a separate location server. It installs the
   agent and systemd service. No VPN core is extracted, installed or started yet.
5. In the central node card, open **Cores**, select the cores you need, and click
   **Install selected**. The agent receives the request on its next poll. Watch
   the actual installation steps/status; a queued command is not a completed setup.
6. Add an inbound, choose its **Server / location**, then assign accounts to it.
   Existing inbounds cannot be moved between hosts. Create another inbound on the
   destination and attach the same accounts instead.

The installer optionally asks Yes/No before installing apt prerequisites. Answer
No to keep installation local. Kernel VPNs still require the matching kernel
modules, networking utilities and privileges. If provisioning requests a reboot,
reboot the **node**, not the central server. The agent does not reboot automatically.

## Cores and protocols

The agent reuses the panel's data-plane implementations: Xray-native protocols,
OpenVPN, L2TP, PPTP, OpenConnect, SSTP, IKEv2, kernel WireGuard, AmneziaWG, GRE,
MTProto and the built-in SSH proxy. The embedded RADIUS service is an internal
VPN dependency and does not appear as an independently installable core.

Selecting a core installs that core and its required dependencies. Xray and
geodata are also extracted when another VPN core needs routing/accounting. This
never selects every unrelated core. The agent binary carries bundled assets, so
its download size does not describe which cores are installed on disk.

Kernel support depends on the node's OS/kernel. An implementation being available
in the panel does not guarantee that its kernel modules are available on every VPS.

## Offline installation and updates

Copy the architecture-matching release asset (`vpn-ui-node-amd64` or
`vpn-ui-node-arm64`) and `install-node.sh` to the location server. Verify the
release checksums before copying. Then run:

```bash
bash install-node.sh --binary ./vpn-ui-node-arm64 \
  --panel-url 'https://panel.example.com/YOUR_WEB_PATH/' \
  --pair-token 'TOKEN_COPIED_FROM_THE_CENTRAL_PANEL' --lang en
```

For x86-64, replace the binary name with `vpn-ui-node-amd64`. Answer No to the apt
question on an offline host. Central-panel connectivity is still necessary: this
means installation without downloading assets, not operation without management.

For an existing paired node, update with `bash install-node.sh --binary ./vpn-ui-node-arm64`.
The agent configuration, database and cumulative counters are retained. Do not
remove the agent database or reuse it for a different node. A fresh reinstall
needs a new node record; attach accounts to new inbounds rather than resetting
accepted counters on an existing record. Back up `/opt/vpn-ui-node` securely.

## Connection and accounting

The node polls the central panel every five seconds over verified TLS. No inbound
node management port, SSH password or remote admin login is needed. Allow the
panel's HTTPS port outbound and each configured VPN port inbound on the node.
Pairing codes expire after 15 minutes and can connect only one agent. Ordinary
node lists contain neither the agent token nor pairing credentials.

Cumulative reports and their sequence numbers persist in SQLite. Central billing
and accepted counters commit in one transaction, so retrying a lost response does
not bill the same bytes twice. Each location receives its account's remaining
shared quota. Bytes transferred after the last acknowledged report are deducted
from that local budget too. Different traffic multipliers within one node use the
highest active multiplier for that account, conservatively.

Shared quota and disable changes propagate at the polling interval; traffic
already in flight and concurrent use across locations can exceed a limit before
the next update. IP/session limits operate per node, not as a globally atomic
cross-server session lock. Online counts deduplicate account identities and stop
using stale node reports after 35 seconds.

If the agent cannot refresh its management lease for 90 seconds, it disables
admission and stops its VPN data plane while keeping counters. A fresh valid
snapshot restores service. This prevents a disconnected node serving indefinitely
with old quota/permissions; it also means a prolonged central outage interrupts
node service.

## Troubleshooting

```bash
systemctl status vpn-ui-node --no-pager
journalctl -u vpn-ui-node -n 100 --no-pager
```

Check the central node card's last error, core status and provisioning steps.
Use the exact HTTPS URL and web path. A certificate error requires a correct
certificate or trusted CA, not disabling TLS verification. A private CA supplied
at install time is copied to `/opt/vpn-ui-node/panel-ca.crt`.

The release workflow tests actual TCP forwarding through a selected Xray core and
returning those bytes to central quota accounting. Other protocol and kernel
implementations share the existing runtime; every kernel protocol has not been
live-tested on every supported distribution.
