// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package devicefiles

import (
	"context"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

const releaseTimeout = 5 * time.Second

// WithSyncLock runs body under the account lock. Cleanup survives cancellation,
// and its error is returned only when body succeeded. Nested scopes fail locally.
func (c *Client) WithSyncLock(ctx context.Context, body func(context.Context, bool) error) (err error) {
	name, err := c.equipment()
	if err != nil {
		return err
	}
	if !c.session.ClaimSyncLock(name) {
		return domain.ErrSyncLocked
	}
	defer c.session.ReleaseSyncLock(name)
	// Release belongs to the login that acquired the lock. A later login must
	// never send the previous account's release with its own token.
	pin := c.session.Pin()
	var out struct {
		HasData *domain.WireFlag `json:"synType"`
	}
	if err = c.call(ctx, "file/2/files/synchronous/start", device{name}, true, pin, &out); err != nil {
		return err
	}
	succeeded := false
	defer func() {
		// Cancellation (or panic unwinding) must still release the remote lock;
		// the independent deadline bounds the extra time spent cleaning up.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
		defer cancel()
		flag := "N"
		if succeeded {
			flag = "Y"
		}
		finish := struct {
			device
			Flag string `json:"flag"`
		}{device{name}, flag}
		release := c.call(cleanup, "file/2/files/synchronous/end", finish, true, pin, nil)
		// A release failure is secondary when the body already failed.
		if err == nil {
			err = release
		}
	}()
	hasData := out.HasData == nil || bool(*out.HasData)
	err = body(ctx, hasData)
	succeeded = err == nil
	return err
}
