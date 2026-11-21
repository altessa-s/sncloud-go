// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"testing"
)

func TestDigestsThePassword(t *testing.T) {
	if got := PasswordDigest("correct horse", "tOiJKwhW"); got != "8397cb69be351af3713553e8347624a612dd29eec0b44e5341fb46555ea23668" {
		t.Fatal(got)
	}
}
