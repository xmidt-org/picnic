// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package picnic

import "syscall"

// bindToDevice binds the socket to the named interface via SO_BINDTODEVICE. The
// index and family arguments are unused on Linux. This requires CAP_NET_RAW; on
// failure the dialer path falls back to a source-address bind.
func bindToDevice(fd uintptr, name string, _ int, _ bool) error {
	return syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, name)
}
