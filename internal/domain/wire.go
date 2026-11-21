// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strconv"
	"strings"
)

// WireInt64 accepts integer numbers and strings without a float conversion.
type WireInt64 int64

// UnmarshalJSON preserves all 64 bits of a snowflake.
func (v *WireInt64) UnmarshalJSON(b []byte) error {
	text := string(b)
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &text); err != nil {
			return err
		}
		text = strings.TrimSpace(text)
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("wire integer: %w", err)
	}
	*v = WireInt64(n)
	return nil
}

// WireFlag accepts JSON booleans and the service's Y/N strings.
type WireFlag bool

// UnmarshalJSON follows Jackson's observed flag spellings.
func (v *WireFlag) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("true")) {
		*v = true
		return nil
	}
	if bytes.Equal(b, []byte("false")) {
		*v = false
		return nil
	}
	var s string
	if bytes.Equal(b, []byte("null")) {
		return fmt.Errorf("null flag")
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*v = WireFlag(strings.EqualFold(s, "Y"))
	return nil
}

// WireEcho preserves the challenge timestamp's number-or-string spelling.
type WireEcho struct{ raw jsontext.Value }

// UnmarshalJSON accepts only a string or a signed 64-bit integer.
func (v *WireEcho) UnmarshalJSON(b []byte) error {
	var n int64
	var s string
	if len(b) == 0 || string(b) == "null" {
		return fmt.Errorf("missing timestamp")
	}
	if b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
	} else if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	v.raw = append(v.raw[:0], b...)
	return nil
}

// Text reads a nonempty optional string, ignoring other shapes.
func Text(b []byte) *string {
	var s string
	if json.Unmarshal(b, &s) != nil || s == "" {
		return nil
	}
	return &s
}

// Integer reads an optional integer without sacrificing identifier precision.
func Integer(b []byte) *int64 {
	var n WireInt64
	if json.Unmarshal(b, &n) != nil {
		return nil
	}
	v := int64(n)
	return &v
}

// Flag reads an optional wire flag; a malformed field is absent.
func Flag(b []byte) *bool {
	var f WireFlag
	if json.Unmarshal(b, &f) != nil {
		return nil
	}
	v := bool(f)
	return &v
}

// MarshalJSON echoes the original scalar.
func (v WireEcho) MarshalJSON() ([]byte, error) {
	if len(v.raw) == 0 {
		return nil, fmt.Errorf("missing timestamp")
	}
	return v.raw, nil
}
