// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"github.com/altessa-s/sncloud-go/internal/domain"
)

// ClaimSyncLock excludes overlapping scopes sharing this session and device.
func (s *Session) ClaimSyncLock(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syncLocks[name] {
		return false
	}
	s.syncLocks[name] = true
	return true
}

// ReleaseSyncLock releases the local claim after remote cleanup.
func (s *Session) ReleaseSyncLock(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.syncLocks, name)
}

// ClaimDigestWrite serializes writes to one digest across clients.
func (s *Session) ClaimDigestWrite(id domain.FileID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.digestWrites[id] {
		return false
	}
	s.digestWrites[id] = true
	return true
}

// ReleaseDigestWrite releases a digest claim.
func (s *Session) ReleaseDigestWrite(id domain.FileID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.digestWrites, id)
}
