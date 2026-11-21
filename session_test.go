// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

type facadeTransport func(*http.Request) (*http.Response, error)

func (f facadeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFacadeSession(t *testing.T) {
	for _, verify := range []bool{false, true} {
		c, err := sncloud.New(&porttest.Secrets{}, nil, sncloud.WithRoundTripper(facadeTransport(func(r *http.Request) (*http.Response, error) {
			body := `{"success":true}`
			switch {
			case strings.HasSuffix(r.URL.Path, "random/code"):
				body = `{"success":true,"randomCode":"salt","timestamp":"123"}`
			case strings.HasSuffix(r.URL.Path, "login/new"):
				body = `{"success":true,"token":"opaque"}`
				if verify {
					body = `{"success":false,"errorCode":"E1760"}`
				}
			case strings.HasSuffix(r.URL.Path, "mail/validcode"):
				body = `{"success":true,"validCodeKey":"key"}`
			case strings.HasSuffix(r.URL.Path, "sms/login"):
				body = `{"success":true,"token":"opaque"}`
			}
			return &http.Response{
				StatusCode: 200,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		out, err := c.LogIn(context.Background(), "synthetic@example.com", "synthetic-password", nil)
		if err != nil {
			t.Fatal(err)
		}
		if verify {
			if out != sncloud.VerificationRequired {
				t.Fatal(out)
			}
			if err := c.SendVerificationCode(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := c.Verify(context.Background(), "123456"); err != nil {
				t.Fatal(err)
			}
		} else if out != sncloud.LoggedIn {
			t.Fatal(out)
		}
		if ok, err := c.IsLoggedIn(); !ok || err != nil {
			t.Fatal(ok, err)
		}
		if account, err := c.Account(); account != "synthetic@example.com" || err != nil {
			t.Fatal(account, err)
		}
		if exp, err := c.Expiry(); exp != nil || err != nil {
			t.Fatal(exp, err)
		}
		if id, err := c.Identity(); id.ClientID == "" || err != nil {
			t.Fatal(id, err)
		}
		if err := c.LogOut(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := c.Forget(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFacadeValues(t *testing.T) {
	if sncloud.Official().PrivateCloud {
		t.Fatal("official cloud is private")
	}
	private := sncloud.PrivateCloud("http://nas.invalid", false)
	if !private.PrivateCloud {
		t.Fatal("private cloud guard still enabled")
	}
	debug := sncloud.EndpointsFromEnvironment(map[string]string{"SNCLOUD_HOST": "http://127.0.0.1"})
	if debug.PrivateCloud {
		t.Fatal("debug override disabled guard")
	}
	if sncloud.NewDeviceIdentity("LINUX", "ABC").ClientID != "abc" {
		t.Fatal("identity not normalized")
	}
	if sncloud.KindOf(sncloud.ErrNotLoggedIn) != sncloud.KindNotLoggedIn {
		t.Fatal("kind not forwarded")
	}
}
