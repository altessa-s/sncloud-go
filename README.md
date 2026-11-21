# sncloud-go

An unofficial Go client for the reverse-engineered Supernote Cloud protocol: login, files, tablet storage, to-do lists, digests and the change feed.
It supports the global official cloud and self-hosted Ratta Supernote Private Cloud servers.

The module speaks the protocol. It supplies no CLI, chooses no local library layout and takes a credential store from the caller.
Downloads return temporary files whose cleanup belongs to the caller. Ordinary tests and examples use synthetic accounts and loopback HTTP and WebSocket servers, with no external service access. The opt-in `live` tests reach the server named by their environment.

## Requirements

Go 1.27.0 or newer is required by [go.mod](go.mod). Its `toolchain go1.27.4` line suggests that toolchain when this is the main module; it does not force that patch version on consumers. With `GOTOOLCHAIN=auto`, Go can download a required or suggested toolchain.

| Module | What this client takes from it |
|---|---|
| go-atlas-core | Retry driver, clock and test leak checks |
| go-atlas-net | Pooled HTTP clients, per-host circuit breakers, transport-error redaction and the SSRF dial guard |
| go-tlsutils | Client TLS configuration |
| coder/websocket | WebSocket transport beneath Engine.IO 3 / Socket.IO |
| go-optgen | Generated options; pinned as a development tool |

## Installing

```sh
go get github.com/altessa-s/sncloud-go@latest
```

Import the module as `sncloud`. The root package is the public API.

## Usage

`New(store, opts...)` makes no network requests. A store is required; nil options are skipped. `Close` releases idle connections without logging
out or interrupting calls. Cancel a feed's context or break its iterator to close its socket.

This function logs in with an email address, asks the caller for an emailed code if required, lists the root and downloads the first file.
The returned path belongs to the caller, who must eventually remove it with `os.Remove`.

```go
package example

import (
    "context"
    "fmt"

    sncloud "github.com/altessa-s/sncloud-go"
)

func downloadFirst(ctx context.Context, store sncloud.SecretStore, account, password string,
    promptCode func(context.Context) (string, error),
) (string, error) {
    c, err := sncloud.New(store)
    if err != nil {
        return "", err
    }
    defer c.Close()
    outcome, err := c.LogIn(ctx, account, password, nil)
    if err != nil {
        return "", err
    }
    if outcome == sncloud.VerificationRequired {
        if err := c.SendVerificationCode(ctx); err != nil {
            return "", err
        }
        code, err := promptCode(ctx)
        if err != nil {
            return "", err
        }
        if err := c.Verify(ctx, code); err != nil {
            return "", err
        }
    }
    entries, err := c.Files().List(ctx, sncloud.RootFileID, sncloud.SortName, true)
    if err != nil {
        return "", err
    }
    for _, entry := range entries {
        if !entry.IsFolder {
            return c.Files().Download(ctx, entry.ID, entry.Size, nil)
        }
    }
    return "", fmt.Errorf("no files in the root folder")
}
```

For a telephone login, pass the numeric country prefix as the fourth argument to `LogIn`. The emailed-code continuation requires an email account.

### Five views

Each accessor returns a cheap view over the same session. Views and the client can be used concurrently.

| View | Use |
|---|---|
| `c.Files()` | Identifier-based browsing, search, folder changes, recycle bin, transfers and PDF/PNG export |
| `c.Device()` | Path-based tablet tree, transfers, inclusive byte ranges, permanent deletion and `WithSyncLock` |
| `c.Tasks()` | Lists, tasks and recurring occurrences returned within task listings |
| `c.Digests()` | Groups, passages, tags, fingerprints and handwritten `.mark` comments |
| `c.Feed()` | File, task and digest announcements |

These functions take an authenticated client:

