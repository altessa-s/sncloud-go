// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestDropsAnExpiredToken(t *testing.T) {
	s, _, store := setup()
	dead := "x." + base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1000}`)) + ".x"
	if err := store.Keep(dead, domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	if err := store.Keep("a@example.com", domain.Account); err != nil {
		t.Fatal(err)
	}
	// This session uses the infallible memory store; the value is the assertion here.
	if ok, _ := s.IsLoggedIn(); ok {
		t.Fatal("expired usable")
	}
	if _, err := s.AccessToken(nil); !errors.Is(err, domain.ErrSessionExpired) {
		t.Fatal(err)
	}
	// This session uses the infallible memory store; the value is the assertion here.
	if token, _ := store.Secret(domain.AccessToken); token != "" {
		t.Fatal("retained token")
	}
	// This session uses the infallible memory store; the value is the assertion here.
	if account, _ := s.Account(); account != "a@example.com" {
		t.Fatal(account)
	}
}
func TestPinsATokenToItsLogin(t *testing.T) {
	s, _, store := setup()
	if err := store.Keep("synthetic", domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	gen := s.Pin()
	if token, err := s.AccessToken(gen); token != "synthetic" || err != nil {
		t.Fatal(token, err)
	}
	if err := s.Forget(); err != nil {
		t.Fatal(err)
	}
	if err := store.Keep("new", domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AccessToken(gen); !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatal(err)
	}
	if token, err := s.AccessToken(nil); token != "new" || err != nil {
		t.Fatal(token, err)
	}
}
func TestInvalidatePreservesNewerToken(t *testing.T) {
	s, _, store := setup()
	if err := store.Keep("new", domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	s.Invalidate("old")
	if token, err := s.AccessToken(nil); token != "new" || err != nil {
		t.Fatal(token, err)
	}
	s.Invalidate("new")
	if _, err := s.AccessToken(nil); !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatal(err)
	}
}

func TestExpiryAndLogoutUseStoredClaimWithinMargin(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, now := range []int64{2100, 2401} {
			s, tr, store := setup(porttest.Refused("E0712"))
			token := "x." + base64.RawURLEncoding.EncodeToString([]byte(`{"exp":2400}`)) + ".x"
			if err := store.Keep(token, domain.AccessToken); err != nil {
				t.Fatal(err)
			}
			if cached {
				if _, err := s.AccessToken(nil); err != nil {
					t.Fatal(err)
				}
			}
			s.clock = &porttest.Clock{Time: time.Unix(now, 0)}
			if ok, err := s.IsLoggedIn(); ok || err != nil {
				t.Fatal(ok, err)
			}
			if expiry, err := s.Expiry(); err != nil || expiry == nil || expiry.Unix() != 2400 {
				t.Fatal(expiry, err)
			}
			if err := s.LogOut(ctx); err != nil {
				t.Fatal(err)
			}
			if len(tr.Calls()) != 1 || tr.Calls()[0].Header.Get(domain.AccessTokenHeader) != token {
				t.Fatal(tr.Calls())
			}
			if expiry, err := s.Expiry(); err != nil || expiry != nil {
				t.Fatal(expiry, err)
			}
			if stored, _ := store.Secret(domain.AccessToken); stored != "" {
				t.Fatal("token retained")
			}
		}
	}
}
