// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json"
	"testing"
)

func TestUniqueLastAndMarked(t *testing.T) {
	got := UniqueLast([]int{1, 2, 1, 3, 2}, func(n int) int { return n })
	if len(got) != 3 || got[0] != 1 || got[1] != 3 || got[2] != 2 {
		t.Fatal(got)
	}
	var row Marked[struct {
		Name string `json:"name"`
	}]
	if err := json.Unmarshal([]byte(`{"name":"synthetic","isDeleted":"Y"}`), &row); err != nil || !row.Deleted || row.Value.Name != "synthetic" {
		t.Fatal(row, err)
	}
	if err := json.Unmarshal([]byte(`{"name":"synthetic","isDeleted":123}`), &row); err != nil || row.Deleted {
		t.Fatal(row, err)
	}
}
