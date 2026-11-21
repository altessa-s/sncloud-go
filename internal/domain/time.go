// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json/v2"
	"time"
)

// Milliseconds reads an optional timestamp, ignoring other wire shapes.
func Milliseconds(b []byte) *time.Time {
	var n WireInt64
	if json.Unmarshal(b, &n) != nil {
		return nil
	}
	t := time.UnixMilli(int64(n))
	return &t
}

// Instant also accepts the Java server's date strings. Zero means no date.
func Instant(b []byte) *time.Time {
	if t := Milliseconds(b); t != nil {
		if t.UnixMilli() == 0 {
			return nil
		}
		return t
	}
	var s string
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999-0700", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// WireTime treats the server's zero timestamp as an absent time.
func WireTime(b []byte) *time.Time {
	t := Milliseconds(b)
	if t == nil || t.UnixMilli() == 0 {
		return nil
	}
	return t
}
