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

func TestUploadReplayAndProgress(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "replay", true: "removed source"}[remove], func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "synthetic.bin")
			if err := os.WriteFile(file, []byte("synthetic"), 0600); err != nil {
				t.Fatal(err)
			}
			var sent, total int64
			tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.ContentLength != 9 || r.Method != "PUT" {
					t.Fatalf("request: %s length %d", r.Method, r.ContentLength)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != "synthetic" {
					t.Fatal(string(body), err)
				}
				if remove {
					if err := os.Remove(file); err != nil {
						t.Fatal(err)
					}
				}
				replay, err := r.GetBody()
				if remove {
					if !errors.Is(err, os.ErrNotExist) {
						t.Fatal(err)
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					body, err = io.ReadAll(replay)
					if err != nil || string(body) != "synthetic" {
						t.Fatal(string(body), err)
					}
					if err := replay.Close(); err != nil {
						t.Fatal(err)
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"success":true}`))}, nil
			})})
			out, err := tr.Upload(t.Context(), port.Request{Method: "PUT", URL: "http://synthetic.invalid/object", File: file, Progress: func(n, size int64) { sent = n; total = size }})
			if err != nil || out.Status != 200 || string(out.Body) != `{"success":true}` || sent != 9 || total != 9 {
				t.Fatal(out, err, sent, total)
			}
		})
	}
}

func TestUploadFailurePropagation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "synthetic.bin")
	if err := os.WriteFile(file, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("synthetic transport failure")
	calls := 0
	tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, sentinel })})
	if _, err := tr.Upload(t.Context(), port.Request{URL: "http://synthetic.invalid", File: file + "-missing"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := tr.Upload(t.Context(), port.Request{URL: "://bad", File: file}); err == nil {
		t.Fatal("invalid URL accepted")
	}
	if calls != 0 {
		t.Fatal("invalid upload reached transport")
	}
	if _, err := tr.Upload(t.Context(), port.Request{Method: "PUT", URL: "http://synthetic.invalid", File: file}); !errors.Is(err, domain.ErrTransport) {
		t.Fatal(err)
	}
}
