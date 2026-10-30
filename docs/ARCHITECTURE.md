# Protocol architecture

## Stable v1.0.0 control and presentation layers

Central accounts own credentials, quota, expiry and subscription identity. Memberships and groups determine inbound access; role/owner policy bounds every operation. Jobs project changes into native and daemon-backed data paths. Dedicated location agents receive authenticated snapshots and return cumulative, sequenced counters. Nodes poll every second; host diagnostics normally refresh every five seconds. A lost 90-second management lease stops their data plane. See [node architecture and limits](NODES.md).

HTML templates use thirteen bounded language-specific renderers. Middleware resolves cookie/header dialects per request; panel and subscription renders no longer depend on a concurrently mutable global language. The built-in management Telegram bot uses its configured language. The separate sales bot currently uses Persian; see [sales bot](../telegram-bot/README-SALES-BOT.md). Shared component/calendar assets are local. See [translation checks](TRANSLATIONS.md).

The diagrams below explain implementations, not certification of every protocol/kernel/distribution combination. The cplane proposal remains historical research. For current UI operations use the [panel guide](PANEL-GUIDE.md).

These diagrams explain representative data/control paths, not a complete protocol inventory or
live-connectivity certification. GRE also uses a managed interface and nftables; see
[its service](../web/service/gre.go). Host prerequisites are listed in
[the offline guide](OFFLINE-INSTALL.md).

## How the New Protocols Interact with Xray-core

```mermaid
flowchart TB
  Client["VPN Client<br/>(L2TP/IPsec · PPTP · OpenVPN · OpenConnect · SSTP · IKEv2 · WireGuard (C) · AmneziaWG)"]
  TGC["Telegram Client<br/>(MTProto Proxy)"]
  SSHC["SSH Client<br/>(ssh -D dynamic SOCKS · badvpn-udpgw for UDP)"]
  GREC["Customer Router<br/>(GRE · IP protocol 47 · optional IPsec / FOU)"]

  subgraph PANEL["vpn-ui panel — root process"]
    PROC["procmgr<br/>supervises the daemons"]
    RAD["in-binary RADIUS<br/>127.0.0.1:1812 auth · :1813 acct"]
    HOOK["OpenVPN hooks<br/>auth / connect / disconnect / evict"]
    CONF["writes Xray config:<br/>dokodemo-door inbound +<br/>per-account source-IP routing"]
    STAT["reads Xray stats (gRPC)<br/>enforces traffic / device limits"]
    SSHSRV["in-binary SSH gateway (x/crypto/ssh)<br/>no daemon, no bundle: direct-tcpip + udpgw"]
  end

  subgraph DAEMON["Bundled VPN daemons (panel children)"]
    D["xl2tpd + strongSwan/charon · pptpd · openvpn · ocserv · accel-ppp<br/>(pppd for L2TP/PPTP · accel-ppp for SSTP · charon for IKEv2 and GRE-over-IPsec)"]
    MT["telemt (MTProto Proxy)<br/>userspace relay: no tunnel, no pool IP"]
  end

  subgraph KERNEL["Linux kernel data plane"]
    IFACE["ppp0 / tun0 / wgc0 / awg0 / gre-*<br/>client is assigned a pool IP"]
    NFT["nftables mark:<br/>UDP → TPROXY · TCP → REDIRECT"]
    RULE["ip rule fwmark 1 → table 100"]
  end

  subgraph XRAY["Xray-core (bundled, panel-managed)"]
    DOKO["dokodemo-door inbound<br/>sockopt tproxy, mark 255"]
    SOCKS["socks inbound (loopback)<br/>tag = MTProto / SSH inbound<br/>username = account"]
    ROUTE{"routing:<br/>match source IP → account<br/>or socks username → account"}
    OUT["outbound<br/>freedom / proxy / WARP"]
  end

  NET["Internet"]

  %% control plane
  Client -->|"tunnel + credentials"| D
  Client -.->|"WireGuard (C): in-kernel wgc, no daemon"| IFACE
  Client -.->|"AmneziaWG: in-kernel awg (DKMS module), no daemon<br/>obfuscated handshake: Jc/Jmin/Jmax · S1/S2 · H1-H4"| IFACE
  GREC -.->|"GRE: in-kernel tunnel, no daemon and no credentials<br/>peer pinned by its public IP, or learned from its first packets<br/>optional ESP transport on the shared charon · FOU for peers behind NAT"| IFACE
  TGC -->|"obfuscated2 / dd / FakeTLS secret"| MT
  SSHC -->|"username + password (checked in-process, no RADIUS)"| SSHSRV
  D -.->|"MS-CHAPv2 Access-Request"| RAD
  RAD -.->|"Accept + pool IP"| D
  D -.->|"user-pass / client-connect"| HOOK
  HOOK -.->|"lease per-account IP"| D
  PROC --- D
  CONF --> DOKO
  CONF --> ROUTE

  %% data plane
  D -->|"decapsulated packets"| IFACE
  IFACE --> NFT --> RULE --> DOKO
  DOKO --> ROUTE --> OUT --> NET
  MT -->|"relayed TCP, socks user = account"| SOCKS
  SSHSRV -->|"direct-tcpip → socks CONNECT · udpgw → socks UDP ASSOCIATE<br/>socks user = account"| SOCKS
  SOCKS --> ROUTE

  %% accounting + return
  OUT -.->|"per-account counters"| STAT
  MT -.->|"per-account octets (Prometheus scrape)"| STAT
  SSHSRV -.->|"per-account octets (in-process counters)"| STAT
  STAT -.->|"disconnect over-limit"| RAD
  NET -.->|"replies (symmetric path back)"| OUT
```

