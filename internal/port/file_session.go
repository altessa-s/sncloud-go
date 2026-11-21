// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package port

import "github.com/altessa-s/sncloud-go/internal/domain"

// FileSession supplies credentials atomically pinned to an optional operation pin.
type FileSession interface {
	ClaimSyncLock(string) bool
	ReleaseSyncLock(string)
	Pin() *domain.CredentialPin
	AccessToken(*domain.CredentialPin) (string, error)
	Invalidate(string)
	AppHeaders(string, bool) (Header, error)
	Identity() (domain.DeviceIdentity, error)
}
