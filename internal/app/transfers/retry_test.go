// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package transfers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/app/session"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

type backoffClock struct {
	porttest.Clock
	during func()
}

func (c *backoffClock) Sleep(context.Context, time.Duration) error { c.during(); return nil }

func TestPartRetryRefreshesSignatureAndPinsGeneration(t *testing.T) {
	for _, changeLogin := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired signature", true: "changed generation"}[changeLogin], func(t *testing.T) {
			policy := domain.DefaultRetryPolicy()
			signatures := &porttest.Transport{Steps: []porttest.Step{
				porttest.OK(`"s3Authorization":"first","xamzDate":"old"`),
				porttest.OK(`"s3Authorization":"fresh","xamzDate":"new"`),
			}}
			clk := &backoffClock{}
			ch := channel.New(domain.Official(), signatures, clk, policy)
			secrets := &porttest.Secrets{}
			_ = secrets.Keep("synthetic", domain.AccessToken)
			sess := session.New(ch, secrets, clk)
			signer := NewSigner(ch, sess, nil, sess.Pin())
			expired := false
			clk.during = func() {
				expired = true
				if changeLogin {
					_ = sess.LogOut(t.Context())
				}
			}
			storage := &porttest.Transport{Steps: []porttest.Step{
				{Run: func(context.Context, port.Request) (port.Response, error) { return port.Response{Status: 503}, nil }},
				{Run: func(_ context.Context, r port.Request) (port.Response, error) {
					if !expired || r.Header.Get("Authorization") != "fresh" || r.Header.Get("x-amz-date") != "new" {
						t.Fatalf("expired credentials reused: %v", r.Header)
					}
					return port.Response{Status: 200}, nil
				}},
			}}
			tr := New(storage, clk, policy)
			o := source(t, []byte("part"))
			_, err := tr.s3(t.Context(), "https://storage.invalid/key", SigningRequest{Method: "PUT", Key: "key", Params: map[string]string{"partNumber": "1"}}, signer, o.File, nil, nil, false)
			if changeLogin {
				if !errors.Is(err, domain.ErrNotLoggedIn) || len(storage.Calls()) != 1 {
					t.Fatalf("changed login retransmitted: %v, %d calls", err, len(storage.Calls()))
				}
			} else if err != nil || len(storage.Calls()) != 2 || len(signatures.Calls()) != 2 {
				t.Fatalf("retry = %v, storage=%d signatures=%d", err, len(storage.Calls()), len(signatures.Calls()))
			}
		})
	}
}
