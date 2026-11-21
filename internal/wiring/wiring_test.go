// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package wiring

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

type noNetwork struct{ t *testing.T }

func (n noNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	n.t.Fatal("building or inspecting the graph performed network I/O")
	return nil, nil
}

func TestBuildConnectsLocalSessionWithoutIO(t *testing.T) {
	store := &porttest.Secrets{}
	if err := store.Keep("opaque", domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	parts := Build(Config{
		Secrets:      store,
		Endpoints:    domain.Official(),
		RoundTripper: noNetwork{t},
		Policy:       domain.DefaultRetryPolicy(),
	})
	defer parts.Transport.CloseIdleConnections()
	if ok, err := parts.Session.IsLoggedIn(); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if err := parts.Session.Forget(); err != nil {
		t.Fatal(err)
	}
	if token, err := store.Secret(domain.AccessToken); token != "" || err != nil {
		t.Fatal(token, err)
	}
	ctx := context.Background()
	if _, err := parts.Files.Capacity(ctx); !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatalf("Files without login = %v, want ErrNotLoggedIn", err)
	}
	if _, err := parts.Device.SpaceUsage(ctx); !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatalf("Device without login = %v, want ErrNotLoggedIn", err)
	}
	if _, err := parts.Tasks.Lists(ctx); !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatalf("Tasks without login = %v, want ErrNotLoggedIn", err)
	}
	if _, err := parts.Digests.Groups(ctx); !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatalf("Digests without login = %v, want ErrNotLoggedIn", err)
	}
	for _, err := range parts.Feed.Changes(ctx) {
		if !errors.Is(err, domain.ErrNotLoggedIn) {
			t.Fatalf("Feed without login = %v, want ErrNotLoggedIn", err)
		}
	}
}
