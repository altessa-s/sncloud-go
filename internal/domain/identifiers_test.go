// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json"
	"testing"
)

func TestEncodesIdentifiersAsStrings(t *testing.T) {
	for _, raw := range []string{`819922212654940161`, `"819922212654940161"`, `" 819922212654940161 "`} {
		var id FileID
		if err := json.Unmarshal([]byte(raw), &id); err != nil {
			t.Fatal(err)
		}
		// FileID marshals a decimal string, which cannot fail.
		b, _ := json.Marshal(id)
		if string(b) != `"819922212654940161"` {
			t.Fatal(string(b))
		}
	}
	for _, raw := range []string{`1.5`, `null`, `9223372036854775808`, `true`, `"x"`} {
		var id FileID
		if json.Unmarshal([]byte(raw), &id) == nil {
			t.Fatal(raw)
		}
	}
}
