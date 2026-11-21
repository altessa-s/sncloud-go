// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"fmt"
	"strings"
	"testing"
)

func TestCredentialPinBindsOnlyTheFirstToken(t *testing.T) {
	pin := NewCredentialPin(7)
	if !pin.Current(7) || pin.Current(8) {
		t.Fatal("generation check failed")
	}
	if !pin.Accept("synthetic-a") || !pin.Accept("synthetic-a") {
		t.Fatal("original token rejected")
	}
	if pin.Accept("synthetic-b") {
		t.Fatal("replacement token accepted")
	}
	if !pin.Accept("synthetic-a") {
		t.Fatal("replacement changed the pin")
	}
	if strings.Contains(fmt.Sprintf("%+v %#v", pin, pin), "synthetic") {
		t.Fatal("pin retains token text")
	}
}
