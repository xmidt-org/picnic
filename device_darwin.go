// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package picnic

import "syscall"

// macOS interface-bind socket options, keyed by interface index. The syscall
// package does not export the option numbers, so they are defined here.
const (
	_IP_BOUND_IF   = 0x19 // 25
	_IPV6_BOUND_IF = 0x7d // 125
)

// bindToDevice binds the socket to the interface index via IP_BOUND_IF /
// IPV6_BOUND_IF. Unlike Linux's SO_BINDTODEVICE, this needs no privilege.
func bindToDevice(fd uintptr, _ string, index int, want6 bool) error {
	if want6 {
		return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, _IPV6_BOUND_IF, index)
	}
	return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, _IP_BOUND_IF, index)
}
