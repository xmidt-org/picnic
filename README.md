# picnic

[![Build Status](https://github.com/xmidt-org/picnic/actions/workflows/ci.yml/badge.svg)](https://github.com/xmidt-org/picnic/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/xmidt-org/picnic/branch/main/graph/badge.svg?token=tWY4sd44iI)](https://codecov.io/gh/xmidt-org/picnic/tree/main)
[![Go Report Card](https://goreportcard.com/badge/github.com/xmidt-org/picnic)](https://goreportcard.com/report/github.com/xmidt-org/picnic)
[![Apache V2 License](http://img.shields.io/badge/license-Apache%20V2-blue.svg)](https://github.com/xmidt-org/picnic/blob/main/LICENSE)
[![GitHub Release](https://img.shields.io/github/release/xmidt-org/picnic.svg)](https://github.com/xmidt-org/picnic/releases)
[![GoDoc](https://pkg.go.dev/badge/github.com/xmidt-org/picnic)](https://pkg.go.dev/github.com/xmidt-org/picnic)

**pic**k **NIC** — bind sockets to a specific local network interface,
cross-platform, with **zero dependencies**.

picnic works through the two callbacks the standard library funnels *all* socket
creation through — `net.Dialer.Control` and `net.ListenConfig.Control` — so one
mechanism covers the whole stack:

| You want… | Routes through | picnic entry |
|-----------|----------------|--------------|
| raw TCP/UDP/unix dials, `net/http`, `crypto/tls`, WebSockets (over `http.Client`) | `net.Dialer.Control` | `BindDialer` |
| UDP → QUIC → HTTP/3 → WebTransport (e.g. quic-go over a `net.PacketConn`), TCP servers | `net.ListenConfig.Control` | `BindListenConfig` |
| anything else | either callback | `Control` |

```go
// Stream path — net/http, WebSocket, TLS, raw TCP:
var d net.Dialer
if err := picnic.Name("eth0").BindDialer(&d); err != nil {
    log.Fatal(err)
}
conn, err := d.DialContext(ctx, "tcp4", "example.com:443")

// Packet path — QUIC / HTTP-3 / WebTransport:
var lc net.ListenConfig
if err := picnic.Name("eth0").BindListenConfig(&lc); err != nil {
    log.Fatal(err)
}
pc, err := lc.ListenPacket(ctx, "udp4", ":0")   // hand pc to quic-go
```

## How it works

picnic binds the device itself with the platform's interface-bind socket option.
This steers egress *without* binding the socket's address, so it composes with a
later `connect` (dialer) or `bind` (listener) — which is why one mechanism serves
both the stream and packet paths.

| Platform | Mechanism | Privilege |
|----------|-----------|-----------|
| Linux | `SO_BINDTODEVICE` | needs `CAP_NET_RAW` |
| macOS | `IP_BOUND_IF` / `IPV6_BOUND_IF` | none |
| Windows | `IP_UNICAST_IF` / `IPV6_UNICAST_IF` | none |

When the device option is unavailable (Linux without `CAP_NET_RAW`, or a BSD that
has no such option), **`BindDialer`** falls back to binding a source address from
the interface. A dialer can do this because `connect` supplies the destination; a
listener cannot, because `ListenPacket` binds the socket itself — so on those
platforms `BindListenConfig` leaves the socket un-bound to the interface.

You never pass an IP version: picnic derives the family (which selects the v4 vs
v6 option) from the dial's network string (`tcp4`/`tcp6`) or the destination
address.

## Caveats

- The **source-address fallback is destination-blind.** On an interface holding
  both a global unicast (GUA) and a unique local (ULA) IPv6 address, picnic
  prefers the GUA, since a ULA source cannot reach a global destination — but no
  in-process source selection is perfect without the route. For *guaranteed*
  egress, use the device-bind path (on Linux, grant `CAP_NET_RAW`) or pair source
  binding with OS policy routing.
- **`BindListenConfig` needs the device option.** On Linux without `CAP_NET_RAW`,
  or a BSD with no per-socket interface bind, a `ListenPacket` socket cannot be
  interface-bound (its own bind precludes the source-address fallback). macOS and
  Windows need no privilege, so the common QUIC targets are covered.

## Why a separate package

Binding to an interface is per-OS socket-option code (`SO_BINDTODEVICE`,
`IP_BOUND_IF`, `IP_UNICAST_IF`). That can only be meaningfully tested on real
Linux, macOS, and Windows runners — which is exactly what this repository's CI
matrix does. Keeping it standalone also gives the wider Go ecosystem the small,
permissively licensed, dependency-free helper it currently lacks.

## License

Apache-2.0. See [LICENSE](LICENSE).