## How RBridge Handles Non-RADIUS Protocols

WireGuard (C), AmneziaWG and the IKEv2 **PSK** / **EAP-TLS** modes authenticate with a public key or
a certificate, so they never make a RADIUS round-trip. On their own they would get no session
record, no traffic accounting, and no **User Limit** enforcement. **RBridge** (Radius Bridge) closes
that gap: once per traffic tick its **Sweeper** polls each protocol's live tunnels, enforces quota,
disable, and the per-account **User Limit** K (evicting the losers), then reconciles the survivors
into the very same in-binary **RADIUS** session registry and **nftables** accounting the RADIUS
protocols already use. A key-based protocol therefore behaves identically for usage, quota, and
device limits, and egresses through the same Xray **dokodemo-door** data plane.

For the two key-based tunnel protocols, **WireGuard (C)** and **AmneziaWG**, a **User Limit** of K
provisions K device slots per account: K keypairs, K configs and K distinct tunnel IPs, one config
per device. That is the same model the commercial providers use, and it is what makes a single
account usable on a phone, a laptop and a router at once without the devices fighting over one key.

```mermaid
flowchart TB
  subgraph SRC["Non-RADIUS protocols (public-key / certificate auth, no RADIUS round-trip)"]
    WG["WireGuard (C)<br/>in-kernel, wgctrl-managed"]
    AWG["AmneziaWG<br/>in-kernel amneziawg (DKMS), obfuscated"]
    IKE["IKEv2 PSK / EAP-TLS<br/>strongSwan charon"]
  end

  subgraph BRIDGE["RBridge, the Radius Bridge (one pass per traffic tick)"]
    SWEEP["Sweeper.Tick()"]
    P1["1 · Poll live tunnels via each Adapter"]
    P2["2 · Enforce quota + disable<br/>+ User-Limit K + strategy"]
    P3["3 · Reconcile survivors into the Sink"]
  end

  subgraph SINK["Sink, the existing RADIUS session model"]
    REG["in-binary RADIUS<br/>session registry"]
    ACCT["nftables per-account counters<br/>→ client_traffics (usage / quota)"]
  end

  XRAY["Xray-core<br/>source-IP routing → outbound → Internet"]

  %% control plane
  WG -.->|"peers + last-handshake"| P1
  AWG -.->|"peers + last-handshake"| P1
  IKE -.->|"active SAs + Framed-IP"| P1
  SWEEP --> P1 --> P2 --> P3
  P2 -.->|"evict: remove peer / terminate SA"| WG
  P2 -.->|"evict: remove peer"| AWG
  P2 -.->|"evict: terminate SA"| IKE
  P3 -->|"tunnel IP → account"| REG
  P3 -->|"add / remove counters"| ACCT
  ACCT -.->|"disabled / over-quota"| P2

  %% data plane
  WG ==> XRAY
  AWG ==> XRAY
  IKE ==> XRAY
  ACCT -.- XRAY
```

## Dedicated nodes

`cmd/vpn-ui-node` reuses the data-plane runtime without panel/admin HTTP listeners.
`nodewire` is the versioned enrollment/snapshot/report protocol; `nodeagent` persists
report delivery and polls central HTTPS. `web/service/node.go` scopes assignments
and commits cumulative high-water marks with shared billing in one transaction.
`web/service/node_worker.go` projects snapshots without resetting local meters.
An inbound `node_id` of zero denotes the central server; runtime queries exclude
remote inbounds. See [node operation and accounting limits](NODES.md).
