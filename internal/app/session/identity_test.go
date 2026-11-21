// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func TestKeepsTheClientID(t *testing.T) {
	s, _, store := setup()
	if err := store.Discard(domain.ClientID); err != nil {
		t.Fatal(err)
	}
	id, err := s.Identity()
	if err != nil || len(id.ClientID) != 36 || id.ClientID != strings.ToLower(id.ClientID) {
		t.Fatal(id, err)
	}
	if err := s.Forget(); err != nil {
		t.Fatal(err)
	}
	// This session uses the infallible memory store; the value is the assertion here.
	again, _ := s.Identity()
	if id != again || domain.EquipmentNo(id) != id.Platform+"-"+id.ClientID || domain.EquipmentHeader(id) != id.Platform+"_"+id.ClientID {
		t.Fatal(id, again)
	}
}
func TestAppAndSocketIdentityAgree(t *testing.T) {
	s, _, store := setup()
	if _, _, _, err := s.SocketCredentials(nil); !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatal(err)
	}
	if err := store.Keep("synthetic", domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	h, err := s.AppHeaders("synthetic", true)
	if err != nil {
		t.Fatal(err)
	}
	token, kind, random, err := s.SocketCredentials(nil)
	if err != nil || h.Get("channel") != token+"_"+kind+"_"+random {
		t.Fatal(h, err)
	}
	if !s.ClaimSyncLock("device") || s.ClaimSyncLock("device") {
		t.Fatal("overlapping sync claim")
	}
	s.ReleaseSyncLock("device")
	if !s.ClaimSyncLock("device") {
		t.Fatal("sync claim retained")
	}
	if !s.ClaimDigestWrite(42) || s.ClaimDigestWrite(42) {
		t.Fatal("overlapping digest claim")
	}
	s.ReleaseDigestWrite(42)
	if !s.ClaimDigestWrite(42) {
		t.Fatal("digest claim retained")
	}
}

func TestUUIDVersionAndVariant(t *testing.T) {
	for range 32 {
		id, err := newUUID()
		if err != nil {
			t.Fatal(err)
		}
		if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' || id[14] != '4' {
			t.Fatalf("not a version 4 UUID: %q", id)
		}
		if !strings.ContainsRune("89ab", rune(id[19])) {
			t.Fatalf("not the RFC variant: %q", id)
		}
	}
}
