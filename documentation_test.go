// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPublicCommentsDescribeRetainedAPI(t *testing.T) {
	for file, removed := range map[string][]string{
		"types.go":  {"EquipmentNo()", "EquipmentHeader()", "SocketType()", "Channel(token"},
		"device.go": {"ListedIn"},
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range removed {
			if strings.Contains(string(b), name) {
				t.Errorf("%s advertises removed %s", file, name)
			}
		}
	}
}

func TestGoroutinePackagesCheckLeaks(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"clean", "leak"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestLeakCheckSubprocess$")
			cmd.Env = append(os.Environ(), "SNCLOUD_LEAK_PROBE="+mode)
			out, err := cmd.CombinedOutput()
			if mode == "clean" {
				if err != nil {
					t.Fatalf("clean process: %v\n%s", err, out)
				}
				return
			}
			exit, ok := errors.AsType[*exec.ExitError](err)
			if !ok || exit.ExitCode() != 1 || !bytes.Contains(out, []byte("PASS")) || !bytes.Contains(out, []byte("leaked goroutines")) {
				t.Fatalf("leak did not fail through TestMain: %v\n%s", err, out)
			}
		})
	}
}

func TestLeakCheckSubprocess(t *testing.T) {
	if os.Getenv("SNCLOUD_LEAK_PROBE") != "leak" {
		return
	}
	started := make(chan struct{})
	go func() {
		close(started)
		select {}
	}()
	<-started
}
