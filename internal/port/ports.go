// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package port defines the boundaries used by the application layer.
package port

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas-core/time/clock"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Header carries HTTP metadata without coupling use cases to net/http.
type Header map[string][]string

// Get looks up a header without regard to case.
func (h Header) Get(name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

// Clone prevents a request mutation from changing another caller's headers.
func (h Header) Clone() Header {
	out := make(Header, len(h))
	for k, v := range h {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// Set replaces a field, including differently cased spellings.
func (h Header) Set(name, value string) {
	for k := range h {
		if strings.EqualFold(k, name) {
			delete(h, k)
		}
	}
	h[name] = []string{value}
}

// Cookie is a parsed response cookie; parsing belongs to the HTTP adapter.
type Cookie struct {
	Name, Value string
	Expires     time.Time
	Delete      bool
}

// Progress reports transferred bytes and a total, negative when unknown.
type Progress func(done, total int64)

// Request describes one exchange. File is streamed by Upload; Dest is owned by Download's caller.
type Request struct {
	Method, URL string
	Header      Header
	Body        []byte
	File, Dest  string
	// Section optionally restricts an upload to a replayable range of File.
	Section  *FileSection
	Limit    int64
	Progress Progress
	// LongRunning selects the bounded five-minute server-processing budget.
	LongRunning bool
}

// FileSection selects an exact byte range; a short source fails the upload.
type FileSection struct {
	Offset, Length int64
}

// Response is uninterpreted HTTP data. File, when present, belongs to the caller.
type Response struct {
	Status  int
	Header  Header
	Body    []byte
	Cookies []Cookie
	File    string
}

// RoundTripper exchanges bounded in-memory protocol messages.
type RoundTripper interface {
	RoundTrip(context.Context, Request) (Response, error)
}

// Transport also moves large bodies without keeping them in memory.
type Transport interface {
	RoundTripper
	Download(context.Context, Request) (Response, error)
	Upload(context.Context, Request) (Response, error)
}

// SecretStore persists only the three protocol secrets. Implementations must be concurrency safe.
// An empty value with nil error means absent. Methods must not call back into the client.
// A failed Keep or Discard is assumed to have possibly changed the value.
// After a failed write the session rejects authentication until a login succeeds,
// attempts to discard the access token, and reports cleanup errors alongside the
// write error. A failed cleanup may leave a persisted value.
//
// Several Clients or processes may share a store. Each operation pins its first
// token and fails with ErrNotLoggedIn rather than continuing under a different
// token written elsewhere. Logins are not coordinated across Clients or processes;
// external logout is noticed at the next authenticated step, not asynchronously.
// The authentication block after a failed write belongs only to the Client that
// observed it. Other Clients and new operations can read the persisted token;
// repair uncertain storage before starting new work if a write failed elsewhere.
//
// A store bounds its own latency. Its methods take no context, and the session
// calls them while holding its lock: a context deadline on an operation, or the
// short deadline of a cleanup such as the abort of an upload or the release of
// the sync lock, does not interrupt a store call, and other calls on the same
// Client wait behind it.
type SecretStore interface {
	Secret(domain.Secret) (string, error)
	Keep(string, domain.Secret) error
	Discard(domain.Secret) error
}

// Clock lets expiry and retries use one notion of time.
type Clock interface {
	Now() time.Time
	Sleep(context.Context, time.Duration) error
}

// Logger is the structured log boundary. Credentials never reach its arguments.
type Logger interface {
	Log(context.Context, slog.Level, string, ...any)
}

// Socket is a framed text connection used by the change feed.
type Socket interface {
	Read(context.Context) ([]byte, error)
	Write(context.Context, []byte) error
	Close() error
}

// SocketDialer establishes a feed connection with protocol headers.
type SocketDialer interface {
	Dial(context.Context, string, Header) (Socket, error)
}

// Call describes a JSON API call. Nil Body means no body; an empty struct means {}.
type Call struct {
	Path, Method, Token string
	Web, Changes        bool
	Header              Header
	Body                any
	Epoch               *uint64
	// LongRunning allows note rendering to finish before response headers arrive.
	LongRunning bool
}

// Channel is the one path for protocol calls and browser conversation resets.
type Channel interface {
	Call(context.Context, Call, any) error
	Forget() uint64
	Epoch() uint64
}

// SystemClock uses the runtime timer, including testing/synctest's virtual time.
type SystemClock struct{}

// Now returns the wall clock.
func (SystemClock) Now() time.Time { return clock.Real().Now() }

// Sleep respects cancellation even for zero delays.
func (SystemClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
