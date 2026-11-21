// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"runtime"
	"strings"
)

// DeviceIdentity names this installation across logins.
type DeviceIdentity struct{ Platform, ClientID string }

// NewDeviceIdentity normalizes the stored UUID.
func NewDeviceIdentity(platform, id string) DeviceIdentity {
	if platform == "" {
		platform = strings.ToUpper(runtime.GOOS)
		if runtime.GOOS == "darwin" {
			platform = "MACOS"
		}
	}
	return DeviceIdentity{Platform: platform, ClientID: strings.ToLower(id)}
}

// EquipmentNo is the body spelling.
func EquipmentNo(d DeviceIdentity) string { return d.Platform + "-" + d.ClientID }

// EquipmentHeader is the header spelling.
func EquipmentHeader(d DeviceIdentity) string { return d.Platform + "_" + d.ClientID }

// SocketType is shared by the socket handshake and channel header.
func SocketType(d DeviceIdentity) string { return d.Platform + d.ClientID }

// Channel binds a request to its token and process start.
func Channel(d DeviceIdentity, token, started string) string {
	return token + "_" + SocketType(d) + "_" + started
}
