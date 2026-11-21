//go:build integration

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/clouddouble"
	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

func TestSessionBothCloudFlavors(t *testing.T) {
	for _, flavor := range []clouddouble.Flavor{clouddouble.Official, clouddouble.Private} {
		for _, verify := range []bool{false, true} {
			name := string(flavor)
			if verify {
				name += "/email"
			}
			t.Run(name, func(t *testing.T) {
				cloud := clouddouble.New(flavor)
				defer cloud.Close()
				cloud.RequireVerification(verify)
				store := &porttest.Secrets{}
				c, err := sncloud.New(store, sncloud.WithEndpoints(cloud.Endpoints()), sncloud.WithRoundTripper(http.DefaultTransport), nil)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				out, err := c.LogIn(context.Background(), "synthetic@example.com", "synthetic-password", nil)
				if err != nil {
					t.Fatal(err)
				}
				if verify {
					if out != sncloud.VerificationRequired {
						t.Fatal(out)
					}
					if err := c.SendVerificationCode(context.Background()); err != nil {
						t.Fatal(err)
					}
					if err := c.Verify(context.Background(), "wrong"); !errors.Is(err, sncloud.ErrInvalidVerificationCode) {
						t.Fatal(err)
					}
					if err := c.Verify(context.Background(), "123456"); err != nil {
						t.Fatal(err)
					}
				} else if out != sncloud.LoggedIn {
					t.Fatal(out)
				}
				if ok, err := c.IsLoggedIn(); !ok || err != nil {
					t.Fatal(ok, err)
				}
				if account, err := c.Account(); account != "synthetic@example.com" || err != nil {
					t.Fatal(account, err)
				}
				expiry, err := c.Expiry()
				if err != nil || (expiry != nil) != (flavor == clouddouble.Official) {
					t.Fatal(expiry, err)
				}
				id, err := c.Identity()
				if err != nil {
					t.Fatal(err)
				}
				if err := c.LogOut(context.Background()); err != nil {
					t.Fatal(err)
				}
				// This session uses the infallible memory store; the value is the assertion here.
				if ok, _ := c.IsLoggedIn(); ok {
					t.Fatal("logout retained token")
				}
				// This session uses the infallible memory store; the value is the assertion here.
				after, _ := c.Identity()
				if id != after {
					t.Fatal("identity lost")
				}
				if err := c.Forget(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
