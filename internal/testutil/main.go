// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package testutil shares test-process lifecycle checks.
package testutil

import (
	"fmt"
	"os"
	"testing"

	"github.com/altessa-s/go-atlas-core/runtime/leakcheck"
)

// Main runs tests, releases fixtures, and fails an otherwise passing process
// when a goroutine remains permanently blocked.
func Main(m *testing.M, cleanup func()) {
	code := m.Run()
	if cleanup != nil {
		cleanup()
	}
	if code == 0 {
		if err := leakcheck.Check(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			code = 1
		}
	}
	os.Exit(code)
}
