// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json/v2"
	"strconv"
)

// FileID is a snowflake; zero is the root directory.
type FileID int64

// RootFileID identifies the top-level directory.
const RootFileID FileID = 0

// String renders the exact decimal identifier.
func (v FileID) String() string { return strconv.FormatInt(int64(v), 10) }

// MarshalJSON always writes identifiers as strings.
func (v FileID) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }

// UnmarshalJSON accepts either wire spelling.
func (v *FileID) UnmarshalJSON(b []byte) error {
	var n WireInt64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*v = FileID(n)
	return nil
}