```go
package views

import (
    "context"
    "fmt"

    sncloud "github.com/altessa-s/sncloud-go"
)

func upload(ctx context.Context, c *sncloud.Client) error {
    return c.Files().UploadBytes(ctx, []byte("synthetic content"), "Sample.txt", sncloud.RootFileID)
}

func listTablet(ctx context.Context, c *sncloud.Client) error {
    entries, err := c.Device().List(ctx, "/", true)
    if err != nil {
        return err
    }
    for _, entry := range entries {
        fmt.Println(entry.Path, entry.ContentHash)
    }
    return nil
}

func addTask(ctx context.Context, c *sncloud.Client) error {
    list, err := c.Tasks().CreateList(ctx, "Reading", "")
    if err != nil {
        return err
    }
    _, err = c.Tasks().Create(ctx, sncloud.Task{ListID: &list.ID, Title: "Review notes"})
    return err
}

func readDigests(ctx context.Context, c *sncloud.Client) error {
    digests, err := c.Digests().List(ctx, "") // Empty group means all groups.
    if err != nil {
        return err
    }
    for _, digest := range digests {
        if digest.Content != nil {
            fmt.Println(*digest.Content)
        }
    }
    return nil
}

func watch(ctx context.Context, c *sncloud.Client, reconcile func(context.Context) error) error {
    for _, err := range c.Feed().Watch(ctx) {
        if err != nil {
            return err
        }
        // Re-read on ChangeConnected and on every announcement.
        if err := reconcile(ctx); err != nil {
            return err
        }
    }
    return nil
}
```

File listings return `ErrMalformedResponse` if still incomplete after 10,000 pages. Task, digest and digest-group listings retain the last
observation of each ID before filtering deletions, so a later deletion supersedes an earlier live row.

`Tasks.Update` needs the full task read from a listing: omitted mutable values can be cleared by the server. Keep the IDs returned by task and
list creation, since the service can change a colliding caller-chosen ID. `Digests.Update` is a partial update using non-nil draft fields;
it reads existing fields when needed to merge an author or compute a fingerprint. A supplied `MD5Hash` overrides computation.

`Files.Delete` moves entries into the recycle bin; `Device.Delete` permanently deletes them. `Device.WithSyncLock` releases an acquired lock
even after cancellation, with a separate five-second deadline. Overlapping scopes on the same client and device fail with `ErrSyncLocked`.

Full downloads check MD5 when supplied by the service and enforce a byte ceiling. A nil web download size selects `DownloadLimit` (2 GiB);
a supplied size is a ceiling, not an assertion of exact length. HTTP content length is checked when present. Device ranges check HTTP 206,
`Content-Range` and byte count instead of the whole-file hash. Uploads use S3 PUT/multipart or private-cloud forms as the upload slot directs.

### Session and store

The caller implements `SecretStore` with these methods:

```go
package storage

import sncloud "github.com/altessa-s/sncloud-go"

type Store interface {
    Secret(sncloud.Secret) (string, error)
    Keep(value string, key sncloud.Secret) error
    Discard(sncloud.Secret) error
}
```

The store must be concurrency-safe and must not call back into the client. `Secret` returns `("", nil)` for an absent key.
The three keys are `AccessToken` (`accessToken`), `Account` (`account`) and `ClientID` (`clientID`). The installation ID is generated on first
use and survives logout. The password is never passed to the store or logged: login sends `SHA-256(MD5-hex(password) + salt)`.
Go strings cannot be reliably erased from memory.

`IsLoggedIn` checks locally, accepting opaque tokens and requiring more than five minutes before a readable JWT `exp` claim.
`Expiry` returns the unverified claim, including within that margin or after expiry; seconds and milliseconds are accepted.
There is no refresh or clock-skew correction. Expiry or refusal requires another login. Token invalidation does not clear the stored account.

`LogOut` contacts the server, then attempts local token/account and cookie cleanup even if the request fails. A newer login or `Forget`
supersedes an older logout. Logout reads the current shared-store token and preserves a replacement observed after the request.
The store has no atomic compare-and-delete, so callers must coordinate concurrent login writes and cleanup across Clients or processes. `Forget` does local cleanup only. Both retain `ClientID` and return store deletion errors.
Multi-step operations pin a SHA-256 identity of their first token together with the local credential generation. Every later authenticated
step reads the store and checks that pin. A successful first write of `ClientID` does not change credentials or interrupt ongoing work.

