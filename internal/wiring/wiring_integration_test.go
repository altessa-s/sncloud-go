//go:build integration

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package wiring

import (
	"context"
	"net/http"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/clouddouble"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestBuildBothCloudFlavors(t *testing.T) {
	for _, flavor := range []clouddouble.Flavor{clouddouble.Official, clouddouble.Private} {
		t.Run(string(flavor), func(t *testing.T) {
			cloud := clouddouble.New(flavor)
			defer cloud.Close()
			parts := Build(Config{
				Secrets:      &porttest.Secrets{},
				Endpoints:    cloud.Endpoints(),
				RoundTripper: http.DefaultTransport,
				Policy:       domain.DefaultRetryPolicy(),
			})
			defer parts.Transport.CloseIdleConnections()
			out, err := parts.Session.LogIn(context.Background(), "synthetic@example.com", "synthetic-password", nil)
			if err != nil || out != domain.LoggedIn {
				t.Fatal(out, err)
			}
			if err := parts.Session.LogOut(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
