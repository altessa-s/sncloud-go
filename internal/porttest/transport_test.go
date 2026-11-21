// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package porttest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestScriptSnapshotsAndFileDefaults(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "input")
	if err := os.WriteFile(file, []byte("synthetic bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	tr := &Transport{
		Steps: []Step{
			OK(""),
			{Response: port.Response{Status: 200, Body: []byte("download")}},
			OK(`"id":"1"`),
			Refused("E0311"),
			{Run: func(_ context.Context, r port.Request) (port.Response, error) {
				return port.Response{Status: 200, Body: r.Body}, nil
			}},
			{Response: port.Response{Status: 200, Body: []byte("oversized")}},
			{Response: port.Response{Status: 200, Body: []byte("saved")}},
			{Response: port.Response{Status: 200, Body: []byte("duplicate")}},
		},
	}
	if _, err := tr.Upload(context.Background(), port.Request{File: filepath.Join(dir, "missing")}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing Upload = %v", err)
	}
	var progress int64
	_, err := tr.Upload(context.Background(), port.Request{
		File:     file,
		Header:   port.Header{"x": {"one"}},
		Progress: func(done, _ int64) { progress = done },
	})
	if err != nil || progress != int64(len("synthetic bytes")) {
		t.Fatal(err, progress)
	}
	snapshot := tr.Calls()
	snapshot[0].Header["x"][0] = "mutated"
	snapshot[0].Body[0] = 'X'
	if r := tr.Calls()[0]; r.Header.Get("x") != "one" || string(r.Body) != "synthetic bytes" {
		t.Fatal(r)
	}
	out, err := tr.Download(context.Background(), port.Request{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(out.File)
	b, err := os.ReadFile(out.File)
	if err != nil || string(b) != "download" {
		t.Fatal(string(b), err)
	}
	if resp, err := tr.RoundTrip(context.Background(), port.Request{}); err != nil || !bytes.Contains(resp.Body, []byte(`"id":"1"`)) {
		t.Fatalf("OK with fields = (%s, %v)", resp.Body, err)
	}
	if resp, err := tr.RoundTrip(context.Background(), port.Request{}); err != nil || !bytes.Contains(resp.Body, []byte(`"E0311"`)) {
		t.Fatalf("Refused = (%s, %v)", resp.Body, err)
	}
	if resp, err := tr.RoundTrip(context.Background(), port.Request{Body: []byte("echo")}); err != nil || string(resp.Body) != "echo" {
		t.Fatalf("Step.Run = (%s, %v)", resp.Body, err)
	}
	if _, err := tr.Download(context.Background(), port.Request{Limit: 4}); !errors.Is(err, domain.ErrOversizedResponse) {
		t.Fatalf("oversized Download = %v", err)
	}
	dest := filepath.Join(dir, "dest.bin")
	var dlDone int64
	if out, err := tr.Download(context.Background(), port.Request{
		Dest:     dest,
		Progress: func(done, _ int64) { dlDone = done },
	}); err != nil || out.File != dest || dlDone != 5 {
		t.Fatalf("Download to Dest = (%+v, %v, progress=%d)", out, err, dlDone)
	}
	if _, err := tr.Download(context.Background(), port.Request{Dest: dest}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("Download existing Dest = %v", err)
	}
	if _, err := tr.Download(context.Background(), port.Request{}); err == nil {
		t.Fatal("unscripted Download accepted")
	}
}

func TestUploadSections(t *testing.T) {
	file := filepath.Join(t.TempDir(), "synthetic.bin")
	if err := os.WriteFile(file, []byte("beforePAYLOADafter"), 0600); err != nil {
		t.Fatal(err)
	}
	tr := &Transport{Steps: []Step{OK("")}}
	req := port.Request{File: file, Section: &port.FileSection{Offset: 6, Length: 7}}
	if _, err := tr.Upload(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if got := string(tr.Calls()[0].Body); got != "PAYLOAD" {
		t.Fatalf("body=%q", got)
	}
	for _, section := range []port.FileSection{{Offset: -1, Length: 1}, {Length: -1}, {Offset: 99, Length: 1}, {Offset: 6, Length: 99}} {
		req.Section = &section
		want := io.ErrUnexpectedEOF
		if section.Offset < 0 || section.Length < 0 {
			want = os.ErrInvalid
		}
		if _, err := tr.Upload(t.Context(), req); !errors.Is(err, want) || errors.Is(err, domain.ErrMalformedResponse) {
			t.Fatalf("section=%+v: %v", section, err)
		}
	}
	if err := os.Truncate(file, 8); err != nil {
		t.Fatal(err)
	}
	req.Section = &port.FileSection{Offset: 6, Length: 7}
	if _, err := tr.Upload(t.Context(), req); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated source: %v", err)
	}
	if len(tr.Calls()) != 1 {
		t.Fatal("invalid section consumed a scripted request")
	}
}
