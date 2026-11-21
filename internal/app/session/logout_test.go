// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"errors"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestForgetsOnLogoutWhateverTheServiceSays(t *testing.T) {
	s, _, store := setup(porttest.Step{Response: port.Response{Status: 503}})
	if err := store.Keep("synthetic", domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	if err := store.Keep("a@example.com", domain.Account); err != nil {
		t.Fatal(err)
	}
	if err := s.LogOut(ctx); !errors.Is(err, domain.ErrHTTP) {
		t.Fatal(err)
	}
	for _, k := range []domain.Secret{domain.AccessToken, domain.Account} {
		// This session uses the infallible memory store; the value is the assertion here.
		if v, _ := store.Secret(k); v != "" {
			t.Fatal(k, v)
		}
	}
	// This session uses the infallible memory store; the value is the assertion here.
	if id, _ := store.Secret(domain.ClientID); id == "" {
		t.Fatal("identity lost")
	}
}
func TestKeepsALoginMadeDuringLogout(t *testing.T) {
	step, arrived, release := held(porttest.OK(""))
	s, _, store := setup(step, challenge(), porttest.OK(`"token":"new"`))
	if err := store.Keep("old", domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.LogOut(ctx) }()
	<-arrived
	login(t, s)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// This session uses the infallible memory store; the value is the assertion here.
	if token, _ := store.Secret(domain.AccessToken); token != "new" {
		t.Fatal(token)
	}
}

func TestLogsOutOfAnEndedSession(t *testing.T) {
	s, _, store := setup(porttest.Refused("E0712"))
	if err := store.Keep("synthetic", domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	if err := s.LogOut(ctx); err != nil {
		t.Fatal(err)
	}
}
