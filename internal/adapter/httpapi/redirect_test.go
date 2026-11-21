// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"net/http"
	"testing"
)

func TestStripsCredentialsOffOrigin(t *testing.T) {
	tr := newTestTransport(t, Config{})
	for _, tc := range []struct {
		from, to, method string
		refuse, strip    bool
	}{
		{"https://cloud.example/x", "https://storage.example/x", "GET", false, true},
		{"https://STORAGE.example:443/x", "https://storage.example/y", "GET", false, false},
		{"https://cloud.example/x", "https://cloud.example/y", "POST", false, false},
		{"https://cloud.example/x", "https://storage.example/y", "POST", true, false},
		{"https://cloud.example/x", "http://cloud.example/x", "GET", true, false},
	} {
		// These literal test URLs and methods are valid; no I/O happens here.
		previous, _ := http.NewRequest("GET", tc.from, nil)
		// These literal test URLs and methods are valid; no I/O happens here.
		next, _ := http.NewRequest(tc.method, tc.to, nil)
		next.Header.Set("x-access-token", "synthetic")
		err := tr.redirect(next, []*http.Request{previous})
		if (err != nil) != tc.refuse {
			t.Fatal(tc, err)
		}
		if !tc.refuse && (next.Header.Get("x-access-token") == "") != tc.strip {
			t.Fatal(tc, next.Header)
		}
	}
}
