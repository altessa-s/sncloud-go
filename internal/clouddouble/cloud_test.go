// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func exchange(t *testing.T, h http.HandlerFunc, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/"+path, strings.NewReader(body))
	r.Header.Set("version", domain.ProtocolVersion)
	r.Header.Set("equipmentNo", "synthetic-device")
	r.Header.Set("channel", "synthetic-channel")
	r.Header.Set(domain.AccessTokenHeader, "synthetic-token")
	w := httptest.NewRecorder()
	h(w, r)
	out := make(map[string]any)
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %v: %s", path, err, w.Body)
		}
	}
	return w, out
}

func requireSuccess(t *testing.T, out map[string]any) {
	t.Helper()
	if out["success"] != true {
		t.Fatalf("expected success: %v", out)
	}
}

func requireCode(t *testing.T, out map[string]any, code string) {
	t.Helper()
	if out["success"] != false || out["errorCode"] != code {
		t.Fatalf("want refusal %s: %v", code, out)
	}
}

func TestCloudAuthentication(t *testing.T) {
	for _, flavor := range []Flavor{Official, Private} {
		t.Run(string(flavor), func(t *testing.T) {
			c := New(flavor)
			t.Cleanup(c.Close)
			ep := c.Endpoints()
			if (ep.AppHost == ep.WebHost) != (flavor == Private) || ep.PrivateCloud != (flavor == Private) || ep.WebUsesXSRF != (flavor == Official) {
				t.Fatalf("endpoints: %+v", ep)
			}
			h := func(w http.ResponseWriter, r *http.Request) { c.serve(w, r, false) }
			challenge := func() string {
				_, out := exchange(t, h, domain.PathRandomCode, `{"account":"test@example.invalid"}`)
				requireSuccess(t, out)
				if out["randomCode"] != "synthetic-salt" {
					t.Fatal(out)
				}
				timestamp, err := json.Marshal(out["timestamp"])
				if err != nil {
					t.Fatal(err)
				}
				return string(timestamp)
			}
			login := func(timestamp, extra string) map[string]any {
				_, out := exchange(t, h, domain.PathLogin, `{"account":"test@example.invalid","password":"`+domain.PasswordDigest("synthetic-password", "synthetic-salt")+`","timestamp":`+timestamp+extra+`}`)
				return out
			}
			fields := `,"equipment":2,"equipmentNo":null,"browser":null`
			stamp := challenge()
			requireCode(t, login(`0`, fields), "E0019")
			requireCode(t, login(stamp, fields), "E0019") // Failed attempts consume the challenge.
			stamp = challenge()
			_, out := exchange(t, h, domain.PathLogin, `{"account":"test@example.invalid","password":"wrong","timestamp":`+stamp+fields+`}`)
			requireCode(t, out, "E0019")
			stamp = challenge()
			badEquipment := `{"account":"test@example.invalid","password":"` + domain.PasswordDigest("synthetic-password", "synthetic-salt") + `","timestamp":` + stamp + `,"equipment":1}`
			rejected, _ := exchange(t, h, domain.PathLogin, badEquipment)
			if rejected.Code != http.StatusBadRequest {
				t.Fatalf("invalid equipment: status %d", rejected.Code)
			}
			c.RequireVerification(true)
			requireCode(t, login(challenge(), fields), "E1760")
			c.RequireVerification(false)
			out = login(challenge(), fields)
			requireSuccess(t, out)
			token, ok := out["token"].(string)
			if !ok || token == "" {
				t.Fatal(out)
			}
			r := httptest.NewRequest(http.MethodPost, "/api/"+domain.PathLogout, strings.NewReader(`{}`))
			r.Header.Set("version", domain.ProtocolVersion)
			r.Header.Set("equipmentNo", "synthetic-device")
			r.Header.Set(domain.AccessTokenHeader, token)
			w := httptest.NewRecorder()
			h(w, r)
			if !strings.Contains(w.Body.String(), `"success":true`) || c.tokens[token] {
				t.Fatal(w.Body)
			}
			_, out = exchange(t, h, domain.PathLogout, `{}`)
			requireCode(t, out, "E0712")
			_, out = exchange(t, h, domain.PathMailCode, `{"email":"test@example.invalid"}`)
			requireSuccess(t, out)
			if out["validCodeKey"] != "synthetic-key" {
				t.Fatal(out)
			}
			_, out = exchange(t, h, domain.PathCheckCode, `{"validCode":"wrong","validCodeKey":"synthetic-key"}`)
			requireCode(t, out, "E1756")
			_, out = exchange(t, h, domain.PathCheckCode, `{"validCode":"123456","validCodeKey":"synthetic-key"}`)
			requireSuccess(t, out)
			_, out = exchange(t, h, domain.PathCodeLogin, `{"validCode":"123456","validCodeKey":"synthetic-key","email":"other@example.invalid"}`)
			requireCode(t, out, "E1756")
			body := `{"validCode":"123456","validCodeKey":"synthetic-key","email":"test@example.invalid"}`
			_, out = exchange(t, h, domain.PathCodeLogin, body)
			requireSuccess(t, out)
			if out["token"] == token {
				t.Fatal("token reused")
			}
			_, out = exchange(t, h, domain.PathCodeLogin, body)
			requireCode(t, out, "E1756")
			for _, tc := range []struct {
				method, body string
				status       int
			}{{"GET", "{}", 400}, {"POST", "bad", 400}, {"POST", "{}", 404}} {
				r := httptest.NewRequest(tc.method, "/api/missing", strings.NewReader(tc.body))
				r.Header.Set("version", domain.ProtocolVersion)
				r.Header.Set("equipmentNo", "synthetic-device")
				w := httptest.NewRecorder()
				h(w, r)
				if w.Code != tc.status {
					t.Fatal(w.Code, tc)
				}
			}
		})
	}
}

