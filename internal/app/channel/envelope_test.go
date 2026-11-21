// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package channel

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestReadsTheEnvelope(t *testing.T) {
	for _, code := range []string{"E0712", "E0019", "E0322", "E0078", "unknown"} {
		err := Open([]byte(`{"success":false,"errorCode":"`+code+`"}`), nil)
		if domain.KindOf(err) != domain.KindOf(domain.FromEnvelope(code, "")) {
			t.Fatal(code, err)
		}
	}
}
func TestRefusesNonJSON(t *testing.T) {
	for _, b := range []string{"<html>secret</html>", `{"token":"secret"`, "null", "[]", ""} {
		err := Open([]byte(b), nil)
		if !errors.Is(err, domain.ErrMalformedResponse) || strings.Contains(err.Error(), "secret") {
			t.Fatal(err)
		}
	}
}
func TestReadsAMissingSuccess(t *testing.T) {
	if err := Open([]byte(`{"token":"synthetic"}`), nil); err != nil {
		t.Fatal(err)
	}
	if err := Open([]byte(`{"errorCode":"E0712"}`), nil); !errors.Is(err, domain.ErrSessionExpired) {
		t.Fatal(err)
	}
	if err := Open([]byte(`{"success":"Y"}`), nil); err != nil {
		t.Fatal(err)
	}
}
func TestDescribesWithoutQuoting(t *testing.T) {
	if got := Shape([]byte(`{"token":"jwt`)); got != "JSON of another shape, 13 bytes" {
		t.Fatal(got)
	}
	if Shape(nil) != "empty, 0 bytes" {
		t.Fatal(Shape(nil))
	}
}
func TestHTTPEnvelopePrecedence(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{409, domain.ErrSyncLocked},
		{500, domain.ErrHTTP},
		{429, domain.ErrRateLimited},
		{401, domain.ErrSessionExpired},
	} {
		err := explained(domain.FromHTTP(tc.status, "/x", "", time.Time{}), []byte(`{"success":false,"errorCode":"E0078"}`))
		if !errors.Is(err, tc.want) {
			t.Fatal(tc, err)
		}
	}
}
func TestMalformedPayloadAndUnencodableBody(t *testing.T) {
	c, tr := makeChannel()
	if err := c.Call(context.Background(), port.Call{Path: "x", Body: make(chan int)}, nil); err == nil || len(tr.Calls()) != 0 {
		t.Fatal(err)
	}
	var out struct {
		Token int `json:"token"`
	}
	if err := Open([]byte(`{"success":true,"token":"synthetic-secret"}`), &out); !errors.Is(err, domain.ErrMalformedResponse) ||
		strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatal(err)
	}
}

func TestStrictJSONEnvelope(t *testing.T) {
	for _, data := range []string{
		`{"success":true,"success":false}`,
		"{\"success\":true,\"token\":\"\xff\"}",
	} {
		if err := Open([]byte(data), nil); !errors.Is(err, domain.ErrMalformedResponse) {
			t.Fatalf("invalid JSON accepted: %v", err)
		}
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := Open([]byte(`{"success":true,"Token":"synthetic"}`), &out); err != nil || out.Token != "" {
		t.Fatalf("wrong-case key matched: %+v, %v", out, err)
	}
}
