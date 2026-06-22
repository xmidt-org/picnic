// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package picnic

import (
	"syscall"
	"unsafe"
)

// Windows interface-bind socket options, keyed by interface index. The syscall
// package does not export these, so the level and option numbers are defined
// here.
const (
	_IPPROTO_IP      = 0
	_IPPROTO_IPV6    = 41
	_IP_UNICAST_IF   = 31
	_IPV6_UNICAST_IF = 31
)

// bindToDevice binds the socket to the interface index via IP_UNICAST_IF /
// IPV6_UNICAST_IF. It needs no privilege.
func bindToDevice(fd uintptr, _ string, index int, want6 bool) error {
	if want6 {
		return setsockoptInt32(syscall.Handle(fd), _IPPROTO_IPV6, _IPV6_UNICAST_IF, int32(index))
	}
	// IP_UNICAST_IF takes the IPv4 index in network byte order. Windows is
	// little-endian, so byte-swap the value to give it a big-endian layout.
	return setsockoptInt32(syscall.Handle(fd), _IPPROTO_IP, _IP_UNICAST_IF, int32(htonl(uint32(index))))
}

func setsockoptInt32(h syscall.Handle, level, opt, v int32) error {
	//nolint:gosec // setsockopt requires a pointer to the option value
	return syscall.Setsockopt(h, level, opt, (*byte)(unsafe.Pointer(&v)), int32(unsafe.Sizeof(v)))
}

// htonl returns x with its bytes reversed, so that an int32 holding the result
// has a big-endian memory layout on a little-endian host (i.e. Windows).
func htonl(x uint32) uint32 {
	return x<<24 | (x&0xff00)<<8 | (x&0xff0000)>>8 | x>>24
}
