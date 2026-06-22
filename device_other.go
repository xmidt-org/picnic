// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build !linux && !darwin && !windows

package picnic

import "errors"

// errNoDeviceBind signals that this platform has no per-socket interface-bind
// option, so the dialer path falls back to a source-address bind.
var errNoDeviceBind = errors.New("picnic: device binding not supported on this platform")

func bindToDevice(uintptr, string, int, bool) error {
	return errNoDeviceBind
}
