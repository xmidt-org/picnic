// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package picnic

import "syscall"

// osBind binds the socket fd to the given source address. On unix the raw
// descriptor is an int.
func osBind(fd uintptr, sa syscall.Sockaddr) error {
	return syscall.Bind(int(fd), sa)
}
