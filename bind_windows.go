// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package picnic

import "syscall"

// osBind binds the socket fd to the given source address. On Windows the raw
// descriptor is a syscall.Handle.
func osBind(fd uintptr, sa syscall.Sockaddr) error {
	return syscall.Bind(syscall.Handle(fd), sa)
}