Several Clients or processes may share a store: no operation crosses accounts. An operation in progress fails with `ErrNotLoggedIn`
rather than following a login made elsewhere; a new operation may use the replacement token. The library does not coordinate their logins.
A Client notices an external logout at its next authenticated step, not asynchronously. A failed store write may have changed storage:
the Client that observed it blocks authentication until a complete login succeeds and attempts token cleanup, reporting any cleanup error.
That block is local to that Client. Other Clients can read the persisted token, so repair uncertain storage before starting new work after
an external write failure. Store reads occur at each authenticated step and can now surface read errors even with a cached token.
Already-issued requests are not canceled by these state changes.

### Hosts, transport and security

`Official()` selects `https://viewer.supernote.com` for the app API, `https://cloud.supernote.com` for the web API with XSRF,
and `https://app-wss-file.supernote.com` for the feed. `Endpoints` exposes `AppHost`, `WebHost`, `SocketHost`, `WebUsesXSRF` and `PrivateCloud`.

```go
package private

import sncloud "github.com/altessa-s/sncloud-go"

func connect(store sncloud.SecretStore) (*sncloud.Client, error) {
    endpoints := sncloud.PrivateCloud("http://cloud.example.test:8080", false)
    return sncloud.New(store, sncloud.WithEndpoints(endpoints))
}
```

Use the server's actual XSRF requirement for the second argument. `PrivateCloud` shares one origin, allows cleartext HTTP and disables the
built-in SSRF dial guard for **all** destinations, including advertised storage hosts. The caller vouches for those hosts.
Otherwise endpoint validation requires HTTPS except on localhost or a loopback IP. Allowing loopback HTTP in validation does not disable the
dial guard: the built-in public-cloud transport still refuses private, loopback and link-local addresses.

`EndpointsFromEnvironment(map[string]string)` applies opt-in `SNCLOUD_HOST` and `SNCLOUD_XSRF` overrides; `New` never reads them itself.
A host override keeps the dial guard enabled. Missing hosts, userinfo and unsupported schemes leave the hosts unchanged; `New` still
validates the resulting endpoints, including query and fragment restrictions.

`WithRoundTripper` replaces the underlying transport for both HTTP and WebSocket calls. A supplied transport owns its dial policy, SSRF
protection, TLS and connection bounds; the client cannot inspect its dials. A nil transport is ignored. HTTP envelope handling, response
limits and redirect checks still apply. WebSocket upgrades do not follow redirects. HTTP redirects refuse HTTPS downgrades and cross-origin
writes; credentials are stripped from cross-origin reads and remain stripped on later hops.

| Option | Default |
|---|---|
| `WithEndpoints` | `Official()` |
| `WithRoundTripper` | Built-in transports |
| `WithLogger` | `slog.Default()`; nil is ignored |
| `WithRetryPolicy` | `DefaultRetryPolicy()` |

### Retries and timeouts

Reads retry transport failures, HTTP 5xx and HTTP 429. State changes retry only 429, since a failed response can hide a completed write.
Replayable byte transfers, including part PUTs and form uploads, use the read policy; S3 initiation and completion use the change policy.
The default is three total attempts, starting at 300 ms, multiplying by three, capped at 20 s, with symmetric 20% jitter.
Positive `Retry-After` seconds or a future HTTP date override backoff, without jitter and capped at 20 s. Caller cancellation and deadlines are not retried.
A web XSRF 403 gets one cookie/token refresh and replay independently of this policy.

A context deadline does not bound the secret store: its methods take no context and run under the session's lock, so a store that waits
(a keychain asking for authorization, a stalled network volume) holds up that call and every other call on the same client, cleanup included.
A store must bound its own latency.

