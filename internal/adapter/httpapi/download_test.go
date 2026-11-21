// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
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

func TestRefusesADeclaredLength(t *testing.T) {
	tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			Header:        make(http.Header),
			ContentLength: 2000,
			Body:          io.NopCloser(strings.NewReader("")),
		}, nil
	})})
	_, err := tr.Download(context.Background(), port.Request{URL: "https://synthetic.invalid", Limit: 1024})
	var over *domain.OversizedResponseError
	if !errors.As(err, &over) || over.Bytes != 2000 {
		t.Fatal(err)
	}
}
func TestStopsAnUndeclaredOverrun(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "partial")
	var read int
	tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			Header:        make(http.Header),
			ContentLength: -1,
			Body:          io.NopCloser(&countReader{n: &read}),
		}, nil
	})})
	_, err := tr.Download(context.Background(), port.Request{URL: "https://synthetic.invalid", Dest: dest, Limit: 1024})
	if !errors.Is(err, domain.ErrOversizedResponse) || read != 1025 {
		t.Fatal(err, read)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("partial file survived", err)
	}
}

func TestDownloadReadFailureRemovesPartialFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "partial")
	tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    200,
			Header:        make(http.Header),
			ContentLength: -1,
			Body:          io.NopCloser(errorReader{}),
		}, nil
	})})
	if _, err := tr.Download(context.Background(), port.Request{URL: "https://synthetic.invalid", Dest: dest}); !errors.Is(err, domain.ErrTransport) {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("partial file survived")
	}
}
func TestCeilingRule(t *testing.T) {
	for _, tc := range []struct {
		written, expected int64
		over              bool
	}{
		{50, -1, false},
		{100, 100, false},
		{10, 101, true},
		{101, -1, true},
	} {
		tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    200,
				Header:        make(http.Header),
				ContentLength: tc.expected,
				Body:          io.NopCloser(io.LimitReader(zeroReader{}, tc.written)),
			}, nil
		})})
		out, err := tr.Download(context.Background(), port.Request{URL: "https://synthetic.invalid", Limit: 100})
		if out.File != "" {
			if err := os.Remove(out.File); err != nil {
				t.Fatal(err)
			}
		}
		if errors.Is(err, domain.ErrOversizedResponse) != tc.over || !tc.over && err != nil {
			t.Fatal(tc, err)
		}
	}
}
