// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package porttest

import (
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func TestSecrets(t *testing.T) {
	s := &Secrets{}
	if err := s.Keep("synthetic", domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	// This session uses the infallible memory store; the value is the assertion here.
	if got, _ := s.Secret(domain.AccessToken); got != "synthetic" {
		t.Fatal(got)
	}
	if err := s.Discard(domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	// This session uses the infallible memory store; the value is the assertion here.
	if got, _ := s.Secret(domain.AccessToken); got != "" {
		t.Fatal(got)
	}

}
