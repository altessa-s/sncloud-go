// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import "crypto/sha256"

// CredentialPin binds an operation to its first authenticated token. It holds
// only a fingerprint, never token text. Its owning session serializes access.
type CredentialPin struct {
	generation  uint64
	fingerprint [sha256.Size]byte
	bound       bool
}

// NewCredentialPin captures the local credential generation before the first step.
func NewCredentialPin(generation uint64) *CredentialPin {
	return &CredentialPin{generation: generation}
}

// Current rejects local credential mutations before reading storage.
func (p *CredentialPin) Current(generation uint64) bool { return p.generation == generation }

// Accept binds the first token and rejects any subsequent change of credential.
func (p *CredentialPin) Accept(token string) bool {
	fingerprint := sha256.Sum256([]byte(token))
	if !p.bound {
		p.fingerprint = fingerprint
		p.bound = true
	}
	return p.fingerprint == fingerprint
}
