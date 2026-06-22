// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

// Package picnic ("pick NIC") binds sockets to a specific local network
// interface, cross-platform, with zero dependencies.
//
// It works through the two callbacks the standard library funnels all socket
// creation through — net.Dialer.Control and net.ListenConfig.Control (same
// signature) — so a single mechanism covers raw TCP/UDP, net/http, crypto/tls
// (tls.Dialer), WebSocket libraries that dial over net/http, and the UDP/QUIC
// stacks behind HTTP-3 and WebTransport (e.g. quic-go over a net.PacketConn).
// Use BindDialer for the stream path, BindListenConfig for the packet path, or
// Control to attach binding to anything else.
//
// picnic binds the device itself with the platform's interface-bind socket
// option — SO_BINDTODEVICE on Linux, IP_BOUND_IF/IPV6_BOUND_IF on macOS,
// IP_UNICAST_IF/IPV6_UNICAST_IF on Windows. These steer egress without binding
// the socket's address, so they compose with a later connect or bind. Only
// Linux's option requires elevated privilege (CAP_NET_RAW); the others do not.
//
// When the device option is unavailable (Linux without CAP_NET_RAW, or a
// platform with no such option), BindDialer falls back to binding a source
// address belonging to the interface — a dialer can do this because connect
// supplies the destination, but a ListenConfig cannot, since ListenPacket binds
// the socket itself. So that source fallback applies to the dialer path only.
//
// The destination's family (which selects the v4 vs v6 option) is taken from
// the dial's network string ("tcp4"/"tcp6") or, failing that, the destination
// IP, so callers never pass an IP version explicitly.
//
// Caveat: the source-address fallback is destination-blind. On an interface
// holding both a global unicast address (GUA) and a unique local address (ULA),
// picnic prefers the GUA, since a ULA source cannot reach a global destination;
// but no in-process source selection can be perfect without the route. Where
// deterministic egress matters, use the device-bind path (on Linux, grant
// CAP_NET_RAW) or pair the source bind with OS policy routing.
package picnic

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"syscall"
)

// Name is the name of a local network interface, e.g. "eth0", "en0", "Wi-Fi".
type Name string

// BindDialer configures d so that sockets it creates egress this interface,
// covering everything built on net.Dialer: raw TCP/UDP/unix dials, net/http
// (http.Transport.DialContext), crypto/tls (tls.Dialer.NetDialer), and WebSocket
// libraries that dial over an *http.Client. It validates that the interface
// exists and sets d.ControlContext (so it is honored even if a plain d.Control
// is also present).
func (n Name) BindDialer(d *net.Dialer) error {
	ctl, err := n.control(true) // dialer may fall back to a source-address bind
	if err != nil {
		return err
	}
	d.ControlContext = func(_ context.Context, network, address string, c syscall.RawConn) error {
		return ctl(network, address, c)
	}
	return nil
}

// BindListenConfig configures lc so that sockets it creates egress this
// interface, covering the packet path: net.ListenConfig.ListenPacket (UDP) — and
// thus QUIC, HTTP-3, and WebTransport stacks (e.g. quic-go) that run over a
// net.PacketConn you supply — as well as net.ListenConfig.Listen for servers.
//
// It binds via the device socket option only; it does not use the dialer's
// source-address fallback, which would collide with ListenPacket's own bind. On
// a platform with no device option (and, for Linux, without CAP_NET_RAW) the
// socket is therefore not interface-bound.
func (n Name) BindListenConfig(lc *net.ListenConfig) error {
	ctl, err := n.control(false) // ListenPacket owns the bind; no source fallback
	if err != nil {
		return err
	}
	lc.Control = ctl
	return nil
}

// Control returns the interface-binding callback. Its signature matches both
// net.Dialer.Control and net.ListenConfig.Control, so it can be attached to any
// standard-library socket constructor. Like BindListenConfig it binds via the
// device socket option only, without the dialer's source-address fallback (which
// would collide with a ListenConfig's own bind) — use BindDialer for that. It
// errors immediately if the interface name is empty or does not currently exist.
func (n Name) Control() (func(network, address string, c syscall.RawConn) error, error) {
	return n.control(false)
}

