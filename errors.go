// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud

import (
	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Error is implemented by every protocol and transport failure returned by this
// package. The set of kinds is closed: adding one is a breaking change.
//
// Every kind has a matching Err... sentinel matched with [errors.Is],
// including kinds represented by pointer types below. Callers that need a
// failure's structured fields use [errors.As] or [errors.AsType]. Context
// cancellation ([context.Canceled] and [context.DeadlineExceeded]) passes
// through unchanged outside Error. Secret-store, local file and constructor
// errors can also fall outside this set.
type Error = domain.Error

// Kind names one case of [Error]. It is a string type whose values are the
// Kind constants below.
type Kind = domain.Kind

// KindOf extracts the [Kind] from err across wrapped errors, or returns "" when
// err contains no protocol [Error].
func KindOf(err error) Kind { return domain.KindOf(err) }

// KindNotLoggedIn identifies a call attempted without a usable local token.
const KindNotLoggedIn = domain.KindNotLoggedIn

// ErrNotLoggedIn reports that no usable [AccessToken] is stored locally.
var ErrNotLoggedIn = domain.ErrNotLoggedIn

// KindSessionExpired identifies an expired or server-refused session.
const KindSessionExpired = domain.KindSessionExpired

// ErrSessionExpired reports an expired or refused token (JWT within five
// minutes of expiry, API HTTP 401, envelope E0712 or E0085, or a change-feed
// refusal whose message names a token).
var ErrSessionExpired = domain.ErrSessionExpired

// KindInvalidCredentials identifies rejected login credentials.
const KindInvalidCredentials = domain.KindInvalidCredentials

// ErrInvalidCredentials reports a rejected account or password (E0019, E0708, E0711).
var ErrInvalidCredentials = domain.ErrInvalidCredentials

// KindAccountLocked identifies a locked account.
const KindAccountLocked = domain.KindAccountLocked

// ErrAccountLocked matches [AccountLockedError] with [errors.Is].
var ErrAccountLocked = domain.ErrAccountLocked

// KindVerificationRequired identifies a challenge requiring an emailed code.
const KindVerificationRequired = domain.KindVerificationRequired

// ErrVerificationRequired matches envelope E1760 outside [Client.LogIn] (which
// reports [VerificationRequired] as a [LoginOutcome] instead).
var ErrVerificationRequired = domain.ErrVerificationRequired

// KindInvalidVerificationCode identifies a rejected verification code.
const KindInvalidVerificationCode = domain.KindInvalidVerificationCode

// ErrInvalidVerificationCode reports a wrong or expired emailed code (E1756, E0101).
var ErrInvalidVerificationCode = domain.ErrInvalidVerificationCode

// KindNoVerificationPending identifies verification calls without a pending login.
const KindNoVerificationPending = domain.KindNoVerificationPending

// ErrNoVerificationPending reports [Client.SendVerificationCode] or
// [Client.Verify] without a pending email verification challenge.
var ErrNoVerificationPending = domain.ErrNoVerificationPending

// KindAlreadyExists identifies a duplicate folder, task, digest group, or tag.
const KindAlreadyExists = domain.KindAlreadyExists

// ErrAlreadyExists reports a name or identifier collision (E0007 on folder
// creation, E0322, E0335, E0337, E0338).
var ErrAlreadyExists = domain.ErrAlreadyExists

// KindNotFound identifies a missing remote resource.
const KindNotFound = domain.KindNotFound

// ErrNotFound matches [NotFoundError] with [errors.Is].
var ErrNotFound = domain.ErrNotFound

// KindSyncLocked identifies a held device synchronization lock.
const KindSyncLocked = domain.KindSyncLocked

// ErrSyncLocked reports that the account sync lock is already held (E0078,
// E0079, E0301, or a nested [Device.WithSyncLock] call on the same [Client]).
var ErrSyncLocked = domain.ErrSyncLocked

// KindRejected identifies an unmapped service refusal or busy guard.
const KindRejected = domain.KindRejected

// ErrRejected matches [RejectedError] with [errors.Is].
var ErrRejected = domain.ErrRejected

// KindMalformedResponse identifies an invalid wire payload or local input format.
const KindMalformedResponse = domain.KindMalformedResponse

// ErrMalformedResponse matches [MalformedResponseError] with [errors.Is].
var ErrMalformedResponse = domain.ErrMalformedResponse

// KindChecksumMismatch identifies a content or chunk MD5 mismatch.
const KindChecksumMismatch = domain.KindChecksumMismatch

// ErrChecksumMismatch matches [ChecksumMismatchError] with [errors.Is].
var ErrChecksumMismatch = domain.ErrChecksumMismatch

// KindRateLimited identifies HTTP 429 rate limiting.
const KindRateLimited = domain.KindRateLimited

// ErrRateLimited matches [RateLimitedError] with [errors.Is].
var ErrRateLimited = domain.ErrRateLimited

// KindHTTP identifies an unmapped non-2xx HTTP status.
const KindHTTP = domain.KindHTTP

// ErrHTTP matches [HTTPError] with [errors.Is].
var ErrHTTP = domain.ErrHTTP

// KindTransport identifies a classified network failure.
const KindTransport = domain.KindTransport

// ErrTransport matches [TransportError] with [errors.Is].
var ErrTransport = domain.ErrTransport

// KindOversizedResponse identifies a response or download exceeding its limit.
const KindOversizedResponse = domain.KindOversizedResponse

// ErrOversizedResponse matches [OversizedResponseError] with [errors.Is].
var ErrOversizedResponse = domain.ErrOversizedResponse

// AccountLockedError reports an account lock (E0709, E0710, E0061). Matches
// [ErrAccountLocked] with [errors.Is]. Field: Code string.
type AccountLockedError = domain.AccountLockedError

// NotFoundError reports a missing resource (E0081, E0308, E0311, E0318, E0320,
// E0321, E0328, E0329, E0336, E0339, E0340). Matches [ErrNotFound] with
// [errors.Is]. Field: Code string.
type NotFoundError = domain.NotFoundError

// RejectedError reports an unmapped envelope code and server message, or
// [DigestBusyCode] on overlapping updates or deletions of the same digest. Matches [ErrRejected]
// with [errors.Is]. Fields: Code, Message string.
type RejectedError = domain.RejectedError

// MalformedResponseError reports a strict JSON or protocol shape violation, or
// a rejected local identifier, tag, or byte range. Matches
// [ErrMalformedResponse] with [errors.Is]. Field: What string.
type MalformedResponseError = domain.MalformedResponseError

// ChecksumMismatchError reports that downloaded or chunk bytes hashed to Actual
// rather than Expected (lowercase hex MD5). Matches [ErrChecksumMismatch] with
// [errors.Is]. Fields: Expected, Actual string.
type ChecksumMismatchError = domain.ChecksumMismatchError

// RateLimitedError reports HTTP 429 rate limiting. RetryAfter is in seconds and
// valid when HasRetryAfter is true. Matches [ErrRateLimited] with [errors.Is].
// Fields: RetryAfter float64, HasRetryAfter bool.
type RateLimitedError = domain.RateLimitedError

// HTTPError reports a non-2xx HTTP response not mapped to a more specific kind.
// Matches [ErrHTTP] with [errors.Is]. Fields: Status int, Path string.
type HTTPError = domain.HTTPError

// TransportError reports a network failure with signed URLs removed. Matches
// [ErrTransport] with [errors.Is]. Field: Reason [TransportReason].
type TransportError = domain.TransportError

// OversizedResponseError reports a declared or streamed response exceeding its
// byte ceiling. Matches [ErrOversizedResponse] with [errors.Is].
// Fields: Bytes, Limit int64.
type OversizedResponseError = domain.OversizedResponseError

// TransportReason classifies a [KindTransport] failure without retaining URLs.
// It is a string type whose values are the Reason constants below.
type TransportReason = domain.TransportReason

// ReasonTimedOut ("timedOut") classifies a transport-owned timer (response
// headers, idle body read, or WebSocket open handshake) expiring.
const ReasonTimedOut = domain.ReasonTimedOut

// ReasonCannotFindHost ("cannotFindHost") classifies a DNS response reporting
// that the hostname does not exist.
const ReasonCannotFindHost = domain.ReasonCannotFindHost

// ReasonCannotConnectToHost classifies a refused TCP connection.
const ReasonCannotConnectToHost = domain.ReasonCannotConnectToHost

// ReasonConnectionLost ("connectionLost") classifies a connection reset,
// broken pipe, or unexpected EOF while exchanging bytes.
const ReasonConnectionLost = domain.ReasonConnectionLost

// ReasonTLS classifies a failed TLS handshake or certificate check.
const ReasonTLS = domain.ReasonTLS

// ReasonOther ("other") classifies any transport failure not covered by a
// more specific reason, with its URL stripped.
const ReasonOther = domain.ReasonOther

// ReasonDNSLookupFailed ("dnsLookupFailed") classifies a DNS resolver failure
// other than a non-existent host.
const ReasonDNSLookupFailed = domain.ReasonDNSLookupFailed

// ReasonNotConnectedToInternet ("notConnectedToInternet") classifies an
// unreachable network or host.
const ReasonNotConnectedToInternet = domain.ReasonNotConnectedToInternet
