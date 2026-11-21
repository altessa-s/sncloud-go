// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"errors"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestReplacementCredentialGeneration(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "installed", true: "failed"}[failed], func(t *testing.T) {
			reply := porttest.OK(`"token":"account-b"`)
			if failed {
				reply = porttest.Step{Err: domain.ErrInvalidCredentials}
			}
			step, arrived, release := held(reply)
			s, _, _ := setup(challenge(), porttest.OK(`"token":"account-a"`), challenge(), step)
			login(t, s)
			before := s.Pin()
			done := make(chan error, 1)
			go func() { _, err := s.LogIn(ctx, "b@example.invalid", "password", nil); done <- err }()
			<-arrived
			during := s.Pin()
			token, err := s.AccessToken(before)
			if err != nil || token != "account-a" {
				t.Errorf("pending login invalidated A: %q %v", token, err)
			}
			close(release)
			err = <-done
			if failed && err == nil || !failed && err != nil {
				t.Fatalf("login: %v", err)
			}
			token, err = s.AccessToken(during)
			if failed {
				if err != nil || token != "account-a" {
					t.Fatalf("failed replacement harmed A: %q %v", token, err)
				}
			} else if !errors.Is(err, domain.ErrNotLoggedIn) {
				t.Fatalf("old generation returned %q, %v", token, err)
			}
		})
	}
}

func TestResendResponsesCannotRestoreObsoleteKey(t *testing.T) {
	step, arrived, release := held(porttest.OK(`"validCodeKey":"old"`))
	s, _, _ := setup(challenge(), porttest.Step{Err: domain.ErrVerificationRequired}, step, porttest.OK(`"validCodeKey":"new"`))
	if _, err := s.LogIn(ctx, "a@example.invalid", "password", nil); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.SendVerificationCode(ctx) }()
	<-arrived
	if err := s.SendVerificationCode(ctx); err != nil {
		t.Error(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, domain.ErrNoVerificationPending) {
		t.Errorf("obsolete resend: %v", err)
	}
	p, _, err := s.verification(true)
	if err != nil || p.key != "new" {
		t.Fatalf("key=%q err=%v", p.key, err)
	}
}

func TestLogoutDuringPendingLoginCannotEraseReplacement(t *testing.T) {
	loginStep, loginArrived, loginRelease := held(porttest.OK(`"token":"account-b"`))
	logoutStep, logoutArrived, logoutRelease := held(porttest.OK(""))
	s, _, store := setup(challenge(), porttest.OK(`"token":"account-a"`), challenge(), loginStep, logoutStep)
	login(t, s)
	loginDone := make(chan error, 1)
	go func() { _, err := s.LogIn(ctx, "b@example.invalid", "password", nil); loginDone <- err }()
	<-loginArrived
	logoutDone := make(chan error, 1)
	go func() { logoutDone <- s.LogOut(ctx) }()
	<-logoutArrived
	close(loginRelease)
	if err := <-loginDone; err != nil {
		t.Error(err)
	}
	close(logoutRelease)
	if err := <-logoutDone; err != nil {
		t.Error(err)
	}
	token, err := store.Secret(domain.AccessToken)
	if err != nil || token != "account-b" {
		t.Fatalf("old logout erased replacement: %q %v", token, err)
	}
}
