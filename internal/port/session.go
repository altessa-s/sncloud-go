// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package port

import (
	"context"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// AccountSession is the facade's session use case.
type AccountSession interface {
	LogIn(context.Context, string, string, *int) (domain.LoginOutcome, error)
	SendVerificationCode(context.Context) error
	Verify(context.Context, string) error
	LogOut(context.Context) error
	Forget() error
	Identity() (domain.DeviceIdentity, error)
	IsLoggedIn() (bool, error)
	Account() (string, error)
	Expiry() (*time.Time, error)
}

// IdleCloser releases idle transport resources.
type IdleCloser interface{ CloseIdleConnections() }
