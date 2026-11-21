// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package domain holds protocol values without I/O.
package domain

import (
	"errors"
	"fmt"
)

// Kind identifies a protocol failure independently of its payload.
type Kind string

// Error is the common protocol error surface.
type Error interface {
	error
	Kind() Kind
}
type sentinel Kind

// Error describes a payload-free protocol failure.
func (e sentinel) Error() string { return "sncloud: " + string(e) }

// Kind identifies the failure without inspecting its text.
func (e sentinel) Kind() Kind { return Kind(e) }

// Protocol error kinds.
const (
	// KindNotLoggedIn identifies no usable login.
	KindNotLoggedIn Kind = "notLoggedIn"
	// KindSessionExpired identifies an expired or refused session.
	KindSessionExpired Kind = "sessionExpired"
	// KindInvalidCredentials identifies invalid account credentials.
	KindInvalidCredentials Kind = "invalidCredentials"
	// KindAccountLocked identifies an account lock.
	KindAccountLocked Kind = "accountLocked"
	// KindVerificationRequired identifies a required emailed challenge.
	KindVerificationRequired Kind = "verificationRequired"
	// KindInvalidVerificationCode identifies a refused verification code.
	KindInvalidVerificationCode Kind = "invalidVerificationCode"
	// KindNoVerificationPending identifies no pending emailed challenge.
	KindNoVerificationPending Kind = "noVerificationPending"
	// KindAlreadyExists identifies an existing destination.
	KindAlreadyExists Kind = "alreadyExists"
	// KindNotFound identifies a missing resource.
	KindNotFound Kind = "notFound"
	// KindSyncLocked identifies a held device sync lock.
	KindSyncLocked Kind = "syncLocked"
	// KindRejected identifies an unrecognized service rejection.
	KindRejected Kind = "rejected"
	// KindMalformedResponse identifies an unreadable protocol response.
	KindMalformedResponse Kind = "malformedResponse"
	// KindChecksumMismatch identifies a mismatched content checksum.
	KindChecksumMismatch Kind = "checksumMismatch"
	// KindRateLimited identifies a server-directed wait.
	KindRateLimited Kind = "rateLimited"
	// KindHTTP identifies an unsuccessful HTTP status.
	KindHTTP Kind = "http"
	// KindTransport identifies a network failure.
	KindTransport Kind = "transport"
	// KindOversizedResponse identifies a response exceeding its byte ceiling.
	KindOversizedResponse Kind = "oversizedResponse"
)

// Sentinel kinds can be matched with errors.Is, including typed failures.
var (
	// ErrNotLoggedIn matches no usable login with errors.Is.
	ErrNotLoggedIn Error = sentinel(KindNotLoggedIn)
	// ErrSessionExpired matches an expired or refused session with errors.Is.
	ErrSessionExpired Error = sentinel(KindSessionExpired)
	// ErrInvalidCredentials matches invalid account credentials with errors.Is.
	ErrInvalidCredentials Error = sentinel(KindInvalidCredentials)
	// ErrAccountLocked matches an account lock with errors.Is.
	ErrAccountLocked Error = sentinel(KindAccountLocked)
	// ErrVerificationRequired matches a required emailed challenge with errors.Is.
	ErrVerificationRequired Error = sentinel(KindVerificationRequired)
	// ErrInvalidVerificationCode matches a refused verification code with errors.Is.
	ErrInvalidVerificationCode Error = sentinel(KindInvalidVerificationCode)
	// ErrNoVerificationPending matches no pending emailed challenge with errors.Is.
	ErrNoVerificationPending Error = sentinel(KindNoVerificationPending)
	// ErrAlreadyExists matches an existing destination with errors.Is.
	ErrAlreadyExists Error = sentinel(KindAlreadyExists)
	// ErrNotFound matches a missing resource with errors.Is.
	ErrNotFound Error = sentinel(KindNotFound)
	// ErrSyncLocked matches a held device sync lock with errors.Is.
	ErrSyncLocked Error = sentinel(KindSyncLocked)
	// ErrRejected matches an unrecognized service rejection with errors.Is.
	ErrRejected Error = sentinel(KindRejected)
	// ErrMalformedResponse matches an unreadable protocol response with errors.Is.
	ErrMalformedResponse Error = sentinel(KindMalformedResponse)
	// ErrChecksumMismatch matches a mismatched content checksum with errors.Is.
	ErrChecksumMismatch Error = sentinel(KindChecksumMismatch)
	// ErrRateLimited matches a server-directed wait with errors.Is.
	ErrRateLimited Error = sentinel(KindRateLimited)
	// ErrHTTP matches an unsuccessful HTTP status with errors.Is.
	ErrHTTP Error = sentinel(KindHTTP)
	// ErrTransport matches a network failure with errors.Is.
	ErrTransport Error = sentinel(KindTransport)
	// ErrOversizedResponse matches a response exceeding its byte ceiling with errors.Is.
	ErrOversizedResponse Error = sentinel(KindOversizedResponse)
)

// AccountLockedError carries the details of a accountLocked failure.
type AccountLockedError struct{ Code string }

// Kind implements Error.
func (*AccountLockedError) Kind() Kind { return KindAccountLocked }

// Is matches the sentinel kind.
func (*AccountLockedError) Is(target error) bool { return target == ErrAccountLocked }

