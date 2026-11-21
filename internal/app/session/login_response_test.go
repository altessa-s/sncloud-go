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

func TestRefusesAWrongPassword(t *testing.T) {
	s, tr, _ := setup(challenge(), porttest.Refused("E0019"))
	if _, err := s.LogIn(ctx, "a@example.com", "wrong", nil); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatal(err)
	}
	if len(tr.Calls()) != 2 {
		t.Fatal("retried password")
	}
}
func TestMalformedSessionResponses(t *testing.T) {
	for _, steps := range [][]porttest.Step{
		{porttest.OK(`"timestamp":1`)},
		{porttest.OK(`"randomCode":"salt"`)},
		{challenge(), porttest.OK(`"token":""`)},
	} {
		s, _, _ := setup(steps...)
		if _, err := s.LogIn(ctx, "a@example.com", "pw", nil); !errors.Is(err, domain.ErrMalformedResponse) {
			t.Fatal(err)
		}
	}
	s, tr, _ := setup()
	if _, err := s.LogIn(ctx, " ", "pw", nil); !errors.Is(err, domain.ErrInvalidCredentials) || len(tr.Calls()) != 0 {
		t.Fatal(err)
	}
	s, _, _ = setup(challenge(), porttest.Refused("E1760"), porttest.OK(""))
	if _, err := s.LogIn(ctx, "a@example.com", "pw", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SendVerificationCode(ctx); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatal(err)
	}
	s, _, _ = setup(challenge(), porttest.Refused("E1760"), porttest.OK(`"validCodeKey":"k"`), porttest.OK(""), porttest.OK(""))
	if _, err := s.LogIn(ctx, "a@example.com", "pw", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SendVerificationCode(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(ctx, "123456"); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatal(err)
	}
}
