// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package picnic

import "syscall"

// bindToDevice binds the socket to the named interface via SO_BINDTODEVICE.
// This requires CAP_NET_RAW; on failure the caller falls back to a source bind.
func bindToDevice(fd uintptr, name string) error {
	return syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, name)
}
