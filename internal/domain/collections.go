// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
)

// UniqueLast keeps the last row of each identifier in arrival order. Offset
// paging can repeat an entry after a concurrent update moves it to a later page.
func UniqueLast[T any, K comparable](values []T, id func(T) K) []T {
	last := make(map[K]int, len(values))
	for i, v := range values {
		last[id(v)] = i
	}
	out := make([]T, 0, len(last))
	for i, v := range values {
		if last[id(v)] == i {
			out = append(out, v)
		}
	}
	return out
}

// Marked decodes a row and its optional deletion marker from the same object.
type Marked[T any] struct {
	Value   T
	Deleted bool
}

// UnmarshalJSON tolerates malformed deletion flags just as optional wire fields do.
func (m *Marked[T]) UnmarshalJSON(b []byte) error {
	var value T
	if err := json.Unmarshal(b, &value); err != nil {
		return err
	}
	var row map[string]jsontext.Value
	if err := json.Unmarshal(b, &row); err != nil {
		return err
	}
	m.Value = value
	m.Deleted = false
	if f := Flag(row["isDeleted"]); f != nil {
		m.Deleted = *f
	}
	return nil
}
