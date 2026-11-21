// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestCancelsOnTheWire(t *testing.T) {
	arrived := make(chan struct{})
	stopped := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(arrived)
		<-r.Context().Done()
		close(stopped)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	tr := newTestTransport(t, Config{})
	done := make(chan error, 1)
	go func() {
		_, err := tr.Download(ctx, port.Request{URL: srv.URL})
		done <- err
	}()
	<-arrived
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	<-stopped
}
func TestMasksTheWire(t *testing.T) {
	for _, field := range []string{"x-access-token", "X-XSRF-TOKEN", "Cookie", "Set-Cookie", "Authorization", "channel"} {
		if !secretField(field) {
			t.Fatal(field)
		}
	}
	if secretField("Content-Type") {
		t.Fatal("public header masked")
	}
	for _, path := range []string{
		"/api/official/user/account/login/new",
		"/api/official/user/sms/login",
		"/api/user/mail/validcode",
		"/api/user/check/validcode",
	} {
		if !credentialBody(path) {
			t.Fatal(path)
		}
	}
	if credentialBody("/api/file/list/query") {
		t.Fatal("file body hidden")
	}
	if got := safeURL("https://s3.example/o?X-Amz-Signature=secret&a=b"); got != "https://s3.example/o (+2 query items)" {
		t.Fatal(got)
	}
}
func TestRedactsLoggedBodies(t *testing.T) {
	s := bodyText(
		[]byte(`{"success":true,"url":"https://bucket.example/o?X-Amz-Signature=secret",
"s3Authorization":"Credential=key","token":"jwt",
"validCodeKey":"k","fileName":"Paper.pdf"}`),
		"application/json",
	)
	for _, leak := range []string{"Signature=secret", "Credential=key", "jwt"} {
		if strings.Contains(s, leak) {
			t.Fatal(s)
		}
	}
	if !strings.Contains(s, `"validCodeKey":"<redacted>"`) || !strings.Contains(s, "Paper.pdf") || !strings.Contains(s, "(+1 query items)") {
		t.Fatal(s)
	}
}
func TestCutsALongBodySafely(t *testing.T) {
	s := bodyText([]byte(`{"fileName":"`+strings.Repeat("ж", 4096)+`"}`), "application/json")
	if len(s) >= 4160 || !utf8.ValidString(s) || !strings.Contains(s, "bytes in all") {
		t.Fatal(len(s), s[len(s)-40:])
	}
}
func TestSanitisesLocationAndNeverPrintsLogin(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	tr := newTestTransport(t, Config{
		WireLevel: WireBodies,
		Logger:    logger, RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Header: http.Header{
					"Location":     {"https://storage.example/o?signature=hidden"},
					"Content-Type": {"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{"token":"synthetic-credential"}`)),
			}, nil
		}),
	})
	_, err := tr.RoundTrip(context.Background(), port.Request{
		URL:    "https://synthetic.invalid/api/official/user/account/login/new?secret=hidden",
		Method: "POST",
		Header: port.Header{"Content-Type": {"application/json"}, "x-access-token": {"synthetic-header"}},
		Body:   []byte(`{"password":"synthetic-password"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"hidden", "synthetic-credential", "synthetic-password", "synthetic-header"} {
		if strings.Contains(logs.String(), leak) {
			t.Fatal(logs.String())
		}
	}
}
func TestLogsAFailureWithoutItsURL(t *testing.T) {
	var logs bytes.Buffer
	tr := newTestTransport(t, Config{
		WireLevel: WireBodies,
		Logger:    slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		RoundTripper: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, &url.Error{
				Op:  "GET",
				URL: "https://storage.example/%zz?signature=hidden",
				Err: context.DeadlineExceeded,
			}
		}),
	})
	_, err := tr.RoundTrip(context.Background(), port.Request{URL: "https://storage.example/o?signature=hidden"})
	if err == nil || strings.Contains(err.Error(), "hidden") || strings.Contains(logs.String(), "hidden") {
		t.Fatal(err, logs.String())
	}
}

func TestWireLevelsAndUnprintableBodies(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want WireLevel
	}{
		{"off", WireOff},
		{"headers", WireHeaders},
		{"true", WireHeaders},
		{"body", WireBodies},
		{"FULL", WireBodies},
	} {
		if ParseWireLevel(tc.s) != tc.want {
			t.Fatal(tc)
		}
	}
	if !strings.Contains(bodyText([]byte("secret"), "text/plain"), "not printed") {
		t.Fatal("text exposed")
	}
	if !strings.Contains(bodyText([]byte(`{"token":"secret"`), "application/json"), "not printed") {
		t.Fatal("invalid JSON exposed")
	}
	if got := bodyText([]byte(`[{"token":"secret"},"https://example/o?key=secret"]`), "application/json"); strings.Contains(got, "secret") {
		t.Fatal(got)
	}
	if safeURL("%") != "<invalid url>" {
		t.Fatal("invalid URL")
	}
	if got := safeURL("https://name:secret@example/o#secret"); strings.Contains(got, "secret") {
		t.Fatal(got)
	}
}

// Protocol errors may still arrive inside net/http's outer URL wrapper.
func TestProtocolFailureRedactsOuterURL(t *testing.T) {
	tr := newTestTransport(t, Config{
		RoundTripper: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, nil }),
	})
	_, err := tr.RoundTrip(context.Background(), port.Request{URL: "https://synthetic.invalid/path?signature=hidden"})
	if !errors.Is(err, domain.ErrMalformedResponse) || strings.Contains(err.Error(), "hidden") {
		t.Fatal(err)
	}
}

func TestWireNumbersRemainExact(t *testing.T) {
	const body = `{"id":9223372036854775807,"nested":[-9223372036854775808,1.234567890123456789],"token":"synthetic"}`
	got := bodyText([]byte(body), "application/json")
	for _, want := range []string{"9223372036854775807", "-9223372036854775808", "1.234567890123456789", "<redacted>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("wire log lost %q: %s", want, got)
		}
	}
	if strings.Contains(got, "synthetic") {
		t.Fatal("token was logged")
	}
}

func TestWireRedactsRelativeLocationsAndBodyURLs(t *testing.T) {
	for _, target := range []string{"/x?signature=hidden", "//cdn.invalid/x?signature=hidden", "x?signature=hidden", "https:hidden", "%bad?signature=hidden"} {
		var logs bytes.Buffer
		tr := newTestTransport(t, Config{WireLevel: WireBodies, Logger: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
		req, _ := http.NewRequestWithContext(t.Context(), "GET", "https://synthetic.invalid/x", nil)
		tr.log(t.Context(), "receive", req, 200, http.Header{"Location": {target}, "Content-Location": {target}, "Content-Type": {"application/json"}}, []byte(`{"url":"/x?signature=hidden"}`))
		if strings.Contains(logs.String(), "hidden") {
			t.Fatal(logs.String())
		}
	}
}
