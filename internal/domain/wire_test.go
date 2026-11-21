// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json"
	"testing"
)

func TestEchoesTheTimestamp(t *testing.T) {
	for _, raw := range []string{`1773336349503`, `"1773336349503"`} {
		var e WireEcho
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(e)
		if err != nil || string(b) != raw {
			t.Fatalf("%s %v", b, err)
		}
	}
	for _, raw := range []string{`null`, `true`, `1.5`} {
		var e WireEcho
		if json.Unmarshal([]byte(raw), &e) == nil {
			t.Fatal(raw)
		}
	}
}
func TestWireScalars(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`true`, true},
		{`false`, false},
		{`"Y"`, true},
		{`"y"`, true},
		{`"N"`, false},
		{`"other"`, false},
	} {
		var f WireFlag
		if err := json.Unmarshal([]byte(tc.raw), &f); err != nil || bool(f) != tc.want {
			t.Fatalf("%s: %v %v", tc.raw, f, err)
		}
	}

}
func TestInvalidWireValues(t *testing.T) {
	for _, raw := range []string{`null`, `123`, `{}`} {
		var f WireFlag
		if json.Unmarshal([]byte(raw), &f) == nil {
			t.Fatal(raw)
		}
	}
	var echo WireEcho
	if _, err := json.Marshal(echo); err == nil {
		t.Fatal("unset echo serialized")
	}

}

func TestOptionalWireFieldsAndLastOccurrence(t *testing.T) {
	if Text(json.RawMessage(`""`)) != nil ||
		Text(json.RawMessage(`123`)) != nil ||
		Integer(json.RawMessage(`1.5`)) != nil ||
		Flag(json.RawMessage(`null`)) != nil ||
		WireTime(json.RawMessage(`0`)) != nil {
		t.Fatal("invalid optional field accepted")
	}
	if got := Text(json.RawMessage(`"name"`)); got == nil || *got != "name" {
		t.Fatal(got)
	}
	if got := Integer(json.RawMessage(`"819922212654940161"`)); got == nil || *got != 819922212654940161 {
		t.Fatal(got)
	}
	if WireTime(json.RawMessage(`1234`)) == nil {
		t.Fatal("valid time lost")
	}

}
