// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseExpressions(t *testing.T) {
	configuration, err := os.ReadFile("../../../.goreleaser.yml")
	if err != nil {
		t.Fatal(err)
	}
	groups, excludes, err := readRules(configuration)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ subject, want string }{
		{"feat(client): new", "Features"}, {"fix(httpapi)!: changed", "Bug Fixes"},
		{"perf(io/read): faster", "Performance"}, {"refactor(domain): simplify", "Refactoring"},
		{"docs(README): describe", "Documentation"}, {"build(deps): bump", "Build"},
		{"ci(checks)!: change", "Excluded"}, {"chore: tidy", "Excluded"}, {"test(client): cover", "Excluded"},
		{"prefix feat(client): invalid", "Other"}, {"feature: invalid", "Other"},
	} {
		if got := classify(tc.subject, groups, excludes); got != tc.want {
			t.Errorf("%q: got %s, want %s", tc.subject, got, tc.want)
		}
	}
}

func TestMergeSubjectsAccepted(t *testing.T) {
	cmd := exec.Command("go", "run", "./.github/scripts/changelog")
	cmd.Dir = "../../.."
	cmd.Stdin = strings.NewReader("Merge branch 'release/0.1.0'\nMerge tag 'v0.1.0' into develop\nMerge pull request #1 from example/fix\nMerge remote-tracking branch 'origin/develop'\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("merge subjects rejected: %v\n%s", err, out)
	}
}

func TestUnintendedExclusionRejected(t *testing.T) {
	dir := t.TempDir()
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	config := []byte("exclude:\n  - '^fix'\n")
	for name, b := range map[string][]byte{"main.go": source, ".goreleaser.yml": config} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "run", "main.go")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("fix(client): important change\n")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "Excluded | fix(client)") {
		t.Fatalf("unintended exclusion accepted: %v %s", err, out)
	}
}
