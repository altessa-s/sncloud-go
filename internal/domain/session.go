// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

// Secret names the only values persisted by the protocol.
type Secret string

// Stable store keys; a password is deliberately absent.
const (
	// AccessToken is the persisted bearer credential.
	AccessToken Secret = "accessToken"
	// Account is the persisted account spelling.
	Account Secret = "account"
	// ClientID is the persistent installation identifier, retained across logouts.
	ClientID Secret = "clientID"
)

// LoginOutcome distinguishes a completed login from an emailed challenge.
type LoginOutcome string

// Login outcomes.
const (
	// LoggedIn means the token and account were stored successfully.
	LoggedIn LoginOutcome = "loggedIn"
	// VerificationRequired means an emailed code must complete the login.
	VerificationRequired LoginOutcome = "verificationRequired"
)