Use a context deadline for the whole operation. Built-in transports wait up to 60 s for response headers, or five minutes after an upload;
in-memory responses allow 45 s without a byte. HTTP/2 health pings use 15 s of silence and a 15 s reply budget. Streaming file bodies do not
have that idle-read timer. PDF conversion has a five-minute context deadline; PNG conversion polls the in-progress refusal every five seconds
with a five-minute polling budget, not an overall deadline for downloading all pages.

### Change-feed delivery

The first event after each connection is `ChangeConnected`: reconcile state over REST, including after a reconnect. The feed is not a durable
log and does not replay changes lost while disconnected. An announcement means look again; it may arrive more than once. Task and digest
announcements are not substitutes for the REST records. Changes from the same installation can be suppressed by the service.

`Changes(ctx)` uses one connection. `Watch(ctx)` reconnects after clean closes and retryable failures using the retry backoff, without a
`MaxAttempts` cap. Backoff resets only after a connection lasts at least one minute; brief connect-and-close cycles keep increasing it. Terminal errors, including missing or expired credentials, end the iterator. Breaking iteration or canceling its context
closes the socket; `Close` alone does not stop it.

## Errors

`Error` has `Kind() Kind`. The protocol error kinds form a closed set, and every kind has an `Err...` sentinel for `errors.Is`:

- Login: `ErrNotLoggedIn`, `ErrSessionExpired`, `ErrInvalidCredentials`, `ErrAccountLocked`, `ErrVerificationRequired`,
  `ErrInvalidVerificationCode`, `ErrNoVerificationPending`.
- Service: `ErrAlreadyExists`, `ErrNotFound`, `ErrSyncLocked`, `ErrRejected`.
- Data: `ErrMalformedResponse`, `ErrChecksumMismatch`, `ErrOversizedResponse`.
- Network: `ErrRateLimited`, `ErrHTTP`, `ErrTransport`.

Payload types are `AccountLockedError`, `NotFoundError`, `RejectedError`, `MalformedResponseError`, `ChecksumMismatchError`,
`OversizedResponseError`, `RateLimitedError`, `HTTPError` and `TransportError`. Recover their pointers with `errors.As`; `KindOf` works through
wrapping. Unknown envelope codes retain their code and message in `RejectedError`. Many service refusals arrive with HTTP 200.
Context errors, secret-store errors, local filesystem errors and constructor validation errors can fall outside the closed set.

```go
package failures

import (
    "errors"
    "fmt"

    sncloud "github.com/altessa-s/sncloud-go"
)

func describe(err error) string {
    if errors.Is(err, sncloud.ErrSessionExpired) || errors.Is(err, sncloud.ErrNotLoggedIn) {
        return "login required"
    }
    var rejected *sncloud.RejectedError
    if errors.As(err, &rejected) {
        return fmt.Sprintf("service refused: %s", rejected.Code)
    }
    var transport *sncloud.TransportError
    if errors.As(err, &transport) {
        return fmt.Sprintf("network: %s", transport.Reason)
    }
    return fmt.Sprint(err)
}
```

## Logging

`WithLogger` selects the `slog` logger. Set `SNCLOUD_WIRE_LOG=headers` or `bodies` and enable debug level to see wire diagnostics.
URLs lose userinfo, fragments and query values; query item counts remain. Credential headers and secret JSON fields are masked.
Salt, login, verification-code and token-query bodies are suppressed in both directions, even in bodies mode.
Other JSON bodies can contain filenames, account profile fields and document content; body logging is not anonymization. Non-JSON bodies are
not printed. Transport errors retain a classified reason rather than a signed URL or an unknown network error message.

## Architecture and testing

The root facade holds ports; `internal/wiring` connects application use cases to adapters. JSON API calls pass through `app/channel`, and
byte transfers through `app/transfers`. [docs/architecture.md](docs/architecture.md) describes the dependencies and protocol state machines.
[architecture_test.go](architecture_test.go) enforces import boundaries.

