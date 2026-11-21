// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package feed

import (
	"errors"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/app/session"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestWatchCannotReconnectUnderAnotherAccount(t *testing.T) {
	dialer := &porttest.SocketDialer{Steps: []porttest.DialStep{
		{Socket: porttest.NewSocket(openFrame, "40", "41")},
		{Socket: porttest.NewSocket(openFrame, "40", "41")},
		{Err: domain.ErrSessionExpired},
	}}
	f, store := newTestFeed(domain.Official(), dialer, &porttest.Clock{})
	var got error
	connected := 0
	for ch, err := range f.Watch(t.Context()) {
		if err != nil {
			got = err
			break
		}
		if ch.Type == domain.ChangeConnected {
			connected++
			if connected == 1 {
				if err := f.session.(*session.Session).Forget(); err != nil {
					t.Fatal(err)
				}
				if err := store.Keep("account-b", domain.AccessToken); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if !errors.Is(got, domain.ErrNotLoggedIn) || connected != 1 || len(dialer.Calls()) != 1 {
		t.Fatalf("watch switched account: %v, events=%d, dials=%d", got, connected, len(dialer.Calls()))
	}
}
