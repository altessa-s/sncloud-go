// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"testing"
)

func TestPinsTheSecretNames(t *testing.T) {
	if AccessToken != "accessToken" || Account != "account" || ClientID != "clientID" {
		t.Fatal("store keys changed")
	}
}
func TestPinsTheLogCategory(t *testing.T) {
	if LogCategory != "supernote" {
		t.Fatal(LogCategory)
	}
}

func TestDeviceIdentitySpellings(t *testing.T) {
	d := NewDeviceIdentity("LINUX", "ABC-DEF")
	if EquipmentNo(d) != "LINUX-abc-def" || EquipmentHeader(d) != "LINUX_abc-def" || SocketType(d) != "LINUXabc-def" || Channel(d, "token", "123") != "token_LINUXabc-def_123" {
		t.Fatalf("identity spellings: %+v", d)
	}
	if d := NewDeviceIdentity("", "ABC"); d.Platform == "" || d.ClientID != "abc" {
		t.Fatalf("default identity: %+v", d)
	}
}
