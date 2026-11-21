// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"errors"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestLogoutSharedStore(t *testing.T) {
	for _, replaceDuring := range []bool{false, true} {
		t.Run(map[bool]string{false: "replacement before logout", true: "replacement during logout"}[replaceDuring], func(t *testing.T) {
			step, arrived, release := held(porttest.OK(""))
			s, tr, store := setup(step)
			if err := store.Keep("old", domain.AccessToken); err != nil {
				t.Fatal(err)
			}
			if _, err := s.AccessToken(nil); err != nil {
				t.Fatal(err)
			}
			otherTransport := &porttest.Transport{Steps: []porttest.Step{challenge(), porttest.OK(`"token":"replacement"`)}}
			other := New(channel.New(domain.Official(), otherTransport, s.clock, domain.DefaultRetryPolicy()), store, s.clock)
			if !replaceDuring {
				login(t, other)
			}
			done := make(chan error, 1)
			go func() { done <- s.LogOut(t.Context()) }()
			<-arrived
			if replaceDuring {
				login(t, other)
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			wantSent, wantStored, wantAccount := "replacement", "", ""
			if replaceDuring {
				wantSent, wantStored, wantAccount = "old", "replacement", "a@example.com"
			}
			if got := tr.Calls()[0].Header.Get(domain.AccessTokenHeader); got != wantSent {
				t.Errorf("revoked %q, want %q", got, wantSent)
			}
			if got, err := store.Secret(domain.AccessToken); err != nil || got != wantStored {
				t.Errorf("token %q, %v; want %q", got, err, wantStored)
			}
			if got, err := store.Secret(domain.Account); err != nil || got != wantAccount {
				t.Errorf("account %q, %v; want %q", got, err, wantAccount)
			}
		})
	}
}

func TestLogoutAfterFailedStoreWrite(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(map[bool]string{false: "retained old token", true: "retained new token"}[mutate], func(t *testing.T) {
			s, tr, store := setup(challenge(), porttest.OK(`"token":"new"`), porttest.OK(""))
			if err := store.Keep("old", domain.AccessToken); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("synthetic persistence failure")
			s.secrets = &cleanupFailureStore{uncertainStore: &uncertainStore{Secrets: store, key: domain.AccessToken, mutate: mutate, failure: failure}, cleanup: failure}
			if _, err := s.LogIn(t.Context(), "a@example.invalid", "password", nil); !errors.Is(err, failure) {
				t.Fatalf("login: %v", err)
			}
			// Storage recovers, but authentication remains blocked until a new login.
			s.secrets = store
			if _, err := s.AccessToken(nil); !errors.Is(err, domain.ErrNotLoggedIn) {
				t.Fatalf("authentication: %v", err)
			}
			if err := s.LogOut(t.Context()); err != nil {
				t.Fatal(err)
			}
			calls := tr.Calls()
			if len(calls) != 3 {
				t.Fatalf("calls=%d, want 3", len(calls))
			}
			want := "old"
			if mutate {
				want = "new"
			}
			if got := calls[2].Header.Get(domain.AccessTokenHeader); got != want {
				t.Fatalf("revoked %q, want %q", got, want)
			}
			for _, key := range []domain.Secret{domain.AccessToken, domain.Account} {
				if got, err := store.Secret(key); got != "" || err != nil {
					t.Errorf("stored %v=%q, %v", key, got, err)
				}
			}
		})
	}
}
