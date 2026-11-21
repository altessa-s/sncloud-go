// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package sncloud is an unofficial client for the reverse-engineered Supernote
// Cloud protocol, including self-hosted Ratta Supernote Private Cloud servers.
// It provides no CLI and leaves credential storage and local file layout to the caller.
//
// [New] takes a concurrency-safe [SecretStore]. The store holds [AccessToken],
// [Account] and the installation's [ClientID], never the password. Login sends
// SHA-256(MD5-hex(password) + salt); Go strings cannot be reliably erased.
// [Client.LogIn] returns [VerificationRequired] when an emailed code is needed:
// follow it with [Client.SendVerificationCode] and [Client.Verify]. There is no
// token refresh or clock-skew correction. [Client.IsLoggedIn] checks local
// usability with a five-minute expiry margin; [Client.Expiry] reads the stored
// claim without that margin or signature verification.
//
// # Views
//
// Five views share the session and network services:
//   - [Client.Files]: identifier-based browsing, recycle bin, uploads, downloads
//     and PDF/PNG export.
//   - [Client.Device]: path-based tablet storage, ranged downloads and
//     [Device.WithSyncLock] for account synchronization.
//   - [Client.Tasks]: to-do lists and tasks, including recurring occurrences
//     returned in task listings.
//   - [Client.Digests]: groups, passages, tags, fingerprints and handwritten comments.
//   - [Client.Feed]: a single connection through [Feed.Changes] or a reconnecting
//     iterator through [Feed.Watch]. Every connection begins with
//     [ChangeConnected]: reconcile over REST, since disconnected changes are not
//     replayed. Announcements may repeat and are signals to read again.
//
// Successful downloads return temporary files the caller must remove. Full-file
// MD5 is checked when supplied; ranges check Content-Range and length instead.
// Upload authentication steps stay pinned to the login that started them.
// [Client.Close] releases idle connections; cancel the feed context or break
// iteration to stop an active socket.
//
// # Network and diagnostics
//
// [Official] selects the global service. [PrivateCloud] allows HTTP and disables
// the built-in private-address dial guard: the caller vouches for the hosts.
// [WithRoundTripper] replaces the HTTP and WebSocket transports and takes over
// their dial, TLS and connection-bound policies. Endpoint overrides are opt-in
// through [EndpointsFromEnvironment].
//
// Reads retry transport failures, HTTP 5xx and rate limits. Mutations retry only
// rate limits; replayable byte transfers use the read policy. The default is
// three attempts with capped exponential backoff. Use context deadlines to
// bound whole operations; the built-in transport bounds stalled headers and
// in-memory body reads, not the total duration of a moving transfer.
//
// SNCLOUD_WIRE_LOG=headers or bodies enables debug diagnostics through the
// configured logger. Query values and credential headers are removed; secret
// JSON fields are masked and credential-exchange bodies are never printed.
// Other body content, including document metadata, may appear in bodies mode.
//
// # Errors
//
// Protocol and classified network failures form the closed [Error] set. Every
// kind has an Err sentinel for errors.Is; payload errors also support errors.As.
// [KindOf] finds a kind through wrapping. Context, secret-store, local file and
// constructor errors can fall outside this set.
//
//   - [ErrNotLoggedIn], [ErrSessionExpired]: absent, expired or refused credentials.
//   - [ErrInvalidCredentials], [ErrAccountLocked], [ErrVerificationRequired],
//     [ErrInvalidVerificationCode], [ErrNoVerificationPending]: login and challenges.
//   - [ErrAlreadyExists], [ErrNotFound], [ErrSyncLocked], [ErrRejected]: service
//     refusals; unknown codes retain their message in [RejectedError].
//   - [ErrMalformedResponse], [ErrChecksumMismatch], [ErrOversizedResponse]:
//     malformed input or responses, integrity failures and byte ceilings.
//   - [ErrRateLimited], [ErrHTTP], [ErrTransport]: rate limits, other HTTP
//     failures and network failures classified by [TransportReason].
package sncloud
