// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud

import (
	"errors"
	"reflect"

	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/wiring"
)

// SecretStore persists the three protocol secrets ([AccessToken], [Account],
// and [ClientID]) on the caller's behalf. Passwords are never passed to it.
//
// Implementations must be safe for concurrent use and must not call back into
// [Client]. Returning ("", nil) from Secret reports that the key is absent.
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
// A store bounds its own latency: its methods take no context and are called
// under the session's lock, so a context deadline does not interrupt a store
// call, and other calls on the same Client wait behind it.
type SecretStore = port.SecretStore

// Client owns one account session, browser cookie jar, and network services.
// A Client and its views are safe for concurrent use.
type Client struct {
	files     port.WebFiles
	device    port.DeviceFiles
	session   port.AccountSession
	tasks     port.TaskService
	digests   port.DigestService
	feed      port.ChangeFeed
	transport port.IdleCloser
}

// New assembles a client without making network requests. A store is required.
// Nil options are ignored. Endpoint environment overrides are opt-in through
// EndpointsFromEnvironment; SNCLOUD_WIRE_LOG controls sanitized diagnostics.
func New(store SecretStore, opts ...Option) (*Client, error) {
	if store == nil {
		return nil, errors.New("sncloud: secret store is required")
	}
	v := reflect.ValueOf(store)
	if (v.Kind() == reflect.Pointer || v.Kind() == reflect.Map || v.Kind() == reflect.Interface) && v.IsNil() {
		return nil, errors.New("sncloud: secret store is required")
	}
	filtered := make([]Option, 0, len(opts))
	for _, o := range opts {
		if o != nil {
			filtered = append(filtered, o)
		}
	}
	cfg := newOptions(filtered...)
	if err := cfg.endpoints.Validate(); err != nil {
		return nil, err
	}
	parts := wiring.Build(wiring.Config{
		Secrets:      store,
		Endpoints:    cfg.endpoints,
		RoundTripper: cfg.roundTripper,
		Logger:       cfg.logger,
		Policy:       cfg.retryPolicy,
	})
	return &Client{
		files:     parts.Files,
		device:    parts.Device,
		session:   parts.Session,
		tasks:     parts.Tasks,
		digests:   parts.Digests,
		feed:      parts.Feed,
		transport: parts.Transport,
	}, nil
}

// Close releases idle connections. It does not log out or interrupt active calls.
func (c *Client) Close() error {
	c.transport.CloseIdleConnections()
	return nil
}