func TestCloudRegistryAndXSRF(t *testing.T) {
	for _, flavor := range []Flavor{Official, Private} {
		t.Run(string(flavor), func(t *testing.T) {
			c := New(flavor)
			t.Cleanup(c.Close)
			c.Register(true, "/api/fixture", func(w http.ResponseWriter, r *http.Request) {
				c.Register(true, "next", func(w http.ResponseWriter, r *http.Request) { Reply(w, nil) })
				Reply(w, map[string]any{"value": "synthetic"})
			})
			web := flavor == Official
			h := func(w http.ResponseWriter, r *http.Request) { c.serve(w, r, web) }
			if web {
				w, _ := exchange(t, h, "fixture", `{}`)
				if w.Code != 403 {
					t.Fatal(w.Code)
				}
				w, out := exchange(t, h, "csrf", `{}`)
				requireSuccess(t, out)
				if w.Header().Get(domain.XSRFHeader) != "synthetic-xsrf" || !strings.Contains(w.Header().Get("Set-Cookie"), "synthetic-xsrf") {
					t.Fatal(w.Header())
				}
			}
			for _, path := range []string{"fixture", "next", "missing"} {
				r := httptest.NewRequest("POST", "/api/"+path, strings.NewReader(`{}`))
				r.AddCookie(&http.Cookie{Name: domain.XSRFCookie, Value: "synthetic-xsrf"})
				r.Header.Set(domain.XSRFHeader, "synthetic-xsrf")
				r.Header.Set("version", domain.ProtocolVersion)
				r.Header.Set("equipmentNo", "device")
				w := httptest.NewRecorder()
				h(w, r)
				want := 200
				if path == "missing" {
					want = 404
				}
				if w.Code != want {
					t.Fatal(path, w.Code)
				}
				if path == "fixture" && !strings.Contains(w.Body.String(), `"value":"synthetic"`) {
					t.Fatal(w.Body)
				}
			}
			calls := c.Calls()
			calls[0] = "changed"
			if c.Calls()[0] == "changed" {
				t.Fatal("Calls aliases state")
			}
		})
	}
}
