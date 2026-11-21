// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestRefusesABodyPastTheCeiling(t *testing.T) {
	for _, declared := range []bool{true, false} {
		t.Run(map[bool]string{true: "declared", false: "chunked"}[declared], func(t *testing.T) {
			closed := false
			tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				length := int64(-1)
				if declared {
					length = BodyLimit + 1
				}
				return &http.Response{
					StatusCode:    200,
					ContentLength: length,
					Header:        make(http.Header),
					Body:          &trackBody{Reader: io.LimitReader(zeroReader{}, BodyLimit+1), closed: &closed},
				}, nil
			})})
			_, err := tr.RoundTrip(context.Background(), port.Request{URL: "https://synthetic.invalid/x"})
			var over *domain.OversizedResponseError
			if !errors.As(err, &over) || over.Limit != BodyLimit || over.Bytes != BodyLimit+1 {
				t.Fatal(err)
			}
			if !closed {
				t.Fatal("body not closed")
			}
		})
	}
}

func TestRefusesAReplyThatIsNotHTTP(t *testing.T) {
	tr := newTestTransport(t, Config{
		RoundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, nil }),
	})
	if _, err := tr.RoundTrip(context.Background(), port.Request{URL: "https://synthetic.invalid"}); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatal(err)
	}
}
func TestReadFailureAndCanceledBody(t *testing.T) {
	for _, cancelBody := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		tr := newTestTransport(t, Config{RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if cancelBody {
				cancel()
			}
			return &http.Response{
				StatusCode:    200,
				Header:        make(http.Header),
				ContentLength: -1,
				Body:          io.NopCloser(errorReader{}),
			}, nil
		})})
		_, err := tr.RoundTrip(ctx, port.Request{URL: "https://synthetic.invalid"})
		cancel()
		if cancelBody {
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		} else if !errors.Is(err, domain.ErrTransport) {
			t.Fatal(err)
		}
	}
}
