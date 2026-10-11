# Local QUIC security patch

This directory preserves the apernet/quic-go transport fork at commit
`6c6cc9bcb716` (v0.59.1-0.20260425001925-6c6cc9bcb716).

The upstream quic-go v0.59.1 changes to `http3/headers.go` and
`http3/headers_test.go` are backported to fix GO-2026-5676: decoded HTTP/3
trailers must fit the header budget, including repeated QPACK fields.
The fork-specific DATAGRAM support is preserved. Original licenses remain.

Run `go test ./http3 -run TestResponseTrailer -count=1` in this directory.
