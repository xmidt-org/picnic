// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package picnic

import (
	"context"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestFamilyIsV6(t *testing.T) {
	tests := []struct {
		network, address string
		want             bool
	}{
		{"tcp4", "1.2.3.4:80", false},
		{"tcp6", "[::1]:80", true},
		{"udp4", "1.2.3.4:80", false},
		{"udp6", "[fe80::1]:80", true},
		{"tcp", "1.2.3.4:80", false},      // derived from address
		{"tcp", "[2001:db8::1]:80", true}, // derived from address
		{"tcp", "8.8.8.8:53", false},
		{"tcp", "1.2.3.4", false}, // no port: falls back to the whole address
		{"udp", "garbage", false}, // unparseable: defaults to IPv4
	}
	for _, tt := range tests {
		if got := familyIsV6(tt.network, tt.address); got != tt.want {
			t.Errorf("familyIsV6(%q, %q) = %v, want %v", tt.network, tt.address, got, tt.want)
		}
	}
}

func TestIsULA(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"fd7a:115c:a1e0::1", true}, // fd00::/8 ULA
		{"fc00::1", true},           // fc00::/8 ULA
		{"2001:db8::1", false},      // GUA
		{"fe80::1", false},          // link-local, not ULA
		{"::1", false},              // loopback
		{"192.168.1.1", false},      // IPv4
		{"10.0.0.1", false},         // IPv4 private, not a ULA
	}
	for _, tt := range tests {
		if got := isULA(net.ParseIP(tt.ip)); got != tt.want {
			t.Errorf("isULA(%q) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestSelectSource(t *testing.T) {
	ipnet := func(s string) *net.IPNet {
		ip, n, err := net.ParseCIDR(s)
		if err != nil {
			t.Fatalf("bad CIDR %q: %v", s, err)
		}
		n.IP = ip
		return n
	}

	tests := []struct {
		name  string
		addrs []net.Addr
		want6 bool
		want  string // "" means a nil result is expected
	}{
		{
			name:  "prefers GUA over ULA and link-local",
			addrs: []net.Addr{ipnet("fe80::1/64"), ipnet("fd7a:115c:a1e0::5/64"), ipnet("2001:db8::5/64")},
			want6: true,
			want:  "2001:db8::5",
		},
		{
			name:  "prefers ULA over link-local when no GUA",
			addrs: []net.Addr{ipnet("fe80::1/64"), ipnet("fd7a:115c:a1e0::5/64")},
			want6: true,
			want:  "fd7a:115c:a1e0::5",
		},
		{
			name:  "ignores the wrong family",
			addrs: []net.Addr{ipnet("2001:db8::5/64"), ipnet("192.168.1.10/24")},
			want6: false,
			want:  "192.168.1.10",
		},
		{
			name:  "no address of the requested family yields nil",
			addrs: []net.Addr{ipnet("192.168.1.10/24")},
			want6: true,
			want:  "",
		},
		{
			name:  "loopback used as a last resort",
			addrs: []net.Addr{ipnet("127.0.0.1/8")},
			want6: false,
			want:  "127.0.0.1",
		},
		{
			name:  "non-*net.IPNet addresses are skipped",
			addrs: []net.Addr{&net.IPAddr{IP: net.ParseIP("10.0.0.1")}, ipnet("192.168.5.5/24")},
			want6: false,
			want:  "192.168.5.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectSource(tt.addrs, tt.want6)
			switch {
			case tt.want == "":
				if got != nil {
					t.Errorf("selectSource() = %v, want nil", got)
				}
			case got == nil || got.String() != tt.want:
				t.Errorf("selectSource() = %v, want %s", got, tt.want)
			}
		})
	}
}

func TestFamilyName(t *testing.T) {
	tests := []struct {
		want6 bool
		want  string
	}{
		{want6: true, want: "IPv6"},
		{want6: false, want: "IPv4"},
	}
	for _, tt := range tests {
		if got := familyName(tt.want6); got != tt.want {
			t.Errorf("familyName(%v) = %q, want %q", tt.want6, got, tt.want)
		}
	}
}

func TestSockaddr(t *testing.T) {
	tests := []struct {
		name     string
		ip       net.IP
		zone     int
		wantErr  bool
		wantZone uint32 // expected ZoneId for IPv6 results
	}{
		{name: "IPv4", ip: net.ParseIP("192.168.1.10"), zone: 0},
		{name: "IPv6 global ignores zone", ip: net.ParseIP("2001:db8::5"), zone: 9, wantZone: 0},
		{name: "IPv6 link-local sets zone", ip: net.ParseIP("fe80::1"), zone: 7, wantZone: 7},
		{name: "invalid IP errors", ip: net.IP{1, 2, 3}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sa, err := sockaddr(tt.ip, tt.zone)
			if tt.wantErr {
				if err == nil {
					t.Fatal("got nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if ip4 := tt.ip.To4(); ip4 != nil {
				s4, ok := sa.(*syscall.SockaddrInet4)
				if !ok {
					t.Fatalf("got %T, want *syscall.SockaddrInet4", sa)
				}
				var want [4]byte
				copy(want[:], ip4)
				if s4.Addr != want || s4.Port != 0 {
					t.Errorf("SockaddrInet4 = %+v, want Addr %v port 0", s4, want)
				}
				return
			}

			s6, ok := sa.(*syscall.SockaddrInet6)
			if !ok {
				t.Fatalf("got %T, want *syscall.SockaddrInet6", sa)
			}
			var want [16]byte
			copy(want[:], tt.ip.To16())
			if s6.Addr != want {
				t.Errorf("SockaddrInet6 Addr = %v, want %v", s6.Addr, want)
			}
			if s6.ZoneId != tt.wantZone {
				t.Errorf("SockaddrInet6 ZoneId = %d, want %d", s6.ZoneId, tt.wantZone)
			}
		})
	}
}

// TestSourceSockaddr covers the source-address resolution (everything on the
// fallback path except the final osBind syscall) without requiring privilege.
func TestSourceSockaddr(t *testing.T) {
	lo := loopbackInterface(t)

	tests := []struct {
		name    string
		iface   Name
		want6   bool
		wantErr bool
	}{
		{name: "loopback IPv4", iface: Name(lo), want6: false},
		{name: "unknown interface", iface: "definitely-not-a-real-iface-0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sa, err := tt.iface.sourceSockaddr(tt.want6)
			if tt.wantErr {
				if err == nil {
					t.Fatal("got nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			s4, ok := sa.(*syscall.SockaddrInet4)
			if !ok {
				t.Fatalf("got %T, want *syscall.SockaddrInet4", sa)
			}
			if !net.IP(s4.Addr[:]).IsLoopback() {
				t.Errorf("got %v, want a loopback address", net.IP(s4.Addr[:]))
			}
		})
	}
}

func TestBindErrors(t *testing.T) {
	tests := []struct {
		name string
		bind func() error
	}{
		{
			name: "BindDialer unknown interface",
			bind: func() error { var d net.Dialer; return Name("definitely-not-a-real-iface-0").BindDialer(&d) },
		},
		{
			name: "BindDialer empty name",
			bind: func() error { var d net.Dialer; return Name("").BindDialer(&d) },
		},
		{
			name: "BindListenConfig unknown interface",
			bind: func() error {
				var lc net.ListenConfig
				return Name("definitely-not-a-real-iface-0").BindListenConfig(&lc)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.bind(); err == nil {
				t.Error("got nil error, want error")
			}
		})
	}
}

// TestBindDialer_LoopbackDial exercises the full path on every OS: it finds the
// loopback interface by flag (portable across "lo"/"lo0"/Windows), binds a
// dialer to it, and dials a loopback listener. On Linux this hits SO_BINDTODEVICE
// (or the source-bind fallback without CAP_NET_RAW); elsewhere the source bind.
func TestBindDialer_LoopbackDial(t *testing.T) {
	lo := loopbackInterface(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err == nil {
			c.Close()
		}
	}()

	var d net.Dialer
	if err := Name(lo).BindDialer(&d); err != nil {
		t.Fatalf("BindDialer(%q): %v", lo, err)
	}

	conn, err := d.DialContext(context.Background(), "tcp4", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial through %q: %v", lo, err)
	}
	conn.Close()
}

// TestBindListenConfig_LoopbackPacket exercises the packet (UDP/QUIC) path on
// every OS: it binds a ListenConfig to the loopback interface, opens a UDP
// socket through it, and round-trips a datagram on loopback.
func TestBindListenConfig_LoopbackPacket(t *testing.T) {
	lo := loopbackInterface(t)
	if ifc, err := net.InterfaceByName(lo); err == nil {
		// The device-bind path keys off the index; log it for diagnosis.
		t.Logf("loopback interface %q index=%d flags=%s", lo, ifc.Index, ifc.Flags)
	}

	rx, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen rx: %v", err)
	}
	defer rx.Close()

	var lc net.ListenConfig
	if err := Name(lo).BindListenConfig(&lc); err != nil {
		t.Fatalf("BindListenConfig(%q): %v", lo, err)
	}

	tx, err := lc.ListenPacket(context.Background(), "udp4", "127.0.0.1:0")
	if err != nil {
		// A "bind: invalid argument" here means the listen path attempted a
		// source-address bind, which collides with ListenPacket's own bind;
		// BindListenConfig must use the device option only.
		t.Fatalf("bound ListenPacket through %q: %v", lo, err)
	}
	defer tx.Close()

	if _, err := tx.WriteTo([]byte("ping"), rx.LocalAddr()); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := rx.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	buf := make([]byte, 8)
	n, _, err := rx.ReadFrom(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := string(buf[:n]); got != "ping" {
		t.Fatalf("got %q, want \"ping\"", got)
	}
}

func loopbackInterface(t *testing.T) string {
	t.Helper()
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces: %v", err)
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 && ifc.Flags&net.FlagUp != 0 {
			return ifc.Name
		}
	}
	t.Skip("no usable loopback interface found")
	return ""
}
