// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"errors"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Pin starts an operation whose first token handover binds its credential.
func (s *Session) Pin() *domain.CredentialPin {
	s.mu.Lock()
	defer s.mu.Unlock()
	return domain.NewCredentialPin(s.generation)
}

// AccessToken checks the optional pin and returns the token atomically.
func (s *Session) AccessToken(expected *domain.CredentialPin) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Check the pin and hand over its token under one lock; otherwise
	// a multi-step operation could send the next step using another account.
	return s.accessToken(expected)
}
func (s *Session) accessToken(expected *domain.CredentialPin) (string, error) {
	if s.blocked != nil {
		return "", errors.Join(domain.ErrNotLoggedIn, s.blocked)
	}
	if expected != nil && !expected.Current(s.generation) {
		return "", domain.ErrNotLoggedIn
	}
	// Read storage at every handover: other Clients and processes do not share
	// our generation or cache. A pin must never follow their replacement login.
	token, err := s.secrets.Secret(domain.AccessToken)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", domain.ErrNotLoggedIn
	}
	if expected != nil && !expected.Accept(token) {
		return "", domain.ErrNotLoggedIn
	}
	if !domain.TokenUsable(token, s.clock.Now()) {
		// Best effort: this token must not be used even if local deletion fails.
		err := s.discardSecret(domain.AccessToken)
		s.cached = ""
		return "", errors.Join(domain.ErrSessionExpired, err)
	}
	s.cached = token
	return token, nil
}

// IsLoggedIn reports local usability without making a network request.
func (s *Session) IsLoggedIn() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.token()
	return t != "", err
}

// token reports local usability with the request margin applied.
func (s *Session) token() (string, error) {
	if s.blocked != nil {
		return "", errors.Join(domain.ErrNotLoggedIn, s.blocked)
	}
	token := s.cached
	if token == "" || !domain.TokenUsable(token, s.clock.Now()) {
		var err error
		token, err = s.secrets.Secret(domain.AccessToken)
		if err != nil {
			return "", err
		}
	}
	if token != "" && !domain.TokenUsable(token, s.clock.Now()) {
		return "", nil
	}
	return token, nil
}

// storedToken reads credentials without applying the request margin so expiry
// inspection preserves the claim even after the token expires.
// The caller holds mu.
func (s *Session) storedToken() (string, error) {
	if s.blocked != nil {
		return "", errors.Join(domain.ErrNotLoggedIn, s.blocked)
	}
	if s.cached != "" {
		return s.cached, nil
	}
	return s.secrets.Secret(domain.AccessToken)
}

// Account returns the account spelling retained at login.
func (s *Session) Account() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.secrets.Secret(domain.Account)
}

// Expiry reads the stored expiry claim, including within the request margin
// and after expiration. An absent or opaque token has no expiry.
func (s *Session) Expiry() (*time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.storedToken()
	return domain.TokenExpiry(t), err
}

// Invalidate drops only the refused token. An old call may finish after another
// login has installed a new token; its refusal says nothing about that login.
// Matching both the cache and store avoids erasing the newer credential.
func (s *Session) Invalidate(refused string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached == refused {
		s.cached = ""
	}
	if t, err := s.secrets.Secret(domain.AccessToken); err == nil && t == refused {
		// Best effort: this token must not be used even if local deletion fails.
		_ = s.discardSecret(domain.AccessToken)
	}
}

func (s *Session) keep(token, account string, attempt uint64) error {
	// The caller holds mu across validation and storage, preventing a late
	// login response from overwriting the current attempt's credentials.
	if attempt != s.attempt {
		return domain.ErrNotLoggedIn
	}
	if token == "" {
		return &domain.MalformedResponseError{What: "the login answered with no token"}
	}
	if err := s.keepSecret(token, domain.AccessToken); err != nil {
		return err
	}
	if err := s.keepSecret(account, domain.Account); err != nil {
		return err
	}
	s.cached = token
	s.blocked = nil
	return nil
}

// The caller holds mu for all credential mutations and their cleanup.
func (s *Session) keepSecret(value string, key domain.Secret) error {
	s.generation++
	return s.writeResult(s.secrets.Keep(value, key))
}

func (s *Session) discardSecret(key domain.Secret) error {
	s.generation++
	return s.writeResult(s.secrets.Discard(key))
}

func (s *Session) writeResult(err error) error {
	if err == nil {
		return nil
	}
	s.cached = ""
	s.generation++
	cleanup := s.secrets.Discard(domain.AccessToken)
	s.blocked = errors.Join(err, cleanup)
	return s.blocked
}
