// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package ws

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"

	"github.com/coder/websocket"

	"github.com/altessa-s/sncloud-go/internal/adapter/neterr"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

type socket struct {
	conn   *websocket.Conn
	closed atomic.Bool
}

// Read returns one complete frame payload, io.EOF on a normal or going-away
// WebSocket close frame, OversizedResponseError when a frame exceeds 4 MiB, or
// a URL-free classified transport error.
func (s *socket) Read(ctx context.Context) ([]byte, error) {
	_, data, err := s.conn.Read(ctx)
	if err == nil {
		return data, nil
	}
	if errors.Is(err, websocket.ErrMessageTooBig) {
		return nil, &domain.OversizedResponseError{Bytes: readLimit + 1, Limit: readLimit}
	}
	if status := websocket.CloseStatus(err); status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
		return nil, io.EOF
	}
	return nil, neterr.Classify(ctx, err)
}

// Write transmits a text frame, classifying any failure without retaining URLs.
func (s *socket) Write(ctx context.Context, data []byte) error {
	if err := s.conn.Write(ctx, websocket.MessageText, data); err != nil {
		return neterr.Classify(ctx, err)
	}
	return nil
}

// Close terminates the underlying connection immediately so a silent peer
// cannot stall teardown on an unanswered WebSocket close handshake.
func (s *socket) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	if err := s.conn.CloseNow(); err != nil && !errors.Is(err, net.ErrClosed) {
		return neterr.Classify(context.Background(), err)
	}
	return nil
}

var _ port.Socket = (*socket)(nil)
