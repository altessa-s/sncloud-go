// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"context"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

type pending struct {
	attempt, epoch, revision uint64
	account, key             string
}

func (s *Session) verification(needsKey bool) (pending, port.Header, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil || !strings.Contains(s.pending.account, "@") || needsKey && s.pending.key == "" {
		return pending{}, nil, domain.ErrNoVerificationPending
	}
	// Reserve the revision before sending: a slower resend cannot restore its key,
	// and verification already in flight must not exchange an obsolete challenge.
	if !needsKey {
		s.pending.revision++
	}
	h, err := s.appHeaders("", false)
	return *s.pending, h, err
}

// SendVerificationCode requests the email for the pending login.
func (s *Session) SendVerificationCode(ctx context.Context) error {
	p, h, err := s.verification(false)
	if err != nil {
		return err
	}
	var answer struct {
		Key *string `json:"validCodeKey"`
	}
	err = s.channel.Call(ctx, port.Call{
		Path:    domain.PathMailCode,
		Body:    mailCodeRequest{Email: p.account, Language: domain.Language},
		Header:  h,
		Changes: true,
		Epoch:   &p.epoch,
	}, &answer)
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.attempt != s.attempt || s.pending == nil || s.pending.revision != p.revision {
		return domain.ErrNoVerificationPending
	}
	if err != nil {
		return err
	}
	if answer.Key == nil || *answer.Key == "" {
		return &domain.MalformedResponseError{What: "code request has no validCodeKey"}
	}
	s.pending.key = *answer.Key
	return nil
}

// Verify checks the emailed code before exchanging it for a token. A wrong
// code leaves the same login pending so the caller may try again.
func (s *Session) Verify(ctx context.Context, code string) error {
	p, h, err := s.verification(true)
	if err != nil {
		return err
	}
	code = strings.TrimSpace(code)
	err = s.channel.Call(ctx, port.Call{
		Path:   domain.PathCheckCode,
		Body:   checkCodeRequest{Code: code, Key: p.key},
		Header: h,
		Epoch:  &p.epoch,
	}, nil)
	s.mu.Lock()
	if p.attempt != s.attempt || s.pending == nil || s.pending.revision != p.revision || s.pending.key != p.key {
		s.mu.Unlock()
		return domain.ErrNoVerificationPending
	}
	if err != nil {
		s.mu.Unlock()
		return err
	}
	id, err := s.identity()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	var answer struct {
		Token string `json:"token"`
	}
	err = s.channel.Call(ctx, port.Call{
		Path:    domain.PathCodeLogin,
		Body:    codeLoginRequest{Equipment: appEquipment, Code: code, Key: p.key, Devices: id.Platform, Email: p.account},
		Header:  h,
		Epoch:   &p.epoch,
		Changes: true,
	}, &answer)
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.attempt != s.attempt || s.pending == nil || s.pending.revision != p.revision || s.pending.key != p.key {
		return domain.ErrNoVerificationPending
	}
	if err != nil {
		return err
	}
	if err := s.keep(answer.Token, p.account, p.attempt); err != nil {
		return err
	}
	s.pending = nil
	return nil
}
