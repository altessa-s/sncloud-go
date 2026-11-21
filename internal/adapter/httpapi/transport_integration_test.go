//go:build integration

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestStripsCredentialsOnARedirect(t *testing.T) {
	var got http.Header
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hop" {
			http.Redirect(w, r, "/final", 302)
			return
		}
		got = r.Header.Clone()
		// A canceled client may close the connection; the response needs no retry.
		_, _ = w.Write([]byte("ok"))
	}))
	defer target.Close()
	start := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL+"/hop", 302) }))
	defer start.Close()
	tr := newTestTransport(t, Config{})
	h := port.Header{}
	for _, k := range []string{"x-access-token", "channel", "Cookie", "Authorization", "X-XSRF-TOKEN"} {
		h[k] = []string{"synthetic-secret"}
	}
	h["User-Agent"] = []string{"synthetic-agent"}
	resp, err := tr.RoundTrip(context.Background(), port.Request{URL: start.URL, Header: h})
	if err != nil || string(resp.Body) != "ok" {
		t.Fatal(resp, err)
	}
	for k := range h {
		if k != "User-Agent" && got.Get(k) != "" {
			t.Fatalf("%s leaked: %v", k, got)
		}
	}
	if got.Get("User-Agent") != "synthetic-agent" {
		t.Fatal(got)
	}
}
func TestRefusesToCarryABodyAcrossOrigins(t *testing.T) {
	var reached atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true) }))
	defer target.Close()
	start := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer start.Close()
	tr := newTestTransport(t, Config{})
	r, err := tr.RoundTrip(
		context.Background(),
		port.Request{URL: start.URL, Method: "POST", Body: []byte(`{"password":"synthetic-digest"}`)},
	)
	if err != nil || r.Status != 307 || reached.Load() {
		t.Fatal(r, err, reached.Load())
	}
}
func TestReportsAndHandsOverAFile(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 512<<10)
	// A canceled client may close the connection; the response needs no retry.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(payload) }))
	defer srv.Close()
	tr := newTestTransport(t, Config{})
	var reports []int64
	out, err := tr.Download(context.Background(), port.Request{
		URL:      srv.URL,
		Limit:    1 << 20,
		Progress: func(done, total int64) { reports = append(reports, done) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(out.File)
	got, err := os.ReadFile(out.File)
	if err != nil || !bytes.Equal(got, payload) || len(reports) < 2 || reports[len(reports)-1] != int64(len(payload)) {
		t.Fatal(len(got), reports, err)
	}
}
func TestUploadsFromDisk(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 200000)
	file := filepath.Join(t.TempDir(), "upload")
	if err := os.WriteFile(file, payload, 0600); err != nil {
		t.Fatal(err)
	}
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		received, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		// A canceled client may close the connection; the response needs no retry.
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer srv.Close()
	tr := newTestTransport(t, Config{})
	var done int64
	resp, err := tr.Upload(context.Background(), port.Request{
		URL:      srv.URL,
		Method:   "PUT",
		File:     file,
		Progress: func(n, total int64) { done = n },
	})
	if err != nil || resp.Status != 200 || !bytes.Equal(received, payload) || done != int64(len(payload)) {
		t.Fatal(err, resp.Status, len(received), done)
	}
}
func TestHeaderTimeoutAndHTTP2Bounds(t *testing.T) {
	tr := newTestTransport(t, Config{HeaderTimeout: 20 * time.Millisecond})
	base := tr.memBase
	if base.HTTP2.SendPingTimeout != 15*time.Second || base.HTTP2.PingTimeout != 15*time.Second {
		t.Fatal("HTTP/2 bounds differ from the house transport")
	}
	upload := tr.upBase
	download := tr.dlBase
	if upload.ResponseHeaderTimeout != 5*time.Minute || download.MaxConnsPerHost != 6 || tr.config.IdleReadTimeout != 45*time.Second {
		t.Fatal("request bounds differ from house transport")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	_, err := tr.RoundTrip(context.Background(), port.Request{URL: srv.URL})
	var e *domain.TransportError
	if !errors.As(err, &e) || e.Reason != domain.ReasonTimedOut {
		t.Fatal(err)
	}
}
func TestUploadFollowsOnlySameOriginReplay(t *testing.T) {
	file := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(file, []byte("synthetic bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", 307)
			return
		}
		b, err := io.ReadAll(r.Body)
		if err != nil || string(b) != "synthetic bytes" {
			w.WriteHeader(400)
			return
		}
		// A canceled client may close the connection; the response needs no retry.
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	tr := newTestTransport(t, Config{})
	out, err := tr.Upload(context.Background(), port.Request{URL: srv.URL + "/start", Method: "PUT", File: file})
	if err != nil || out.Status != 200 || string(out.Body) != "ok" {
		t.Fatal(out, err)
	}
}
