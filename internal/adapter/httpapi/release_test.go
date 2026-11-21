// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestMalformedURLsAreRedacted(t *testing.T) {
	for _, body := range []string{
		`{"url":"https://user:secret@storage.invalid/%zz?credential=secret"}`,
		`{"downloadUrl":"/%zz?credential=secret"}`,
		`{"value":"https://user:secret@storage.invalid/%zz?credential=secret"}`,
	} {
		got := bodyText([]byte(body), "application/json")
		if strings.Contains(got, "secret") || !strings.Contains(got, "<invalid url>") {
			t.Errorf("unsafe log: %s", got)
		}
	}
}
func TestMaximumDownloadCeilingIsRejected(t *testing.T) {
	tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: -1, Body: io.NopCloser(strings.NewReader("nonempty"))}, nil
	})})
	out, err := tr.Download(context.Background(), port.Request{URL: "https://synthetic.invalid", Limit: math.MaxInt64})
	if out.File != "" {
		os.Remove(out.File)
	}
	if !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatalf("download accepted unsupported ceiling: %v", err)
	}
}

func TestMalformedURLsInHeadersAndErrorBodies(t *testing.T) {
	for _, target := range []string{"https://user:hidden@storage.invalid/%zz?key=hidden", "/%zz?key=hidden", "file%zz?key=hidden"} {
		var logs bytes.Buffer
		tr := newTestTransport(t, Config{WireLevel: WireBodies, Logger: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})), RoundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": {"application/json"}, "Location": {target}, "Content-Location": {target}, "Referer": {target}, "X-Storage": {target}}, Body: io.NopCloser(strings.NewReader(`{"detail":"` + target + `"}`))}, nil
		})})
		out, err := tr.RoundTrip(t.Context(), port.Request{URL: "https://synthetic.invalid/file"})
		if err != nil || out.Status != 400 || strings.Contains(logs.String(), "hidden") {
			t.Errorf("unsafe error: %v logs=%s", err, logs.String())
		}
		if !strings.Contains(logs.String(), "<invalid url>") {
			t.Error("missing invalid URL placeholder")
		}
	}
}
