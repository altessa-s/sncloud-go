// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestIdleReadTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tr := newTestTransport(t, Config{IdleReadTimeout: time.Second, RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    200,
				Header:        make(http.Header),
				ContentLength: -1,
				Body:          contextBody{ctx: r.Context()},
			}, nil
		})})
		start := time.Now()
		_, err := tr.RoundTrip(context.Background(), port.Request{URL: "https://synthetic.invalid"})
		var transport *domain.TransportError
		if !errors.As(err, &transport) || transport.Reason != domain.ReasonTimedOut || time.Since(start) != time.Second {
			t.Fatal(err, time.Since(start))
		}
	})
}
