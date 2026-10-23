# TLS certificate pinning security update

**Stable v1.0.0:** retain the pinned-CA hostname verification fixes and the release workflow's real TLS regressions. A configured CA fingerprint does not justify ignoring the destination name. This guide concerns outbound transport TLS; node pairing additionally uses verified HTTPS and an optional trusted CA file. See [Nodes](NODES.md) and [security scope](../SECURITY.md).

The release build requires Go 1.27.3 to include the crypto/x509 distrust update
for [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809). Both the panel and bundled
Xray build use the toolchain selected by the root go.mod and the core security
dependency patch.

The panel's Xray library is pinned to the first upstream revision fixing
[GO-2026-6651](https://pkg.go.dev/vuln/GO-2026-6651), commit
[64fada32b5b9](https://github.com/XTLS/Xray-core/commit/64fada32b5b9e6ae064a038fa0e4e2b766499bd5).
The separately built custom Xray fork retains its existing features and receives
the same fix through build/core/tls-ca-pin.patch and the Hysteria compatibility
patch. The submodule revision is unchanged.

Pinning a CA certificate must verify the peer's name as well as its chain. Set
serverName or verifyPeerCertByName, or use an outbound destination whose domain/IP
matches the certificate. gRPC and Hysteria now pass the outbound destination into
the shared TLS configuration. Exact leaf-certificate pinning keeps its existing
behavior, and Hysteria keeps its required HTTP/3 ALPN.

The release workflow tests missing/wrong names, matching names, explicit name
overrides, unrelated certificate chains and transport destination handling. Both
panel and embedded-core vulnerability scans remain required. Applying the patches
twice and building from a clean pinned checkout must produce the same fixed code.
