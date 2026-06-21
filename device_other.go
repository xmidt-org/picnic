// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package picnic

import "errors"

// errNoDeviceBind signals that this platform has no SO_BINDTODEVICE equivalent,
// so callers fall back to binding a source address from the interface.
var errNoDeviceBind = errors.New("picnic: device binding not supported on this platform")

func bindToDevice(uintptr, string) error {
	return errNoDeviceBind
}
