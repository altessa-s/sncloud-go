// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package transfers is the shared byte path for web, device and digest clients.
// Callers obtain an UploadSlot through the channel, Measure a source, then Send
// it with a credential-pinned Signer. Fetch returns a temporary file owned by
// the caller. Failed operations remove their temporary files.
package transfers

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

const (
	methodPost           = "POST"
	uploadIDParam        = "uploadId"
	statusPartialContent = 206
	// DownloadLimit bounds a download whose size the service did not declare.
	DownloadLimit = domain.DownloadLimit
	// ChunkSize matches the tablet's eight-mebibyte upload parts.
	ChunkSize      int64 = 8 * 1024 * 1024
	cleanupTimeout       = 5 * time.Second
)

// Transfers moves bytes through the same transport as the protocol channel.
type Transfers struct {
	transport port.Transport
	clock     port.Clock
	policy    domain.RetryPolicy
}

// New takes the shared network stack and retry policy from wiring.
func New(t port.Transport, clock port.Clock, policy domain.RetryPolicy) *Transfers {
	return &Transfers{transport: t, clock: clock, policy: policy}
}

func malformed(what string) error { return &domain.MalformedResponseError{What: what} }

// accepted classifies storage errors without invalidating the account session.
// A storage signature can expire while the account token is still valid.
func accepted(r port.Response, target string, now time.Time) error {
	u, err := url.Parse(target)
	if err != nil {
		return malformed("invalid storage URL")
	}
	err = domain.FromHTTP(r.Status, u.Path, r.Header.Get("Retry-After"), now)
	if errors.Is(err, domain.ErrSessionExpired) {
		return &domain.HTTPError{Status: r.Status, Path: u.Path}
	}
	return err
}

// storageURL rejects missing or unusable destinations before entering retries.
// Never include the server value in an error: it may contain a signed query.
func storageURL(target string) (*url.URL, error) {
	u, err := url.Parse(target)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, malformed("invalid storage URL")
	}
	return u, nil
}

// ServedFrom repairs private storage links using the configured origin only.
func ServedFrom(target, host string) string {
	u, err := url.Parse(target)
	if err != nil || !strings.HasPrefix(u.Path, "/api/oss/") {
		return target
	}
	return moved(target, host)
}

// moved preserves the storage path and query but removes any embedded credentials.
func moved(target, host string) string {
	u, err := url.Parse(target)
	if err != nil {
		return target
	}
	base, err := url.Parse(host)
	if err != nil || base.Host == "" || base.Scheme == "" {
		return target
	}
	u.Scheme = base.Scheme
	u.Host = base.Host
	u.User = nil
	return u.String()
}

// Authorized sends a protocol call with a token checked against the operation pin.
// Storage requests never use this helper: a storage refusal cannot expire a login.
func Authorized(ctx context.Context, ch port.Channel, s port.FileSession, call port.Call, pin *domain.CredentialPin, out any) error {
	epoch := ch.Epoch()
	token, err := s.AccessToken(pin)
	if err != nil {
		return err
	}
	call.Token = token
	call.Epoch = &epoch
	if !call.Web {
		call.Header, err = s.AppHeaders(token, true)
		if err != nil {
			return err
		}
	}
	err = ch.Call(ctx, call, out)
	if errors.Is(err, domain.ErrSessionExpired) {
		s.Invalidate(token)
	}
	return err
}