// control returns the binding callback for this interface. When allowSourceBind
// is true (the dialer path) and the device option is unavailable, it falls back
// to binding a source address; the ListenConfig path passes false, because
// ListenPacket performs its own bind and a second bind would fail.
func (n Name) control(allowSourceBind bool) (func(network, address string, c syscall.RawConn) error, error) {
	if n == "" {
		return nil, errors.New("picnic: empty interface name")
	}
	ifc, err := net.InterfaceByName(string(n))
	if err != nil {
		return nil, fmt.Errorf("picnic: interface %q: %w", string(n), err)
	}
	name, index := ifc.Name, ifc.Index

	return func(network, address string, c syscall.RawConn) error {
		want6 := familyIsV6(network, address)
		var inner error
		if err := c.Control(func(fd uintptr) {
			// Prefer the device-bind socket option; it steers egress without
			// binding the socket's address, so it composes with connect/bind.
			if bindToDevice(fd, name, index, want6) == nil {
				return
			}
			// Device option unavailable. A dialer can still steer egress by
			// binding a source address; a ListenConfig cannot, so it is left to
			// bind its own address normally.
			if allowSourceBind {
				inner = n.bindSourceFamily(fd, want6)
			}
		}); err != nil {
			return err
		}
		return inner
	}, nil
}

// bindSourceFamily binds fd to a source address of this interface of the given
// family. It is the dialer-only fallback used when no device option applies.
func (n Name) bindSourceFamily(fd uintptr, want6 bool) error {
	sa, err := n.sourceSockaddr(want6)
	if err != nil {
		return err
	}
	return osBind(fd, sa)
}

// sourceSockaddr resolves a bindable source address of the requested family on
// this interface, applying the selectSource preference. It performs no syscalls
// beyond interface enumeration, so the whole fallback path is unit-testable
// without privilege; only osBind, its caller's actual bind, needs a live socket.
func (n Name) sourceSockaddr(want6 bool) (syscall.Sockaddr, error) {
	ifc, err := net.InterfaceByName(string(n))
	if err != nil {
		return nil, fmt.Errorf("picnic: interface %q: %w", string(n), err)
	}
	addrs, err := ifc.Addrs()
	if err != nil {
		return nil, fmt.Errorf("picnic: interface %q addrs: %w", string(n), err)
	}
	ip := selectSource(addrs, want6)
	if ip == nil {
		return nil, fmt.Errorf("picnic: no %s address on interface %q", familyName(want6), string(n))
	}
	return sockaddr(ip, ifc.Index)
}

// selectSource picks the best source address of the requested family from the
// interface's addresses, preferring a true global unicast address over a ULA
// over a link-local one. It is a pure function for testability.
func selectSource(addrs []net.Addr, want6 bool) net.IP {
	var global, ula, linkLocal, other net.IP
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipn.IP
		if (ip.To4() == nil) != want6 {
			continue // wrong family
		}
		switch {
		case ip.IsLinkLocalUnicast():
			if linkLocal == nil {
				linkLocal = ip
			}
		case isULA(ip):
			if ula == nil {
				ula = ip
			}
		case ip.IsGlobalUnicast():
			if global == nil {
				global = ip
			}
		default:
			// Loopback and anything else usable, as a last resort. Without this,
			// binding to the loopback interface (whose addresses are none of the
			// above) would fail.
			if other == nil {
				other = ip
			}
		}
	}
	for _, ip := range []net.IP{global, ula, linkLocal, other} {
		if ip != nil {
			return ip
		}
	}
	return nil
}

// familyIsV6 reports whether the connection is IPv6, from the network suffix
// ("tcp6"/"udp6" vs "tcp4"/"udp4") or, when unspecified, the destination IP.
func familyIsV6(network, address string) bool {
	switch {
	case strings.HasSuffix(network, "6"):
		return true
	case strings.HasSuffix(network, "4"):
		return false
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() == nil
}

// isULA reports whether ip is an IPv6 unique local address (fc00::/7).
func isULA(ip net.IP) bool {
	if ip.To4() != nil {
		return false
	}
	ip16 := ip.To16()
	return ip16 != nil && ip16[0]&0xfe == 0xfc
}

func familyName(want6 bool) string {
	if want6 {
		return "IPv6"
	}
	return "IPv4"
}

// sockaddr builds a syscall.Sockaddr (port 0) for ip, setting the IPv6 zone to
// the interface index for link-local addresses, which require it to bind.
func sockaddr(ip net.IP, zone int) (syscall.Sockaddr, error) {
	if ip4 := ip.To4(); ip4 != nil {
		var a [4]byte
		copy(a[:], ip4)
		return &syscall.SockaddrInet4{Addr: a}, nil
	}
	ip16 := ip.To16()
	if ip16 == nil {
		return nil, fmt.Errorf("picnic: invalid IP %v", ip)
	}
	var a [16]byte
	copy(a[:], ip16)
	sa := &syscall.SockaddrInet6{Addr: a}
	// Link-local addresses need the zone (interface index) to bind. Interface
	// indices are small non-negative ints; bound the conversion to satisfy the
	// overflow checker.
	if ip.IsLinkLocalUnicast() && zone > 0 && zone <= math.MaxUint32 {
		sa.ZoneId = uint32(zone)
	}
	return sa, nil
}
