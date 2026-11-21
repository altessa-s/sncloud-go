// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestUploadSectionReplay(t *testing.T) {
	file := filepath.Join(t.TempDir(), "synthetic.bin")
	if err := os.WriteFile(file, []byte("beforePAYLOADafter"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.ContentLength != 7 {
			t.Fatalf("length=%d", r.ContentLength)
		}
		first, err := io.ReadAll(r.Body)
		if err != nil || string(first) != "PAYLOAD" {
			t.Fatalf("body=%q, %v", first, err)
		}
		for range 2 {
			replay, err := r.GetBody()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(replay)
			if closeErr := replay.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if err != nil || string(b) != "PAYLOAD" {
				t.Fatalf("replay=%q, %v", b, err)
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	var sent, total int64
	req := port.Request{Method: "PUT", URL: "http://synthetic.invalid/object", File: file, Section: &port.FileSection{Offset: 6, Length: 7}, Progress: func(n, size int64) { sent, total = n, size }}
	if _, err := tr.Upload(t.Context(), req); err != nil {
		t.Fatal(err)
	}
	if sent != 7 || total != 7 {
		t.Fatalf("progress=%d/%d", sent, total)
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
	if calls != 1 {
		t.Fatalf("invalid section reached transport: calls=%d", calls)
	}
}
