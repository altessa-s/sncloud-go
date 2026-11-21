// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

var ctx = context.Background()

func challenge() porttest.Step {
	return porttest.OK(`"randomCode":"tOiJKwhW","timestamp":1773336349503`)
}
func setup(steps ...porttest.Step) (*Session, *porttest.Transport, *porttest.Secrets) {
	tr := &porttest.Transport{Steps: steps}
	store := &porttest.Secrets{}
	// The in-memory store cannot fail.
	_ = store.Keep("00000000-0000-4000-8000-000000000001", domain.ClientID)
	clock := &porttest.Clock{Time: time.Unix(2000, 0)}
	ch := channel.New(domain.Official(), tr, clock, domain.DefaultRetryPolicy())
	return New(ch, store, clock), tr, store
}
func login(t *testing.T, s *Session) {
	t.Helper()
	if out, err := s.LogIn(ctx, " a@example.com ", "correct horse", nil); err != nil || out != domain.LoggedIn {
		t.Fatalf("%s %v", out, err)
	}
}
func body(t *testing.T, r port.Request) map[string]any {
	t.Helper()
	var b map[string]any
	if err := json.Unmarshal(r.Body, &b); err != nil {
		t.Fatal(err)
	}
	return b
}
func held(reply porttest.Step) (porttest.Step, <-chan struct{}, chan<- struct{}) {
	arrived := make(chan struct{})
	release := make(chan struct{})
	return porttest.Step{Run: func(ctx context.Context, r port.Request) (port.Response, error) {
		close(arrived)
		select {
		case <-release:
			return reply.Response, reply.Err
		case <-ctx.Done():
			return port.Response{}, ctx.Err()
		}
	}}, arrived, release
}

type failingStore struct {
	*porttest.Secrets
	read, write, remove domain.Secret
	failure             error
}

func (s *failingStore) Secret(k domain.Secret) (string, error) {
	if k == s.read {
		return "", s.failure
	}
	return s.Secrets.Secret(k)
}
func (s *failingStore) Keep(v string, k domain.Secret) error {
	if k == s.write {
		return s.failure
	}
	return s.Secrets.Keep(v, k)
}
func (s *failingStore) Discard(k domain.Secret) error {
	// The embedded in-memory store cannot fail; this double injects its own error below.
	_ = s.Secrets.Discard(k)
	if k == s.remove {
		return s.failure
	}
	return nil
}
