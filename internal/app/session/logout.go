// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"context"
	"errors"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Forget ends the local conversation even if one store deletion fails.
func (s *Session) Forget() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.forget()
}
func (s *Session) forget() error {
	s.attempt++
	s.cached = ""
	s.pending = nil
	err := s.discardSecret(domain.AccessToken)
	other := s.discardSecret(domain.Account)
	s.channel.Forget()
	return errors.Join(err, other)
}

// LogOut forgets locally regardless of the server's answer, unless a newer
// login has overtaken it. The caller asked to stop using this account; a server
// failure must not leave credentials ready for the next call. A newer login
// changes the attempt or credential generation and must survive the older logout.
// An already expired session counts as logged out.
func (s *Session) LogOut(ctx context.Context) error {
	s.mu.Lock()
	// Pin both the credential and conversation before releasing the lock for I/O.
	attempt := s.attempt
	// A failed store write blocks authentication, but logout must still revoke
	// any credential left in storage when best-effort cleanup failed.
	token, err := s.secrets.Secret(domain.AccessToken)
	epoch := s.channel.Epoch()
	var h port.Header
	if err == nil && token != "" {
		h, err = s.appHeaders(token, false)
	}
	gen := s.generation
	s.mu.Unlock()
	if err == nil && token != "" {
		err = s.channel.Call(ctx, port.Call{
			Path:    domain.PathLogout,
			Body:    struct{}{},
			Token:   token,
			Header:  h,
			Changes: true,
			Epoch:   &epoch,
		}, nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// An older logout must not erase a login completed while it was on the wire.
	if attempt != s.attempt || gen != s.generation {
		return nil
	}
	// Another Client may have replaced the token while this request was in flight.
	current, readErr := s.secrets.Secret(domain.AccessToken)
	if readErr != nil {
		s.cached = ""
		return errors.Join(err, readErr)
	}
	if current != token {
		s.cached = ""
		return err
	}
	// Local logout is unconditional for this token, even if the server failed.
	forgetErr := s.forget()
	if errors.Is(err, domain.ErrSessionExpired) {
		err = nil
	}
	return errors.Join(err, forgetErr)
}
