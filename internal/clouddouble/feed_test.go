// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

type socketPipeAddr struct{}

func (socketPipeAddr) Network() string { return "pipe" }
func (socketPipeAddr) String() string  { return "pipe" }

type socketPipeListener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func (l *socketPipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *socketPipeListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *socketPipeListener) Addr() net.Addr { return socketPipeAddr{} }

func dialSocketDouble(t *testing.T, h http.HandlerFunc, query string) *websocket.Conn {
	t.Helper()
	ln := &socketPipeListener{conns: make(chan net.Conn), closed: make(chan struct{})}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			c1, c2 := net.Pipe()
			select {
			case ln.conns <- c2:
				return c1, nil
			case <-ln.closed:
				_ = c1.Close()
				_ = c2.Close()
				return nil, net.ErrClosed
			case <-ctx.Done():
				_ = c1.Close()
				_ = c2.Close()
				return nil, ctx.Err()
			}
		},
	}
	t.Cleanup(func() {
		tr.CloseIdleConnections()
		_ = srv.Close()
		_ = ln.Close()
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws://feed.example.invalid/socket.io/?"+query, &websocket.DialOptions{
		HTTPClient: &http.Client{Transport: tr},
	})
	if err != nil {
		t.Fatalf("websocket.Dial(%s): %v", query, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func readSocketText(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, raw, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("conn.Read: %v", err)
	}
	return string(raw)
}

func writeSocketText(t *testing.T, conn *websocket.Conn, frame string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, []byte(frame)); err != nil {
		t.Fatalf("conn.Write(%s): %v", frame, err)
	}
}

func decodeEventFrame(t *testing.T, frame string) (string, map[string]any) {
	t.Helper()
	if !strings.HasPrefix(frame, "42") {
		t.Fatalf("expected 42 event frame, got %q", frame)
	}
	var outer []string
	if err := json.Unmarshal([]byte(frame[2:]), &outer); err != nil || len(outer) != 2 {
		t.Fatalf("outer event %q: %v", frame, err)
	}
	var inner map[string]any
	if err := json.Unmarshal([]byte(outer[1]), &inner); err != nil {
		t.Fatalf("inner payload %q: %v", outer[1], err)
	}
	return outer[0], inner
}

func TestSocketIOHandshakeValidation(t *testing.T) {
	for _, flavor := range []Flavor{Official, Private} {
		t.Run(string(flavor), func(t *testing.T) {
			_, h := serviceFixture(t, flavor)

			for _, rawURL := range []string{"/socket.io/", "/socket.io/?EIO=4&transport=websocket", "/socket.io/?EIO=3&transport=polling"} {
				r := httptest.NewRequest(http.MethodGet, rawURL, nil)
				w := httptest.NewRecorder()
				h(w, r)
				if w.Code != http.StatusBadRequest {
					t.Fatalf("%s: status %d, want 400", rawURL, w.Code)
				}
			}

			// A non-hijackable ResponseWriter fails websocket.Accept cleanly.
			noHijack := httptest.NewRequest(http.MethodGet, "/socket.io/?EIO=3&transport=websocket", nil)
			h(httptest.NewRecorder(), noHijack)

			for _, tc := range []struct {
				name, token, sign, wantSubstr string
			}{
				{
					name:       "bad signature",
					token:      "synthetic-token",
					sign:       "wrong",
					wantSubstr: "signature verification failed",
				},
				{
					name:       "revoked token",
					token:      "revoked-token",
					sign:       domain.FeedSignature("revoked-token_DESKTOP_nonce"),
					wantSubstr: "token rejected",
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					q := "EIO=3&transport=websocket&token=" + tc.token + "&type=DESKTOP&random=nonce&sign=" + tc.sign
					conn := dialSocketDouble(t, h, q)
					open := readSocketText(t, conn)
					if !strings.HasPrefix(open, "0{") || readSocketText(t, conn) != "40" {
						t.Fatalf("unexpected open handshake: %q", open)
					}
					evName, payload := decodeEventFrame(t, readSocketText(t, conn))
					if evName != eventFiles || payload[keyCode] != "403" || !strings.Contains(payload["msg"].(string), tc.wantSubstr) {
						t.Fatalf("expected 403 %q, got %s %v", tc.wantSubstr, evName, payload)
					}
				})
			}
		})
	}
}

