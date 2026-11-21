// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"os"
	"strings"
	"testing"
)

// Inspect the live harness without connecting to an account or deleting data.
func TestLiveAccountWideOperationsRequireDestructiveOptIn(t *testing.T) {
	b, err := os.ReadFile("public_api_live_test.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, call string }{
		{"Files/EmptyRecycleBin", "f.EmptyRecycleBin(ctx)"},
		{"Device/WithSyncLock", "d.WithSyncLock(ctx"},
		{"Client/WrongPassword", "bad.LogIn(ctx"},
	} {
		_, sub, ok := strings.Cut(string(b), `run("`+tc.name+`", func(t *testing.T) {`)
		if !ok {
			t.Fatalf("%s subtest missing", tc.name)
		}
		before, _, ok := strings.Cut(sub, tc.call)
		if !ok || !strings.Contains(before, `os.Getenv("SNCLOUD_LIVE_DESTRUCTIVE") != "1"`) || !strings.Contains(before, "disposable account only") || !strings.Contains(before, "t.Skip(") {
			t.Errorf("%s lacks disposable-account opt-in", tc.name)
		}
	}
	if strings.Contains(string(b), "strings.HasPrefix(v.Name, name)") {
		t.Fatal("cleanup matches names instead of run-created IDs")
	}
}
