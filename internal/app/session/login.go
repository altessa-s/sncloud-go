// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"context"
	"encoding/json/v2"
	"errors"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// LogIn never stores or logs the password; only its salted digest leaves the
// process. Go strings cannot be reliably erased from memory. CountryCode
// distinguishes a phone login; nil is encoded as the app's explicit null.
func (s *Session) LogIn(ctx context.Context, account, password string, countryCode *int) (domain.LoginOutcome, error) {
	account = strings.TrimSpace(account)
	if account == "" || password == "" {
		return "", domain.ErrInvalidCredentials
	}
	s.mu.Lock()
	// Supersede in-flight logins before any I/O, and discard their browser
	// conversation under the same lock so old cookies cannot reach this login.
	s.attempt++
	attempt := s.attempt
	s.pending = nil
	epoch := s.channel.Forget()
	headers, err := s.appHeaders("", false)
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	var challenge struct {
		RandomCode *string         `json:"randomCode"`
		Timestamp  domain.WireEcho `json:"timestamp"`
	}
	err = s.channel.Call(ctx, port.Call{
		Path:   domain.PathRandomCode,
		Body:   challengeRequest{Account: account, CountryCode: countryCode},
		Header: headers,
		Epoch:  &epoch,
	}, &challenge)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	// Network calls release the lock: a newer login or Forget may now own it.
	if attempt != s.attempt {
		s.mu.Unlock()
		return "", domain.ErrNotLoggedIn
	}
	id, err := s.identity()
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	if challenge.RandomCode == nil {
		return "", &domain.MalformedResponseError{What: "challenge has no randomCode"}
	}
	if _, marshalErr := json.Marshal(challenge.Timestamp); marshalErr != nil {
		return "", &domain.MalformedResponseError{What: "challenge has no timestamp"}
	}
	method := emailLogin
	if countryCode != nil {
		method = phoneLogin
	}
	body := loginRequest{
		Account:     account,
		Password:    domain.PasswordDigest(password, *challenge.RandomCode),
		CountryCode: countryCode,
		Timestamp:   challenge.Timestamp,
		Equipment:   appEquipment,
		LoginMethod: method,
		Language:    domain.Language,
		Devices:     id.Platform,
	}
	var answer struct {
		Token string `json:"token"`
	}
	err = s.channel.Call(ctx, port.Call{Path: domain.PathLogin, Body: body, Header: headers, Epoch: &epoch, Changes: true}, &answer)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Network calls release the lock: a newer login or Forget may now own it.
	if attempt != s.attempt {
		return "", domain.ErrNotLoggedIn
	}
	if errors.Is(err, domain.ErrVerificationRequired) {
		// Verification continues this challenge and its browser conversation;
		// resetting the epoch here would invalidate the emailed-code exchange.
		s.pending = &pending{attempt: attempt, epoch: epoch, account: account}
		return domain.VerificationRequired, nil
	}
	if err != nil {
		return "", err
	}
	if err := s.keep(answer.Token, account, attempt); err != nil {
		return "", err
	}
	return domain.LoggedIn, nil
}
