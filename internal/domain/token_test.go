// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestReadsTheExpiry(t *testing.T) {
	for _, raw := range []string{`4102444800`, `"4102444800"`, `4102444800000`, `"4102444800000"`} {
		token := "x." + base64.RawURLEncoding.EncodeToString([]byte(`{"exp":`+raw+`}`)) + ".x"
		e := TokenExpiry(token)
		if e == nil || e.Unix() != 4102444800 {
			t.Fatalf("%s: %v", raw, e)
		}
		if TokenUsable(token, e.Add(-5*time.Minute)) {
			t.Fatal("expiry margin ignored")
		}
	}
	for _, s := range []string{"opaque", "x.e30.x", "x.bad.x"} {
		if TokenExpiry(s) != nil {
			t.Fatal(s)
		}
	}
}

func TestInvalidExpiry(t *testing.T) {
	for _, payload := range []string{`not JSON`, `{"exp":"NaN"}`, `{"exp":-1}`, `{"exp":1e300}`} {
		token := "x." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".x"
		if TokenExpiry(token) != nil {
			t.Fatal(payload)
		}
	}
}
