//go:build integration

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package ws_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"

	"github.com/altessa-s/sncloud-go/internal/adapter/ws"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestLoopbackUpgradeRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_, data, err := conn.Read(r.Context())
		if err != nil {
			_ = conn.CloseNow()
			return
		}
		_ = conn.Write(r.Context(), websocket.MessageText, data)
		_ = conn.Close(websocket.StatusGoingAway, "")
	}))
	defer srv.Close()

	// Official dialer blocks loopback addresses via the SSRF dial guard.
	guarded := ws.New(ws.Config{})
	defer guarded.CloseIdleConnections()
	if _, err := guarded.Dial(context.Background(), srv.URL+"/socket.io/", nil); !errors.Is(err, domain.ErrTransport) {
		t.Fatalf("guarded loopback Dial = %v, want ErrTransport", err)
	}

	d := ws.New(ws.Config{PrivateCloud: true})
	defer d.CloseIdleConnections()

	sock, err := d.Dial(context.Background(), srv.URL+"/socket.io/", port.Header{"User-Agent": {domain.AppUserAgent}})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = sock.Close() }()

	if err := sock.Write(context.Background(), []byte("40")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := sock.Read(context.Background())
	if err != nil || string(got) != "40" {
		t.Fatalf("Read = (%q, %v)", got, err)
	}
	if _, err := sock.Read(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("going-away close = %v, want io.EOF", err)
	}
}
