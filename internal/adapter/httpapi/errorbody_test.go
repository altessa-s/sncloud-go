// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestDownloadErrorDoesNotUseFileCeiling(t *testing.T) {
	for _, status := range []int{403, 416, 429, 503} {
		for _, length := range []int64{-1, 300, BodyLimit + 1} {
			tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: make(http.Header), ContentLength: length, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", 300)))}, nil
			})})
			out, err := tr.Download(t.Context(), port.Request{URL: "https://synthetic.invalid/x", Limit: 4})
			if err != nil || out.Status != status || out.File != "" {
				t.Errorf("status %d length %d: %+v %v", status, length, out, err)
			}
		}
	}
}
func TestRelativeAndOpaqueURLRedaction(t *testing.T) {
	for _, raw := range []string{"/x?signature=secret", "//cdn.invalid/x?signature=secret", "x?signature=secret", "https:secret", "https://user:secret@cdn.invalid/x#secret"} {
		got := redact(raw, "url").(string)
		if strings.Contains(got, "secret") {
			t.Errorf("leaked %q", got)
		}
		if strings.Contains(safeURL(raw), "secret") {
			t.Errorf("unsafe URL %q", raw)
		}
	}
}

func TestHTTPErrorStatusSurvivesUnreadableBody(t *testing.T) {
	for _, status := range []int{401, 416, 429, 503} {
		for _, kind := range []string{"declared", "chunked", "truncated"} {
			tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) {
				var body io.Reader = io.LimitReader(zeroReader{}, BodyLimit+1)
				length := int64(-1)
				if kind == "declared" {
					length = BodyLimit + 1
				}
				if kind == "truncated" {
					body = errorReader{}
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), ContentLength: length, Body: io.NopCloser(body)}, nil
			})})
			out, err := tr.RoundTrip(t.Context(), port.Request{URL: "https://synthetic.invalid/x"})
			if err != nil || out.Status != status || len(out.Body) != 0 {
				t.Errorf("%d %s: %v", status, kind, err)
			}
		}
	}
}
