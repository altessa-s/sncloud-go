// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package clouddouble emulates both cloud layouts with synthetic data and extensible handlers.
package clouddouble

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Fixed synthetic challenge time exercises exact millisecond echoing.
const challengeTime int64 = 1234567890123

// Flavor selects split official hosts or a shared private host.
type Flavor string

// Supported server layouts.
const (
	// Official emulates the separate app and web origins.
	Official Flavor = "official"
	// Private emulates a shared private-cloud origin.
	Private Flavor = "private"
)

// Cloud owns local servers and protocol state.
type Cloud struct {
	mu        sync.Mutex
	flavor    Flavor
	app, web  *httptest.Server
	handlers  map[string]http.HandlerFunc
	challenge map[string]bool
	tokens    map[string]bool
	pending   map[string]string
	verify    bool
	serial    int
	calls     []string
	state     *appState
}

// New starts an emulator. Close must be called after its clients finish.
func New(f Flavor) *Cloud {
	c := &Cloud{
		flavor:    f,
		handlers:  make(map[string]http.HandlerFunc),
		challenge: make(map[string]bool),
		tokens:    make(map[string]bool),
		pending:   make(map[string]string),
	}
	c.initServices()
	c.app = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { c.serve(w, r, false) }))
	if f == Official {
		c.web = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { c.serve(w, r, true) }))
	} else {
		c.web = c.app
	}
	return c
}

// Close shuts down both hosts.
func (c *Cloud) Close() {
	c.closeServices()
	c.app.Close()
	if c.web != c.app {
		c.web.Close()
	}
}

// Endpoints returns this emulator's configuration.
func (c *Cloud) Endpoints() domain.Endpoints {
	return domain.Endpoints{
		AppHost:      c.app.URL,
		WebHost:      c.web.URL,
		SocketHost:   c.app.URL,
		WebUsesXSRF:  c.flavor == Official,
		PrivateCloud: c.flavor == Private,
	}
}

// RequireVerification switches subsequent logins to the email flow.
func (c *Cloud) RequireVerification(on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.verify = on
}

// Register adds or replaces a handler under /api. web selects the web host.
// Handlers run without the registry lock, and may register other handlers.
func (c *Cloud) Register(web bool, path string, h http.HandlerFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[fmt.Sprint(web)+":"+strings.TrimPrefix(path, "/api/")] = h
}

// Calls returns the paths observed by both hosts.
func (c *Cloud) Calls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}

func (c *Cloud) serve(w http.ResponseWriter, r *http.Request, web bool) {
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	c.mu.Lock()
	c.calls = append(c.calls, r.URL.Path)
	h := c.handlers[fmt.Sprint(web)+":"+path]
	if h == nil && c.flavor == Private {
		h = c.handlers["true:"+path]
	}
	c.mu.Unlock()
	if web && path == "csrf" {
		// #nosec G124 -- Synthetic XSRF cookie on a loopback HTTP emulator; browser-readable by protocol design.
		http.SetCookie(w, &http.Cookie{Name: domain.XSRFCookie, Value: "synthetic-xsrf", Path: "/"})
		w.Header().Set(domain.XSRFHeader, "synthetic-xsrf")
		Reply(w, nil)
		return
	}
	if web {
		cookie, err := r.Cookie(domain.XSRFCookie)
		if err != nil || cookie.Value != "synthetic-xsrf" || r.Header.Get(domain.XSRFHeader) != cookie.Value {
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}
	if h != nil {
		h(w, r)
		return
	}
	if !web && c.serveServices(w, r, path) {
		return
	}
	c.serveApp(w, r, web, path)
}

func (c *Cloud) serveApp(w http.ResponseWriter, r *http.Request, web bool, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if web {
		http.NotFound(w, r)
		return
	}
	if r.Method != "POST" || r.Header.Get("version") != domain.ProtocolVersion || r.Header.Get("equipmentNo") == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	b, ok := decodeObject[map[string]json.RawMessage](w, r)
	if !ok {
		return
	}
	str := func(k string) string {
		var s string
		if json.Unmarshal(b[k], &s) != nil {
			return ""
		}
		return s
	}
	switch path {
	case domain.PathRandomCode:
		c.challenge[str("account")] = true
		timestamp := any(challengeTime)
		if c.flavor == Private {
			timestamp = "1234567890123"
		}
		Reply(w, map[string]any{"randomCode": "synthetic-salt", "timestamp": timestamp})
	case domain.PathLogin:
		account := str("account")
		fresh := c.challenge[account]
		delete(c.challenge, account)
		wantTime := `1234567890123`
		if c.flavor == Private {
			wantTime = `"1234567890123"`
		}
		if !fresh || str("password") != domain.PasswordDigest("synthetic-password", "synthetic-salt") || string(b["timestamp"]) != wantTime {
			Refuse(w, "E0019")
			return
		}
		if string(b["equipment"]) != "2" || string(b["equipmentNo"]) != "null" || string(b["browser"]) != "null" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if c.verify {
			Refuse(w, "E1760")
			return
		}
		c.issue(w)
	case domain.PathMailCode:
		c.pending["synthetic-key"] = str("email")
		Reply(w, map[string]any{"validCodeKey": "synthetic-key"})
	case domain.PathCheckCode:
		if str("validCode") != "123456" || c.pending[str("validCodeKey")] == "" {
			Refuse(w, "E1756")
			return
		}
		Reply(w, nil)
	case domain.PathCodeLogin:
		if str("validCode") != "123456" || c.pending[str("validCodeKey")] != str("email") {
			Refuse(w, "E1756")
			return
		}
		delete(c.pending, str("validCodeKey"))
		c.issue(w)
	case domain.PathLogout:
		token := r.Header.Get(domain.AccessTokenHeader)
		if !c.tokens[token] {
			Refuse(w, "E0712")
			return
		}
		delete(c.tokens, token)
		Reply(w, nil)
	default:
		http.NotFound(w, r)
	}
}
