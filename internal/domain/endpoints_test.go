// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"testing"
)

func TestKnowsWhoWantsXSRF(t *testing.T) {
	e := Official()
	if e.App("x") != "https://viewer.supernote.com/api/x" || e.Web("x") != "https://cloud.supernote.com/api/x" || !e.WebUsesXSRF {
		t.Fatal(e)
	}
	p := PrivateCloud("http://nas.example:8080", false)
	if p.AppHost != p.WebHost || p.WebUsesXSRF || p.Validate() != nil {
		t.Fatal(p)
	}
}
func TestGuardsTheHostOverride(t *testing.T) {
	for _, tc := range []struct {
		host   string
		accept bool
	}{
		{"https://fake.example", true},
		{"http://127.0.0.1:8080", true},
		{"http://[::1]:8080", true},
		{"http://evil.example", false},
		{"garbage", false},
		{"https://user:password@fake.example", false},
	} {
		got := EndpointsFromEnvironment(map[string]string{"SNCLOUD_HOST": tc.host})
		if (got.AppHost == tc.host) != tc.accept {
			t.Fatal(tc, got)
		}
	}
	if EndpointsFromEnvironment(map[string]string{"SNCLOUD_XSRF": "off"}).WebUsesXSRF {
		t.Fatal("xsrf override")
	}
}
