// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud

import (
	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Endpoints names the app, web, and change-feed origins.
//
// Public clouds (PrivateCloud == false) require HTTPS except on loopback
// hosts (localhost or a loopback IP address). Setting PrivateCloud allows
// cleartext HTTP and disables the built-in SSRF dial guard for every host,
// including public ones; the caller vouches for those destinations.
type Endpoints = domain.Endpoints

// AccountPage is the official account-management website (https://cloud.supernote.com).
const AccountPage = domain.AccountPage

// Official returns the global Supernote Cloud configuration (viewer.supernote.com,
// cloud.supernote.com, app-wss-file.supernote.com, XSRF enabled, dial guard on).
func Official() Endpoints { return domain.Official() }

// PrivateCloud shares host across the app, web, and change-feed origins and
// sets PrivateCloud to true, allowing cleartext HTTP for servers without TLS
// certificates and disabling the built-in SSRF dial guard for all hosts. The
// caller vouches for the destination; validation runs in [Endpoints].Validate
// or [New].
func PrivateCloud(host string, usesXSRF bool) Endpoints { return domain.PrivateCloud(host, usesXSRF) }

// EndpointsFromEnvironment starts from [Official] and applies opt-in overrides
// from env without reading the process environment:
//   - SNCLOUD_HOST replaces AppHost, WebHost, and SocketHost when it is an
//     https URL (or an http URL on localhost, 127.0.0.1, or ::1) without
//     userinfo, leaving PrivateCloud false so the SSRF dial guard stays active;
//   - SNCLOUD_XSRF set to "off", "0", "false", or "no" (case-insensitive)
//     disables WebUsesXSRF.
func EndpointsFromEnvironment(env map[string]string) Endpoints {
	return domain.EndpointsFromEnvironment(env)
}

// Secret is a string key in the caller's [SecretStore]: [AccessToken],
// [Account], or [ClientID]. The raw string values are the persistence contract.
type Secret = domain.Secret

// Persistent secret names in [SecretStore].
const (
	// AccessToken ("accessToken") is the persisted bearer credential.
	AccessToken = domain.AccessToken
	// Account ("account") is the persisted account identifier.
	Account = domain.Account
	// ClientID ("clientID") is the persistent installation UUID, retained across logouts.
	ClientID = domain.ClientID
)

// LoginOutcome indicates whether [Client.LogIn] completed immediately or needs
// an emailed verification code.
type LoginOutcome = domain.LoginOutcome

// Login outcomes returned by [Client.LogIn].
const (
	// LoggedIn ("loggedIn") means the token and account were stored.
	LoggedIn = domain.LoggedIn
	// VerificationRequired ("verificationRequired") means an emailed code must
	// complete the login via [Client.SendVerificationCode] and [Client.Verify].
	VerificationRequired = domain.VerificationRequired
)

// DeviceIdentity names this installation across app and change-feed requests.
//
// Platform and ClientID identify the installation. [NewDeviceIdentity] normalizes
// their values; the client constructs protocol headers and socket parameters.
type DeviceIdentity = domain.DeviceIdentity

// NewDeviceIdentity normalizes clientID to lowercase; an empty platform selects
// the current operating system's protocol spelling ("MACOS" on darwin,
// uppercase GOOS elsewhere).
func NewDeviceIdentity(platform, clientID string) DeviceIdentity {
	return domain.NewDeviceIdentity(platform, clientID)
}

// Protocol header names and the browser User-Agent string.
const (
	// AccessTokenHeader ("x-access-token") carries the bearer token on API requests.
	AccessTokenHeader = domain.AccessTokenHeader
	// XSRFHeader ("X-XSRF-TOKEN") carries the current browser anti-forgery token.
	XSRFHeader = domain.XSRFHeader
	// WebUserAgent identifies browser-origin requests on the web host.
	WebUserAgent = domain.WebUserAgent
)

// FileID is a signed 64-bit snowflake identifier. It
// marshals as a JSON string, unmarshals from either a JSON string or integer
// without float64 precision loss, and formats in decimal via String().
type FileID = domain.FileID

// RootFileID (0) identifies the top-level directory.
const RootFileID = domain.RootFileID

// RetryPolicy controls total attempts and capped exponential backoff.
//
// Read calls retry [ErrTransport], 5xx [HTTPError], and [ErrRateLimited];
// state-changing calls retry only [ErrRateLimited]. Server Retry-After hints
// are clamped to MaxDelay without jitter.
type RetryPolicy = domain.RetryPolicy

// DefaultRetryPolicy returns 3 total attempts, 300ms BaseDelay, 20s MaxDelay,
// 3x Multiplier, and 0.2 (±20%) symmetric Jitter.
func DefaultRetryPolicy() RetryPolicy { return domain.DefaultRetryPolicy() }
