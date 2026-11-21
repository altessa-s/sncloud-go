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

type uncertainStore struct {
	*porttest.Secrets
	key            domain.Secret
	remove, mutate bool
	failure        error
}

func (s *uncertainStore) Keep(v string, k domain.Secret) error {
	if k != s.key || s.remove {
		return s.Secrets.Keep(v, k)
	}
	if s.mutate {
		_ = s.Secrets.Keep(v, k)
	}
	return s.failure
}
func (s *uncertainStore) Discard(k domain.Secret) error {
	if k != s.key || !s.remove {
		return s.Secrets.Discard(k)
	}
	if s.mutate {
		_ = s.Secrets.Discard(k)
	}
	return s.failure
}

func TestFailedCredentialMutationBlocksAuthentication(t *testing.T) {
	for _, mutation := range []string{"password", "code", "logout", "forget", "account", "identity", "forget-account", "invalidate", "expiry"} {
		for _, mutate := range []bool{false, true} {
			t.Run(mutation+map[bool]string{false: "/unchanged", true: "/changed"}[mutate], func(t *testing.T) {
				s, _, store := setup()
				old := "x." + base64.RawURLEncoding.EncodeToString([]byte(`{"exp":2400}`)) + ".x"
				_ = store.Keep(old, domain.AccessToken)
				if _, err := s.AccessToken(nil); err != nil {
					t.Fatal(err)
				}
				gen := s.Pin()
				failure := errors.New("persistence failed")
				faulty := &uncertainStore{Secrets: store, key: domain.AccessToken, mutate: mutate, failure: failure}
				s.secrets = faulty
				var err error
				switch mutation {
				case "logout", "forget", "forget-account":
					faulty.remove = true
					if mutation == "forget-account" {
						faulty.key = domain.Account
					}
					if mutation != "logout" {
						err = s.Forget()
					} else {
						err = s.LogOut(ctx)
					}
				case "invalidate", "expiry":
					faulty.remove = true
					if mutation == "invalidate" {
						s.Invalidate(old)
						_, err = s.AccessToken(nil)
					} else {
						s.clock = &porttest.Clock{Time: time.Unix(2101, 0)}
						_, err = s.AccessToken(nil)
					}
				case "identity":
					faulty.key = domain.ClientID
					_ = store.Discard(domain.ClientID)
					_, err = s.Identity()
				default:
					if mutation == "account" {
						faulty.key = domain.Account
					}
					steps := []porttest.Step{challenge(), porttest.OK(`"token":"account-b"`)}
					if mutation == "code" {
						steps = []porttest.Step{challenge(), {Err: domain.ErrVerificationRequired}, porttest.OK(`"validCodeKey":"key"`), porttest.OK(""), porttest.OK(`"token":"account-b"`)}
					}
					other, _, _ := setup(steps...)
					s.channel = other.channel
					_, err = s.LogIn(ctx, "b@example.invalid", "password", nil)
					if mutation == "code" {
						if err != nil {
							t.Fatal(err)
						}
						if err = s.SendVerificationCode(ctx); err != nil {
							t.Fatal(err)
						}
						err = s.Verify(ctx, "123456")
					}
				}
				if !errors.Is(err, failure) {
					t.Fatalf("mutation: %v", err)
				}
				for _, margin := range []bool{false, true} {
					if margin {
						s.clock = &porttest.Clock{Time: time.Unix(2101, 0)}
					}
					for _, pin := range []*domain.CredentialPin{gen, nil} {
						token, err := s.AccessToken(pin)
						if !errors.Is(err, domain.ErrNotLoggedIn) || token != "" {
							t.Errorf("margin=%v token=%q err=%v", margin, token, err)
						}
					}
					if ok, err := s.IsLoggedIn(); ok || !errors.Is(err, domain.ErrNotLoggedIn) {
						t.Errorf("logged in=%v err=%v", ok, err)
					}
				}
			})
		}
	}
}

func TestFailedWriteReportsCleanupAndRequiresSuccessfulLogin(t *testing.T) {
	s, _, store := setup(challenge(), porttest.OK(`"token":"account-b"`), challenge(), porttest.OK(`"token":"account-c"`))
	writeErr, cleanupErr := errors.New("write failed"), errors.New("cleanup failed")
	faulty := &uncertainStore{Secrets: store, key: domain.AccessToken, mutate: true, failure: writeErr}
	// Fail the installation and its best-effort cleanup independently.
	s.secrets = &cleanupFailureStore{uncertainStore: faulty, cleanup: cleanupErr}
	_, err := s.LogIn(ctx, "b@example.invalid", "password", nil)
	if !errors.Is(err, writeErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("lost failure: %v", err)
	}
	if token, _ := store.Secret(domain.AccessToken); token != "account-b" {
		t.Fatalf("fixture did not retain uncertain value: %q", token)
	}
	s.secrets = store
	if _, err := s.AccessToken(nil); !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatal(err)
	}
	if _, err := s.LogIn(ctx, "c@example.invalid", "password", nil); err != nil {
		t.Fatal(err)
	}
	if token, err := s.AccessToken(nil); err != nil || token != "account-c" {
		t.Fatal(token, err)
	}
}

type cleanupFailureStore struct {
	*uncertainStore
	cleanup error
}

func (s *cleanupFailureStore) Discard(domain.Secret) error { return s.cleanup }
