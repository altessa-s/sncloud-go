//go:build integration

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/adapter/httpapi"
	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

func TestRegistryAndWebHandshake(t *testing.T) {
	for _, f := range []Flavor{Official, Private} {
		t.Run(string(f), func(t *testing.T) {
			cloud := New(f)
			defer cloud.Close()
			cloud.Register(
				true,
				"fixture",
				func(w http.ResponseWriter, r *http.Request) { Reply(w, map[string]any{"value": "synthetic"}) },
			)
			tr := httpapi.New(httpapi.Config{RoundTripper: http.DefaultTransport})
			defer tr.CloseIdleConnections()
			ch := channel.New(cloud.Endpoints(), tr, port.SystemClock{}, domain.DefaultRetryPolicy())
			var out struct {
				Value string `json:"value"`
			}
			if err := ch.Call(context.Background(), port.Call{Path: "fixture", Web: true, Body: struct{}{}}, &out); err != nil || out.Value != "synthetic" {
				t.Fatal(out, err)
			}
			want := 1
			if f == Official {
				want = 2
			}
			if len(cloud.Calls()) != want {
				t.Fatal(cloud.Calls())
			}
		})
	}
}

func TestLoginAndVerificationEmulation(t *testing.T) {
	for _, f := range []Flavor{Official, Private} {
		cloud := New(f)
		func() {
			defer cloud.Close()
			store := &porttest.Secrets{}
			client, err := sncloud.New(store, sncloud.WithEndpoints(cloud.Endpoints()), sncloud.WithRoundTripper(http.DefaultTransport))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ctx := context.Background()
			if _, err := client.LogIn(ctx, "synthetic@example.com", "wrong", nil); !errors.Is(err, sncloud.ErrInvalidCredentials) {
				t.Fatal(err)
			}
			if _, err := client.LogIn(ctx, "synthetic@example.com", "synthetic-password", nil); err != nil {
				t.Fatal(err)
			}
			if err := client.LogOut(ctx); err != nil {
				t.Fatal(err)
			}
			cloud.RequireVerification(true)
			if out, err := client.LogIn(ctx, "synthetic@example.com", "synthetic-password", nil); err != nil || out != sncloud.VerificationRequired {
				t.Fatal(out, err)
			}
			if err := client.SendVerificationCode(ctx); err != nil {
				t.Fatal(err)
			}
			if err := client.Verify(ctx, "bad"); !errors.Is(err, sncloud.ErrInvalidVerificationCode) {
				t.Fatal(err)
			}
			if err := client.Verify(ctx, "123456"); err != nil {
				t.Fatal(err)
			}
			if err := client.LogOut(ctx); err != nil {
				t.Fatal(err)
			}
		}()
	}
}
func TestEmulatorRefusals(t *testing.T) {
	cloud := New(Official)
	defer cloud.Close()
	for _, tc := range []struct {
		web                bool
		path, method, body string
		status             int
	}{
		{true, "fixture", "POST", "{}", 403},
		{false, "missing", "GET", "{}", 400},
		{false, "missing", "POST", "bad", 400},
		{false, "missing", "POST", "{}", 404},
		{false, domain.PathLogout, "POST", "{}", 200},
		{false, domain.PathCodeLogin, "POST", `{"validCode":"wrong"}`, 200},
	} {
		host := cloud.Endpoints().App(tc.path)
		if tc.web {
			host = cloud.Endpoints().Web(tc.path)
		}
		req, err := http.NewRequest(tc.method, host, strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("version", domain.ProtocolVersion)
		req.Header.Set("equipmentNo", "synthetic-device")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != tc.status {
			t.Fatal(tc, resp.StatusCode)
		}
	}
}
