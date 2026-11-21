// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package session owns credentials, login and the credential generation.
package session

import (
	"strconv"
	"sync"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Session serializes state transitions, never network calls. The token is
// opaque except for an optional JWT exp; private tokens may have
// none. There is no refresh or clock-skew correction. Credential-pin checks and token
// handover share one lock so multi-step operations cannot switch accounts.
type Session struct {
	mu      sync.Mutex
	channel port.Channel
	secrets port.SecretStore
	clock   port.Clock
	// generation advances before every credential Keep or Discard, even if it fails.
	// A failed write may have changed storage. We clear the cache, block authentication
	// until login succeeds, and attempt to discard the token, joining any cleanup error.
	// Neither cached nor stored uncertain credentials may authenticate this session.
	// attempt supersedes login/verification/logout conversations without invalidating
	// work using the existing credential when a replacement fails before storage.
	generation   uint64
	blocked      error
	attempt      uint64
	cached       string
	pending      *pending
	started      string
	syncLocks    map[string]bool
	digestWrites map[domain.FileID]bool
}

// New builds a session using the shared protocol channel.
func New(c port.Channel, s port.SecretStore, clock port.Clock) *Session {
	return &Session{
		channel:      c,
		secrets:      s,
		clock:        clock,
		started:      strconv.FormatInt(clock.Now().UnixMilli(), 10),
		syncLocks:    make(map[string]bool),
		digestWrites: make(map[domain.FileID]bool),
	}
}
