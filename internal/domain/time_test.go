// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json"
	"testing"
)

func TestWireTimes(t *testing.T) {
	if Milliseconds(json.RawMessage(`"date"`)) != nil {
		t.Fatal("date text accepted as milliseconds")
	}
	for _, raw := range []string{`1234`, `"1234"`, `"2026-10-03T17:41:34.000+0000"`, `"2026-10-03 17:41:34"`} {
		if Instant(json.RawMessage(raw)) == nil {
			t.Fatal(raw)
		}
	}
	if Instant(json.RawMessage(`0`)) != nil {
		t.Fatal("zero date")
	}
}

func TestInvalidTimes(t *testing.T) {
	if Instant(json.RawMessage(`false`)) != nil || Instant(json.RawMessage(`"invalid"`)) != nil {
		t.Fatal("invalid date")
	}

}
