// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestLogsIn(t *testing.T) {
	s, tr, store := setup(challenge(), porttest.OK(`"token":"synthetic-token"`))
	login(t, s)
	calls := tr.Calls()
	if len(calls) != 2 {
		t.Fatal(calls)
	}
	if calls[0].URL != domain.Official().App(domain.PathRandomCode) || calls[1].URL != domain.Official().App(domain.PathLogin) {
		t.Fatal(calls)
	}
	for _, r := range calls {
		if r.Header.Get("version") != "202407" ||
			r.Header.Get("equipmentNo") == "" ||
			r.Header.Get("User-Agent") != domain.AppUserAgent ||
			r.Header.Get("channel") != "" ||
			r.Header.Get("Cookie") != "" {
			t.Fatal(r.Header)
		}
	}
	b := body(t, calls[1])
	if b["password"] != domain.PasswordDigest("correct horse", "tOiJKwhW") ||
		b["account"] != "a@example.com" ||
		b["equipment"] != float64(2) ||
		b["loginMethod"] != float64(2) ||
		b["language"] != "EN" {
		t.Fatal(b)
	}
	for _, k := range []string{"countryCode", "equipmentNo", "browser"} {
		v, ok := b[k]
		if !ok || v != nil {
			t.Fatal(k, b)
		}
	}
	if strings.Contains(string(calls[1].Body), "correct horse") {
		t.Fatal("plaintext password")
	}
	// This session uses the infallible memory store; the value is the assertion here.
	if token, _ := store.Secret(domain.AccessToken); token != "synthetic-token" {
		t.Fatal(token)
	}
	if ok, err := s.IsLoggedIn(); !ok || err != nil {
		t.Fatal(ok, err)
	}
}
func TestLogsInWithAPhone(t *testing.T) {
	s, tr, _ := setup(challenge(), porttest.OK(`"token":"synthetic"`))
	country := 1
	if _, err := s.LogIn(ctx, "5551234", "pw", &country); err != nil {
		t.Fatal(err)
	}
	for _, r := range tr.Calls() {
		if body(t, r)["countryCode"] != float64(1) {
			t.Fatal(string(r.Body))
		}
	}
	if body(t, tr.Calls()[1])["loginMethod"] != float64(1) {
		t.Fatal("not phone")
	}
}

func TestDoesNotReviveAForgottenLogin(t *testing.T) {
	step, arrived, release := held(porttest.OK(`"validCodeKey":"k"`))
	s, _, store := setup(challenge(), porttest.Refused("E1760"), step)
	if _, err := s.LogIn(ctx, "a@example.com", "pw", nil); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.SendVerificationCode(ctx) }()
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
func TestKeepsTheNewerLogin(t *testing.T) {
	step, arrived, release := held(porttest.OK(`"token":"old"`))
	s, _, store := setup(challenge(), step, challenge(), porttest.OK(`"token":"new"`))
	done := make(chan error, 1)
	go func() {
		_, err := s.LogIn(ctx, "old@example.com", "pw", nil)
		done <- err
	}()
	<-arrived
	if _, err := s.LogIn(ctx, "new@example.com", "pw", nil); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatal(err)
	}
	// This session uses the infallible memory store; the value is the assertion here.
	if token, _ := store.Secret(domain.AccessToken); token != "new" {
		t.Fatal(token)
	}
}
func TestDropsAnOvertakenLoginBeforeItSends(t *testing.T) {
	step, arrived, release := held(challenge())
	s, tr, _ := setup(step)
	done := make(chan error, 1)
	go func() {
		_, err := s.LogIn(ctx, "a@example.com", "pw", nil)
		done <- err
	}()
	<-arrived
	if err := s.Forget(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatal(err)
	}
	if len(tr.Calls()) != 1 {
		t.Fatal("digest sent")
	}
}
func TestPinsEachLoginToItsOwnConversation(t *testing.T) {
	step, arrived, release := held(challenge())
	s, tr, _ := setup(step, challenge(), porttest.OK(`"token":"new"`))
	done := make(chan error, 1)
	go func() {
		_, err := s.LogIn(ctx, "old@example.com", "pw", nil)
		done <- err
	}()
	<-arrived
	login(t, s)
	close(release)
	if err := <-done; !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatal(err)
	}
	if len(tr.Calls()) != 3 {
		t.Fatal("old digest sent")
	}
}
func TestStoreFailuresDoNotPublishPartialLogin(t *testing.T) {
	failure := errors.New("synthetic store failure")
	for _, key := range []domain.Secret{domain.AccessToken, domain.Account} {
		s, _, store := setup(challenge(), porttest.OK(`"token":"synthetic"`))
		s.secrets = &failingStore{Secrets: store, write: key, failure: failure}
		if _, err := s.LogIn(ctx, "a@example.com", "pw", nil); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		// This session uses the infallible memory store; the value is the assertion here.
		if token, _ := store.Secret(domain.AccessToken); token != "" {
			t.Fatal("partial token retained")
		}
	}
	s, _, store := setup()
	if err := store.Discard(domain.ClientID); err != nil {
		t.Fatal(err)
	}
	s.secrets = &failingStore{Secrets: store, write: domain.ClientID, failure: failure}
	if _, err := s.Identity(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	s.secrets = &failingStore{Secrets: store, read: domain.ClientID, failure: failure}
	if _, err := s.LogIn(ctx, "a@example.com", "pw", nil); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := s.AppHeaders("", true); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	s.secrets = &failingStore{Secrets: store, read: domain.AccessToken, failure: failure}
	if _, err := s.AccessToken(nil); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	s.secrets = &failingStore{Secrets: store, remove: domain.AccessToken, failure: failure}
	if err := store.Keep("account", domain.Account); err != nil {
		t.Fatal(err)
	}
	if err := s.Forget(); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	// This session uses the infallible memory store; the value is the assertion here.
	if account, _ := store.Secret(domain.Account); account != "" {
		t.Fatal("second deletion skipped")
	}
}