`internal/porttest` supplies in-memory ports and clocks. `internal/clouddouble` provides synthetic official and private layouts over loopback
`httptest` servers, with login, XSRF, file storage, tasks, digests and a WebSocket feed. The [runnable examples](example_test.go) use that emulator;
they require no account or external network. Tests using real timers and adapters also run under the `integration` build tag.
The emulator checks client behavior; it is not evidence that every deployed server version behaves identically.

## Development

| Target | What it runs |
|---|---|
| `make lint` | Module tidy, formatting and golangci-lint |
| `make vet` | `go vet` and the `go fix -diff` modernizer check |
| `make test`, `make test-race`, `make test-shuffle` | Tests, optionally with the race detector or shuffled order |
| `make coverage-gate` | Race/shuffle coverage, the 95% total floor and per-package thresholds in [.coverage.yml](.coverage.yml) |
| `make test-integration` | Unit and integration tests with race detection and shuffle |
| `make bench`, `make bench-count`, `make bench-compare` | Benchmarks, repeated measurements and comparison against `BASE` |
| `make fuzz-seed`, `make fuzz` | Fuzz seeds or mutation for `FUZZTIME` per target |
| `make generate` | Generate options with pinned optgen and gci |
| `make security-scan`, `make dupl-check` | Vulnerability/static-security scans and duplicate-code checks |
| `make ci` | Lint, vet, coverage gate, integration tests, security scans and duplication gate |

[client_bench_test.go](client_bench_test.go) measures paged listings (10/1000 entries) and verified downloads (1 KiB/1 MiB) for both cloud
flavours. Fixtures and login are outside `b.Loop`; allocations are reported. Measurements include the emulator and local temporary-file work.
The benchmark workflow compares time and allocations on pull requests; benchmarks present on only one side do not fail comparison.
See [CONTRIBUTING.md](CONTRIBUTING.md) for development conventions and [SECURITY.md](SECURITY.md) for vulnerability reports.

## Status and compatibility

The implemented API covers both cloud layouts, S3 and private form transfers, to-do lists, digests and Engine.IO 3 / Socket.IO notifications.
The China region and Aliyun OSS uploads are unsupported. This is an unofficial, reverse-engineered protocol with no server compatibility
guarantee: cloud changes may require client updates.

The ordinary tests and examples open loopback HTTP and WebSocket servers but reach no external service. A separate check under the `live` build tag, `TestPublicAPILive`, drives every exported
method (except account-wide deletion by default) against a server named in `SNCLOUD_LIVE_HOST`, `SNCLOUD_LIVE_ACCOUNT` and `SNCLOUD_LIVE_PASSWORD` and is skipped without them. `SNCLOUD_LIVE_SOCKET` optionally overrides the WebSocket origin. It has
been run against a self-hosted Supernote Private Cloud, service image 26.10.09; note conversion and the emailed login code were not exercised
there, and the official cloud has not been checked with this client.
`SNCLOUD_LIVE_DESTRUCTIVE=1` enables emptying the entire account recycle bin, the account sync-lock probe and the wrong-password test: use a disposable account only.
Without it, that subtest is skipped and cleanup deletes only identifiers created by the run.

To-do listings are always read whole. The service also offers a delta since a sync token, and this client does not ask for it: a deleted task
or list is removed outright and never shows in a delta, and service images before 26.10.09 answered a delta with other accounts' tasks, with
nothing in the protocol to tell a client which server it is talking to. The change feed says when to read the listing again.

Identifiers preserve signed 64-bit precision. `FileID` writes JSON strings; `DigestID` writes JSON numbers, and both accept either form on
input. The device folder-by-ID endpoint also requires a JSON number. Response decoding uses `encoding/json/v2` with explicit protocol
conversions for the service's mixed scalar spellings.

## License

MIT — see [LICENSE](LICENSE).

## Trademark

Supernote is a trademark of Ratta Software. This project is not affiliated with, endorsed by or sponsored by Ratta Software.
