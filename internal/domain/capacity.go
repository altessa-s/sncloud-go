// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import "encoding/json/v2"

// UnmarshalJSON requires both quota fields; an omitted or null count is not a zero quota.
func (c *Capacity) UnmarshalJSON(b []byte) error {
	var w struct {
		Used  *WireInt64 `json:"usedCapacity"`
		Total *WireInt64 `json:"totalCapacity"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	if w.Used == nil || w.Total == nil {
		return &MalformedResponseError{What: "capacity missing used or total bytes"}
	}
	*c = Capacity{Used: int64(*w.Used), Total: int64(*w.Total)}
	return nil
}
