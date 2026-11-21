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

func TestLogsInWithAnEmailedCode(t *testing.T) {
	s, tr, store := setup(
		challenge(),
		porttest.Refused("E1760"),
		porttest.OK(`"validCodeKey":"key-42"`),
		porttest.OK(""),
		porttest.OK(`"token":"synthetic"`),
	)
	if o, e := s.LogIn(ctx, "user@example.com", "pw", nil); o != domain.VerificationRequired || e != nil {
		t.Fatal(o, e)
	}
	if e := s.SendVerificationCode(ctx); e != nil {
		t.Fatal(e)
	}
	if e := s.Verify(ctx, " 123456 "); e != nil {
		t.Fatal(e)
	}
	calls := tr.Calls()
	for i, path := range []string{domain.PathMailCode, domain.PathCheckCode, domain.PathCodeLogin} {
		if calls[i+2].URL != domain.Official().App(path) {
			t.Fatal(calls[i+2])
		}
	}
	if body(t, calls[3])["validCode"] != "123456" || body(t, calls[4])["telephone"] != nil || body(t, calls[4])["equipment"] != float64(2) {
		t.Fatal("wrong code body")
	}
	// This session uses the infallible memory store; the value is the assertion here.
	if token, _ := store.Secret(domain.AccessToken); token != "synthetic" {
		t.Fatal(token)
	}
	if err := s.Verify(ctx, "x"); !errors.Is(err, domain.ErrNoVerificationPending) {
		t.Fatal(err)
	}
}
func TestKeepsTheLoginPendingAfterAWrongCode(t *testing.T) {
	s, _, _ := setup(
		challenge(),
		porttest.Refused("E1760"),
		porttest.OK(`"validCodeKey":"k"`),
		porttest.Refused("E1756"),
		porttest.OK(""),
		porttest.OK(`"token":"synthetic"`),
	)
	if _, err := s.LogIn(ctx, "user@example.com", "pw", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SendVerificationCode(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(ctx, "wrong"); !errors.Is(err, domain.ErrInvalidVerificationCode) {
		t.Fatal(err)
	}
	if err := s.Verify(ctx, "right"); err != nil {
		t.Fatal(err)
	}
}
func TestRefusesACodeWithNothingPending(t *testing.T) {
	s, _, _ := setup()
	if err := s.SendVerificationCode(ctx); !errors.Is(err, domain.ErrNoVerificationPending) {
		t.Fatal(err)
	}
}
func TestVerificationOvertakenAtEachStep(t *testing.T) {
	for _, check := range []bool{true, false} {
		steps := []porttest.Step{challenge(), porttest.Refused("E1760"), porttest.OK(`"validCodeKey":"k"`)}
		reply := porttest.OK("")
		if !check {
			steps = append(steps, reply)
			reply = porttest.OK(`"token":"old"`)
		}
		step, arrived, release := held(reply)
		steps = append(steps, step)
		s, _, store := setup(steps...)
		if _, err := s.LogIn(ctx, "a@example.com", "pw", nil); err != nil {
			t.Fatal(err)
		}
		if err := s.SendVerificationCode(ctx); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- s.Verify(ctx, "123456") }()
		<-arrived
		if err := s.Forget(); err != nil {
			t.Fatal(err)
		}
		close(release)
		if err := <-done; !errors.Is(err, domain.ErrNoVerificationPending) {
			t.Fatal(err)
		}
		// This session uses the infallible memory store; the value is the assertion here.
		if token, _ := store.Secret(domain.AccessToken); token != "" {
			t.Fatal(token)
		}
	}
}
func TestVerificationFailuresStayPending(t *testing.T) {
	s, _, _ := setup(
		challenge(),
		porttest.Refused("E1760"),
		porttest.Refused("E0101"),
		porttest.OK(`"validCodeKey":"k"`),
		porttest.OK(""),
		porttest.Refused("E1756"),
	)
	if _, err := s.LogIn(ctx, "a@example.com", "pw", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SendVerificationCode(ctx); !errors.Is(err, domain.ErrInvalidVerificationCode) {
		t.Fatal(err)
	}
	if err := s.SendVerificationCode(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(ctx, "123456"); !errors.Is(err, domain.ErrInvalidVerificationCode) {
		t.Fatal(err)
	}
}

func TestVerificationSupersededWithoutNewLogin(t *testing.T) {
	for _, check := range []bool{true, false} {
		for _, resend := range []bool{true, false} {
			steps := []porttest.Step{challenge(), porttest.Refused("E1760"), porttest.OK(`"validCodeKey":"k1"`)}
			if !check {
				steps = append(steps, porttest.OK(""))
			}
			step, arrived, release := held(porttest.OK(`"token":"stale"`))
			steps = append(steps, step)
			if resend {
				steps = append(steps, porttest.OK(`"validCodeKey":"k1"`))
			} else {
				steps = append(steps, porttest.OK(""), porttest.OK(`"token":"winner"`))
			}
			s, tr, store := setup(steps...)
			if _, err := s.LogIn(ctx, "a@example.com", "pw", nil); err != nil {
				t.Fatal(err)
			}
			if err := s.SendVerificationCode(ctx); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- s.Verify(ctx, "123456") }()
			<-arrived
			if resend {
				if err := s.SendVerificationCode(ctx); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := s.Verify(ctx, "123456"); err != nil {
					t.Fatal(err)
				}
			}
			count := len(tr.Calls())
			close(release)
			if err := <-done; !errors.Is(err, domain.ErrNoVerificationPending) {
				t.Errorf("check=%v resend=%v: %v", check, resend, err)
			}
			if len(tr.Calls()) != count {
				t.Error("stale request sent")
			}
			token, _ := store.Secret(domain.AccessToken)
			if resend && token != "" || !resend && token != "winner" {
				t.Error(token)
			}
		}
	}
}
func TestEmptyVerificationKeyIsMalformed(t *testing.T) {
	s, _, _ := setup(challenge(), porttest.Refused("E1760"), porttest.OK(`"validCodeKey":""`))
	if _, err := s.LogIn(ctx, "a@example.com", "pw", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SendVerificationCode(ctx); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatal(err)
	}
}
