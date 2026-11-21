// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/coder/websocket"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

type feedSub struct {
	deviceType string
	conn       *websocket.Conn
	mu         sync.Mutex
	writeMu    sync.Mutex
	queues     map[string][]map[string]any
	sent       map[string]int
	closed     bool
}

func (s *feedSub) writeText(ctx context.Context, msg []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.Write(ctx, websocket.MessageText, msg)
}

func (s *feedSub) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	_ = s.conn.CloseNow()
}

// Announce queues a synthetic change-feed announcement for connected subscribers
// whose device type differs from originType. Event is one of "ServerMessage",
// "to-do" or "digest".
func (c *Cloud) Announce(originType, event string, item map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enqueueChangeLocked(originType, event, item)
}

func (c *Cloud) enqueueChangeLocked(originType, event string, item map[string]any) {
	for sub := range c.state.subs {
		if originType != "" && sub.deviceType == originType {
			continue
		}
		sub.mu.Lock()
		sub.queues[event] = append(sub.queues[event], item)
		sub.mu.Unlock()
	}
}

func (c *Cloud) serveSocketIO(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("EIO") != "3" || q.Get("transport") != "websocket" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ctx := r.Context()
	_ = conn.Write(ctx, websocket.MessageText, []byte(`0{"sid":"synthetic-sid","upgrades":["websocket"],"pingInterval":5000,"pingTimeout":25000}`))
	_ = conn.Write(ctx, websocket.MessageText, []byte("40"))

	token := q.Get("token")
	kind := q.Get("type")
	random := q.Get("random")
	sign := q.Get("sign")

	if want := domain.FeedSignature(token + "_" + kind + "_" + random); sign != want {
		_ = conn.Write(ctx, websocket.MessageText, encodeSocketEvent(eventFiles, map[string]any{
			keyCode: "403",
			"msg":   "signature verification failed",
		}))
		_ = conn.Close(websocket.StatusNormalClosure, "")
		return
	}

	c.mu.Lock()
	validToken := c.tokens[token]
	if !validToken {
		c.mu.Unlock()
		_ = conn.Write(ctx, websocket.MessageText, encodeSocketEvent(eventFiles, map[string]any{
			keyCode: "403",
			"msg":   "token rejected",
		}))
		_ = conn.Close(websocket.StatusNormalClosure, "")
		return
	}
	sub := &feedSub{
		deviceType: kind,
		conn:       conn,
		queues:     make(map[string][]map[string]any),
		sent:       make(map[string]int),
	}
	c.state.subs[sub] = struct{}{}
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.state.subs, sub)
		c.mu.Unlock()
		sub.close()
	}()

	c.readSocketLoop(ctx, sub)
}

func (c *Cloud) readSocketLoop(ctx context.Context, sub *feedSub) {
	for {
		_, data, err := sub.conn.Read(ctx)
		if err != nil {
			return
		}
		frame := string(data)
		switch {
		case frame == "2":
			_ = sub.writeText(ctx, []byte("3"))
		case strings.HasPrefix(frame, "42"):
			var arr []string
			if json.Unmarshal([]byte(frame[2:]), &arr) != nil || len(arr) < 2 {
				continue
			}
			evName, evArg := arr[0], arr[1]
			if evName == "ratta_ping" && evArg == "ping" {
				_ = sub.writeText(ctx, []byte(`42["ratta_ping","Received"]`))
				flushSubscriberQueues(ctx, sub)
			} else if evArg == "Received" {
				sub.mu.Lock()
				sub.queues[evName] = sub.queues[evName][sub.sent[evName]:]
				delete(sub.sent, evName)
				sub.mu.Unlock()
			}
		}
	}
}

func flushSubscriberQueues(ctx context.Context, sub *feedSub) {
	sub.mu.Lock()
	snapshot := make(map[string][]map[string]any, len(sub.queues))
	for k, v := range sub.queues {
		if len(v) > 0 {
			// Only this prefix belongs to the next acknowledgement. Later changes stay queued.
			if sub.sent[k] == 0 {
				sub.sent[k] = len(v)
			}
			snapshot[k] = append([]map[string]any(nil), v[:sub.sent[k]]...)
		}
	}
	sub.mu.Unlock()

	for _, ev := range []string{eventFiles, eventTodo, eventDigest} {
		items := snapshot[ev]
		if len(items) == 0 {
			continue
		}
		pkt := encodeSocketEvent(ev, map[string]any{
			keyCode: "200",
			"data":  items,
		})
		_ = sub.writeText(ctx, pkt)
	}
}

func encodeSocketEvent(name string, payload map[string]any) []byte {
	inner, _ := json.Marshal(payload)
	outer, _ := json.Marshal([]string{name, string(inner)})
	return append([]byte("42"), outer...)
}