// Error describes the failure without request credentials.
func (e *AccountLockedError) Error() string { return "sncloud: " + "account locked: " + e.Code }

// NotFoundError carries the details of a notFound failure.
type NotFoundError struct{ Code string }

// Kind implements Error.
func (*NotFoundError) Kind() Kind { return KindNotFound }

// Is matches the sentinel kind.
func (*NotFoundError) Is(target error) bool { return target == ErrNotFound }

// Error describes the failure without request credentials.
func (e *NotFoundError) Error() string { return "sncloud: " + "not found: " + e.Code }

// RejectedError carries the details of a rejected failure.
type RejectedError struct{ Code, Message string }

// Kind implements Error.
func (*RejectedError) Kind() Kind { return KindRejected }

// Is matches the sentinel kind.
func (*RejectedError) Is(target error) bool { return target == ErrRejected }

// Error describes the failure without request credentials.
func (e *RejectedError) Error() string { return "sncloud: " + e.Code + ": " + e.Message }

// MalformedResponseError carries the details of a malformedResponse failure.
type MalformedResponseError struct{ What string }

// Kind implements Error.
func (*MalformedResponseError) Kind() Kind { return KindMalformedResponse }

// Is matches the sentinel kind.
func (*MalformedResponseError) Is(target error) bool { return target == ErrMalformedResponse }

// Error describes the failure without request credentials.
func (e *MalformedResponseError) Error() string { return "sncloud: " + "malformed response: " + e.What }

// ChecksumMismatchError carries the details of a checksumMismatch failure.
type ChecksumMismatchError struct{ Expected, Actual string }

// Kind implements Error.
func (*ChecksumMismatchError) Kind() Kind { return KindChecksumMismatch }

// Is matches the sentinel kind.
func (*ChecksumMismatchError) Is(target error) bool { return target == ErrChecksumMismatch }

// Error describes the failure without request credentials.
func (e *ChecksumMismatchError) Error() string {
	return "sncloud: " + "checksum mismatch: expected " + e.Expected + ", got " + e.Actual
}

// RateLimitedError carries the details of a rateLimited failure.
type RateLimitedError struct {
	RetryAfter    float64
	HasRetryAfter bool
}

// Kind implements Error.
func (*RateLimitedError) Kind() Kind { return KindRateLimited }

// Is matches the sentinel kind.
func (*RateLimitedError) Is(target error) bool { return target == ErrRateLimited }

// Error describes the failure without request credentials.
func (e *RateLimitedError) Error() string { return "sncloud: " + "rate limited" }

// HTTPError carries the details of a http failure.
type HTTPError struct {
	Status int
	Path   string
}

// Kind implements Error.
func (*HTTPError) Kind() Kind { return KindHTTP }

// Is matches the sentinel kind.
func (*HTTPError) Is(target error) bool { return target == ErrHTTP }

// Error describes the failure without request credentials.
func (e *HTTPError) Error() string { return "sncloud: " + fmt.Sprintf("http %d %s", e.Status, e.Path) }

// TransportError carries the details of a transport failure.
type TransportError struct{ Reason TransportReason }

// Kind implements Error.
func (*TransportError) Kind() Kind { return KindTransport }

// Is matches the sentinel kind.
func (*TransportError) Is(target error) bool { return target == ErrTransport }

// Error describes the failure without request credentials.
func (e *TransportError) Error() string { return "sncloud: " + "transport: " + string(e.Reason) }

// OversizedResponseError carries the details of a oversizedResponse failure.
type OversizedResponseError struct{ Bytes, Limit int64 }

// Kind implements Error.
func (*OversizedResponseError) Kind() Kind { return KindOversizedResponse }

// Is matches the sentinel kind.
func (*OversizedResponseError) Is(target error) bool { return target == ErrOversizedResponse }

// Error describes the failure without request credentials.
func (e *OversizedResponseError) Error() string {
	return "sncloud: " + fmt.Sprintf("oversized response: %d bytes over limit %d", e.Bytes, e.Limit)
}

// TransportReason classifies failures without retaining signed URLs.
type TransportReason string

// Network failure categories.
const (
	// ReasonTimedOut classifies a transport-owned timer expiring.
	ReasonTimedOut TransportReason = "timedOut"
	// ReasonCannotFindHost classifies a DNS name that does not exist.
	ReasonCannotFindHost TransportReason = "cannotFindHost"
	// ReasonCannotConnectToHost classifies a refused connection.
	ReasonCannotConnectToHost TransportReason = "cannotConnectToHost"
	// ReasonConnectionLost classifies a connection lost while exchanging bytes.
	ReasonConnectionLost TransportReason = "connectionLost"
	// ReasonTLS classifies a failed TLS handshake or certificate check.
	ReasonTLS TransportReason = "secureConnectionFailed"
	// ReasonDNSLookupFailed classifies a resolver failure other than a missing name.
	ReasonDNSLookupFailed TransportReason = "dnsLookupFailed"
	// ReasonNotConnectedToInternet classifies an unreachable host or network.
	ReasonNotConnectedToInternet TransportReason = "notConnectedToInternet"
	// ReasonOther classifies an unrecognized transport failure, without its raw URL.
	ReasonOther TransportReason = "other"
)

// KindOf recovers the kind through wrapped errors.
func KindOf(err error) Kind {
	if e, ok := errors.AsType[Error](err); ok {
		return e.Kind()
	}
	return ""
}
