// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package porttest

import (
	"sync"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Secrets is a memory-only store. Its zero value is ready for use.
type Secrets struct {
	mu     sync.Mutex
	values map[domain.Secret]string
}

// Secret reads a value.
func (s *Secrets) Secret(k domain.Secret) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[k], nil
}

// Keep stores a value.
func (s *Secrets) Keep(v string, k domain.Secret) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[domain.Secret]string)
	}
	s.values[k] = v
	return nil
}

// Discard removes a value.
func (s *Secrets) Discard(k domain.Secret) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, k)
	return nil
}
