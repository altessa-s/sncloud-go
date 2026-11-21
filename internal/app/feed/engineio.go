// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package feed

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strings"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

const (
	defaultPingIntervalMS = 25000
	defaultPingTimeoutMS  = 60000
	minHeartbeatMS        = 1000
	maxHeartbeatMS        = 300000

	eventFiles     = "ServerMessage"
	eventTasks     = "to-do"
	eventDigests   = "digest"
	eventRattaPing = "ratta_ping"

	argPing      = "ping"
	argReceived  = "Received"
	codeOK       = "200"
	socketIOCode = "socket.io"
)

type frameKind int

const (
	frameIgnored frameKind = iota
	frameOpen
	frameConnected
	frameClosed
	frameEvent
)

type parsedFrame struct {
	kind      frameKind
	interval  time.Duration
	timeout   time.Duration
	eventName string
	eventArg  string
}

type openPayload struct {
	PingInterval *int `json:"pingInterval"`
	PingTimeout  *int `json:"pingTimeout"`
}

type eventEnvelope struct {
	Code *string `json:"code"`
	Msg  *string `json:"msg"`
}

type eventBatch[T any] struct {
	Data []T `json:"data"`
}

// parseFrame decodes a single Engine.IO 3 / Socket.IO text frame by its leading
// packet type digits.
func parseFrame(raw string) (parsedFrame, error) {
	if raw == "" {
		return parsedFrame{kind: frameIgnored}, nil
	}
	rest := raw[1:]
	switch raw[0] {
	case '0':
		var open openPayload
		_ = json.Unmarshal([]byte(rest), &open)
		iv := defaultPingIntervalMS
		if open.PingInterval != nil {
			iv = *open.PingInterval
		}
		to := defaultPingTimeoutMS
		if open.PingTimeout != nil {
			to = *open.PingTimeout
		}
		return parsedFrame{
			kind:     frameOpen,
			interval: time.Duration(clampMS(iv)) * time.Millisecond,
			timeout:  time.Duration(clampMS(to)) * time.Millisecond,
		}, nil
	case '1':
		return parsedFrame{kind: frameClosed}, nil
	case '4':
		if rest == "" {
			return parsedFrame{kind: frameIgnored}, nil
		}
		switch rest[0] {
		case '0':
			return parsedFrame{kind: frameConnected}, nil
		case '1':
			return parsedFrame{kind: frameClosed}, nil
		case '2':
			return parseSocketEvent(rest[1:])
		case '4':
			return parsedFrame{}, &domain.RejectedError{Code: socketIOCode, Message: rest[1:]}
		default:
			return parsedFrame{kind: frameIgnored}, nil
		}
	default:
		return parsedFrame{kind: frameIgnored}, nil
	}
}

const errNotSocketEvent = "a socket.io event that is not one"

func parseSocketEvent(body string) (parsedFrame, error) {
	idx := strings.IndexByte(body, '[')
	if idx < 0 {
		return parsedFrame{}, &domain.MalformedResponseError{What: errNotSocketEvent}
	}
	var elems []jsontext.Value
	if err := json.Unmarshal([]byte(body[idx:]), &elems); err != nil || len(elems) == 0 {
		return parsedFrame{}, &domain.MalformedResponseError{What: errNotSocketEvent}
	}
	var name string
	if err := json.Unmarshal(elems[0], &name); err != nil {
		return parsedFrame{}, &domain.MalformedResponseError{What: errNotSocketEvent}
	}
	var arg string
	if len(elems) > 1 {
		_ = json.Unmarshal(elems[1], &arg)
	}
	return parsedFrame{
		kind:      frameEvent,
		eventName: name,
		eventArg:  arg,
	}, nil
}

func formatEventPacket(name, arg string) ([]byte, error) {
	raw, err := json.Marshal([]string{name, arg})
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(raw)+len("42"))
	out = append(out, '4', '2')
	out = append(out, raw...)
	return out, nil
}

func parseAnnouncement(name, arg string) (*domain.Change, error) {
	if name != eventFiles && name != eventTasks && name != eventDigests {
		return nil, nil //nolint:nilnil // nil *Change signals an unannounced/heartbeat event.
	}
	if !strings.HasPrefix(arg, "{") {
		return nil, nil //nolint:nilnil // non-JSON string args like "true" or "Received" carry no change.
	}
	data := []byte(arg)
	var env eventEnvelope
	_ = json.Unmarshal(data, &env)
	var code string
	if env.Code != nil {
		code = *env.Code
	}
	if code != codeOK {
		var msg string
		if env.Msg != nil {
			msg = *env.Msg
		}
		if strings.Contains(strings.ToLower(msg), "token") {
			return nil, domain.ErrSessionExpired
		}
		return nil, &domain.RejectedError{Code: code, Message: msg}
	}
	switch name {
	case eventFiles:
		return decodeBatch(data, name, domain.ChangeFiles, func(c *domain.Change, v []domain.FileChange) { c.Files = v })
	case eventTasks:
		return decodeBatch(data, name, domain.ChangeTasks, func(c *domain.Change, v []domain.TaskChange) { c.Tasks = v })
	default:
		return decodeBatch(data, name, domain.ChangeDigests, func(c *domain.Change, v []domain.DigestChange) { c.Digests = v })
	}
}

func decodeBatch[T any](
	data []byte,
	name string,
	typ domain.ChangeType,
	assign func(*domain.Change, []T),
) (*domain.Change, error) {
	var batch eventBatch[T]
	if err := json.Unmarshal(data, &batch); err != nil {
		return nil, &domain.MalformedResponseError{What: fmt.Sprintf("a %s announcement: %v", name, err)}
	}
	if batch.Data == nil {
		batch.Data = []T{}
	}
	out := &domain.Change{Type: typ}
	assign(out, batch.Data)
	return out, nil
}

func clampMS(ms int) int {
	return max(minHeartbeatMS, min(ms, maxHeartbeatMS))
}
