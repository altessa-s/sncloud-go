// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"net/http"
	"net/url"
	"strings"
)

// Bound redirect chains, matching net/http.
const redirectLimit = 10

func origin(u *url.URL) string {
	if u == nil {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return scheme + "://" + host + ":" + port
}
func (t *Transport) redirect(r *http.Request, via []*http.Request) error {
	if len(via) >= redirectLimit {
		return http.ErrUseLastResponse
	}
	previous := via[len(via)-1]
	same := origin(previous.URL) == origin(r.URL)
	// Never downgrade credentials to cleartext, or replay a write to a new origin.
	if strings.EqualFold(previous.URL.Scheme, "https") && !strings.EqualFold(r.URL.Scheme, "https") {
		return http.ErrUseLastResponse
	}
	if !same && r.Method != "GET" && r.Method != "HEAD" {
		return http.ErrUseLastResponse
	}
	// net/http reconstructs headers from the initial request on each hop.
	// Once stripped, credentials must not reappear on a later same-origin hop.
	crossed := !same
	for i := 1; i < len(via); i++ {
		crossed = crossed || origin(via[i-1].URL) != origin(via[i].URL)
	}
	if crossed {
		for k := range r.Header {
			if secretField(k) {
				delete(r.Header, k)
			}
		}
	}
	t.log(r.Context(), "redirect", r, 0, r.Header, nil)
	return nil
}
