// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud

import (
	"log/slog"
	"net/http"
	"reflect"
)

// options is read once by New. Optgen generates setters except the documented manual option.
//
//go:generate optgen generate --all-fields --string-setter plain --source-file $GOFILE
type options struct {
	endpoints Endpoints `optgen:"default=Official()"`
	// The pinned generator omits field documentation; keep the public setter manual
	// so the transport-policy contract survives regeneration.
	// roundTripper replaces the built-in transport. The built-in transport guards
	// against private and loopback addresses; supplying a transport takes over
	// that dial policy, including address protection and connection bounds.
	roundTripper http.RoundTripper `optgen:"manual"`
	logger       *slog.Logger      `optgen:"default=slog.Default()"`
	retryPolicy  RetryPolicy       `optgen:"default=DefaultRetryPolicy()"`
}

// WithRoundTripper replaces the built-in transport in both the HTTP and
// WebSocket adapters; a nil value is ignored. The built-in transport blocks
// private, loopback, and link-local addresses unless [Endpoints].PrivateCloud
// is true. Supplying a transport takes over that dial policy, including
// address/SSRF protection, TLS settings, and connection bounds.
func WithRoundTripper(v http.RoundTripper) Option {
	return func(o *options) {
		switch rv := reflect.ValueOf(v); rv.Kind() {
		case reflect.Invalid:
			return
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
			if rv.IsNil() {
				return
			}
		default:
			// Value implementations have no nil state.
		}
		o.roundTripper = v
	}
}
