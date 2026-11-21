// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"context"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/coder/websocket"

	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

type pipeListener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

func TestFacadeFeed(t *testing.T) {
	ln := &pipeListener{conns: make(chan net.Conn), closed: make(chan struct{})}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx := r.Context()
		if conn.Write(ctx, websocket.MessageText, []byte(`0{"sid":"s1","upgrades":["websocket"],"pingInterval":5000,"pingTimeout":25000}`)) != nil {
			return
		}
		if conn.Write(ctx, websocket.MessageText, []byte("40")) != nil {
			return
		}
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
		_ = conn.Write(ctx, websocket.MessageText, []byte(`42["ratta_ping","Received"]`))
		_ = conn.Write(ctx, websocket.MessageText, []byte(`42["to-do","{\"code\":\"200\",\"data\":[{\"messageType\":\"QUERY\",\"taskListId\":\"1\"}]}"]`))
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})}
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

	store := &porttest.Secrets{}
	if err := store.Keep("synthetic-token", sncloud.AccessToken); err != nil {
		t.Fatal(err)
	}
	c, err := sncloud.New(
		store,
		sncloud.WithEndpoints(sncloud.PrivateCloud("http://feed.example.test", false)),
		sncloud.WithRoundTripper(tr),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	feedView := c.Feed()
	var seen []sncloud.Change
	for ch, err := range feedView.Changes(context.Background()) {
		if err != nil {
			t.Fatalf("Changes: %v", err)
		}
		seen = append(seen, ch)
	}
	if len(seen) != 2 || seen[0].Type != sncloud.ChangeConnected || seen[1].Type != sncloud.ChangeTasks {
		t.Fatalf("Changes = %+v", seen)
	}
	if seen[1].Tasks[0].Kind != sncloud.ChangeTasksChanged || seen[1].Tasks[0].Kind.String() != "QUERY" {
		t.Fatalf("TaskChange = %+v", seen[1].Tasks[0])
	}

	ctx := t.Context()
	var watchTypes []sncloud.ChangeType
	for ch, err := range feedView.Watch(ctx) {
		if err != nil {
			t.Fatalf("Watch: %v", err)
		}
		watchTypes = append(watchTypes, ch.Type)
		if len(watchTypes) == 3 {
			break
		}
	}
	if len(watchTypes) != 3 || watchTypes[0] != sncloud.ChangeConnected || watchTypes[1] != sncloud.ChangeTasks || watchTypes[2] != sncloud.ChangeConnected {
		t.Fatalf("Watch = %v", watchTypes)
	}
}
