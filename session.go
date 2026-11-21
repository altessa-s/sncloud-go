// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud

import (
	"context"
	"time"
)

// LogIn authenticates an email or telephone account. Pass a nil countryCode for
// email accounts or the numeric telephone prefix for phone accounts. When the
// server asks for an emailed code (E1760), LogIn returns ([VerificationRequired], nil)
// rather than an error so the caller can run [Client.SendVerificationCode] and
// [Client.Verify]. The password is never stored or logged; only its salted
// digest leaves the process. Go strings cannot be reliably erased from memory.
func (c *Client) LogIn(ctx context.Context, account, password string, countryCode *int) (LoginOutcome, error) {
	return c.session.LogIn(ctx, account, password, countryCode)
}

// SendVerificationCode emails a one-time code to the email address left pending
// by a [VerificationRequired] outcome from [Client.LogIn]. It returns
// [ErrNoVerificationPending] when no email login challenge is pending.
func (c *Client) SendVerificationCode(ctx context.Context) error {
	return c.session.SendVerificationCode(ctx)
}

// Verify checks and exchanges the emailed code requested by
// [Client.SendVerificationCode], storing the resulting token and account.
// Calling Verify before SendVerificationCode returns [ErrNoVerificationPending];
// a rejected code returns [ErrInvalidVerificationCode] while keeping the
// challenge pending for another attempt.
func (c *Client) Verify(ctx context.Context, code string) error { return c.session.Verify(ctx, code) }

// LogOut ends the remote session (user/logout) and clears the local
// [AccessToken], [Account], and browser cookies while retaining [ClientID].
// An already-expired remote token ([ErrSessionExpired]) is treated as logged out;
// local cleanup is attempted even if the network request fails, unless a
// newer login or Forget superseded it. Store deletion errors are returned.
func (c *Client) LogOut(ctx context.Context) error { return c.session.LogOut(ctx) }

// Forget removes the local [AccessToken], [Account], and browser cookies
// without contacting the server, retaining [ClientID] and advancing the login
// generation so later authenticated steps of an in-flight upload cannot use
// that login. Store deletion errors are returned; active requests are not canceled.
func (c *Client) Forget() error { return c.session.Forget() }

// Identity returns the persistent [DeviceIdentity] backed by [ClientID] in the
// [SecretStore], generating and storing a fresh UUIDv4 on first use.
func (c *Client) Identity() (DeviceIdentity, error) { return c.session.Identity() }

// IsLoggedIn reports whether a non-empty [AccessToken] is stored and has more
// than five minutes remaining before any JWT exp claim, without contacting the
// service. Opaque private-cloud tokens carry no local expiry and report true.
// After a failed credential write it returns false and [ErrNotLoggedIn] until
// a login succeeds. It answers from this Client's cached token when it has one:
// a login or logout made through another Client or process over the same store
// shows here only after this Client's next authenticated call. It is not a way
// to synchronize Clients that share a store.
func (c *Client) IsLoggedIn() (bool, error) { return c.session.IsLoggedIn() }

// Account returns the account identifier stored by the last completed login,
// or an empty string when absent. Token expiry does not clear this value.
// A failed store write may leave an uncertain account value; see [SecretStore].
func (c *Client) Account() (string, error) { return c.session.Account() }

// Expiry decodes the unverified JWT exp claim (seconds, or milliseconds above
// 1e11) from the stored [AccessToken], including within the five-minute request
// margin and after expiration. Absent or opaque tokens return (nil, nil).
// Like [Client.IsLoggedIn] it answers from the cached token when there is one,
// so with a shared store it can describe a token another Client has replaced.
func (c *Client) Expiry() (*time.Time, error) { return c.session.Expiry() }
