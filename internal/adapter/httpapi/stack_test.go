// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestAtlasClientsHaveIndependentPools(t *testing.T) {
	tr := New(Config{})
	defer tr.CloseIdleConnections()
	if tr.memBase == tr.upBase || tr.memBase == tr.dlBase || tr.upBase == tr.dlBase {
		t.Fatal("clients share a pool")
	}
	for _, base := range []*http.Transport{tr.memBase, tr.upBase, tr.dlBase} {
		if base.TLSClientConfig.MinVersion < tls.VersionTLS12 || base.MaxIdleConns == 0 {
			t.Fatal("pooled TLS defaults missing")
		}
	}
	for _, c := range []*http.Client{tr.client, tr.upload, tr.download} {
		if c.Timeout != 0 || c.Jar != nil {
			t.Fatal("unexpected global deadline or cookie jar")
		}
	}
}

func TestAtlasDoesNotRetryAndBreaksPerHost(t *testing.T) {
	calls := map[string]int{}
	tr := New(Config{RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls[r.URL.Host]++
		if r.URL.Host == "dead.invalid" {
			return nil, errors.New("synthetic failure")
		}
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})})
	defer tr.CloseIdleConnections()
	for range 3 {
		before := calls["dead.invalid"]
		if _, err := tr.RoundTrip(context.Background(), port.Request{URL: "https://dead.invalid"}); err == nil {
			t.Fatal("failure lost")
		}
		if calls["dead.invalid"] != before+1 {
			t.Fatal("transport retried")
		}
	}
	if _, err := tr.RoundTrip(context.Background(), port.Request{URL: "https://dead.invalid"}); err == nil {
		t.Fatal("open breaker accepted request")
	}
	if calls["dead.invalid"] != 3 {
		t.Fatal("open breaker dialed host")
	}
	out, err := tr.RoundTrip(context.Background(), port.Request{URL: "https://live.invalid"})
	if err != nil || string(out.Body) != "ok" {
		t.Fatal(out, err)
	}
}