func TestSocketIOPingBatchesSuppressionAndClose(t *testing.T) {
	for _, flavor := range []Flavor{Official, Private} {
		t.Run(string(flavor), func(t *testing.T) {
			c, h := serviceFixture(t, flavor)

			sign := domain.FeedSignature("synthetic-token_DESKTOP_r1")
			conn := dialSocketDouble(t, h, "EIO=3&transport=websocket&token=synthetic-token&type=DESKTOP&random=r1&sign="+sign)
			if !strings.HasPrefix(readSocketText(t, conn), "0{") || readSocketText(t, conn) != "40" {
				t.Fatal("missing Engine.IO/Socket.IO open frames")
			}

			// Engine.IO ping and malformed Socket.IO frames do not disrupt the stream.
			writeSocketText(t, conn, "2")
			if got := readSocketText(t, conn); got != "3" {
				t.Fatalf("Engine.IO pong = %q, want 3", got)
			}
			for _, ignored := range []string{"42not-json", `42["one-element"]`, `42["unknown","noop"]`} {
				writeSocketText(t, conn, ignored)
			}
			writeSocketText(t, conn, "2")
			if got := readSocketText(t, conn); got != "3" {
				t.Fatalf("pong after malformed frames = %q, want 3", got)
			}

			postWithChannel := func(channel, path, body string) {
				t.Helper()
				r := httptest.NewRequest(http.MethodPost, "/api/"+path, strings.NewReader(body))
				r.Header.Set("version", domain.ProtocolVersion)
				r.Header.Set("equipmentNo", "synthetic-device")
				r.Header.Set("channel", channel)
				r.Header.Set(domain.AccessTokenHeader, "synthetic-token")
				rec := httptest.NewRecorder()
				h(rec, r)
				if rec.Code != http.StatusOK {
					t.Fatalf("%s status = %d", path, rec.Code)
				}
			}

			// Queue mutations before ratta_ping: DESKTOP self-originated events must be suppressed.
			postWithChannel("synthetic-token_DESKTOP_1", domain.PathTaskList, `{"taskListId":"self-list","title":"Self"}`)
			postWithChannel("synthetic-token_TABLET_1", domain.PathTaskList, `{"taskListId":"remote-list","title":"Remote"}`)
			postWithChannel("synthetic-token_TABLET_1", domain.PathAddDigestGroup, `{"uniqueIdentifier":"remote-grp","name":"Remote Group"}`)
			c.Announce("DESKTOP", eventFiles, map[string]any{"messageType": "DOWNLOADFILE", "id": "1"})
			c.Announce("TABLET", eventFiles, map[string]any{"messageType": "DOWNLOADFILE", "id": "42"})

			writeSocketText(t, conn, `42["ratta_ping","ping"]`)
			if got := readSocketText(t, conn); got != `42["ratta_ping","Received"]` {
				t.Fatalf("ratta_ping ack = %q", got)
			}

			for _, wantEvent := range []string{eventFiles, eventTodo, eventDigest} {
				evName, payload := decodeEventFrame(t, readSocketText(t, conn))
				if evName != wantEvent || payload[keyCode] != "200" {
					t.Fatalf("want %s 200 batch, got %s %v", wantEvent, evName, payload)
				}
				items := payload["data"].([]any)
				if len(items) != 1 {
					t.Fatalf("%s batch items = %v, want 1 remote item", wantEvent, items)
				}
			}

			c.Announce("TABLET", eventTodo, map[string]any{keyMessageType: msgQuery, keyTaskListID: "queued-after-send"})
			// Acknowledge ServerMessage and to-do only; unacknowledged digest replays on the next ratta_ping.
			writeSocketText(t, conn, `42["ServerMessage","Received"]`)
			writeSocketText(t, conn, `42["to-do","Received"]`)
			writeSocketText(t, conn, `42["ratta_ping","ping"]`)
			if got := readSocketText(t, conn); got != `42["ratta_ping","Received"]` {
				t.Fatalf("second ratta_ping ack = %q", got)
			}
			queuedName, queuedPayload := decodeEventFrame(t, readSocketText(t, conn))
			if queuedName != eventTodo || queuedPayload["data"].([]any)[0].(map[string]any)[keyTaskListID] != "queued-after-send" {
				t.Fatalf("ack lost queued change: %s %v", queuedName, queuedPayload)
			}
			replayedName, _ := decodeEventFrame(t, readSocketText(t, conn))
			if replayedName != eventDigest {
				t.Fatalf("expected unacknowledged digest replay, got %s", replayedName)
			}
			writeSocketText(t, conn, `42["digest","Received"]`)
			writeSocketText(t, conn, `42["to-do","Received"]`)

			// New changes remain queued until the next application heartbeat.
			c.Announce("", eventTodo, map[string]any{keyMessageType: msgQuery, keyTaskListID: "live"})
			writeSocketText(t, conn, "2")
			if got := readSocketText(t, conn); got != "3" {
				t.Fatalf("change escaped before heartbeat: %s", got)
			}
			writeSocketText(t, conn, `42["ratta_ping","ping"]`)
			if got := readSocketText(t, conn); got != `42["ratta_ping","Received"]` {
				t.Fatal(got)
			}
			liveName, livePayload := decodeEventFrame(t, readSocketText(t, conn))
			if liveName != eventTodo || livePayload["data"].([]any)[0].(map[string]any)[keyTaskListID] != "live" {
				t.Fatalf("live push = %s %v", liveName, livePayload)
			}

			// Closing the emulator tears down active subscribers and wakes parked reads.
			c.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if _, _, err := conn.Read(ctx); err == nil {
				t.Fatal("expected read error after Cloud.Close")
			}
		})
	}
}
