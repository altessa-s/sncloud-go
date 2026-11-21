// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package porttest

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestSocketAndDialer(t *testing.T) {
	ctx := context.Background()
	sock := NewSocket("f1")
	f, err := sock.Read(ctx)
	if err != nil || string(f) != "f1" {
		t.Fatalf("Read 1 = (%q, %v)", f, err)
	}
	if _, err := sock.Read(ctx); !errors.Is(err, io.EOF) {
		t.Fatalf("Read EOF = %v", err)
	}
	if err := sock.Write(ctx, []byte("out")); err != nil || len(sock.Sent()) != 1 {
		t.Fatalf("Write = %v, sent=%v", err, sock.Sent())
	}
	wantErr := errors.New("write-fail")
	sock.SetWriteError(wantErr)
	if !errors.Is(sock.Write(ctx, []byte("x")), wantErr) {
		t.Fatal("expected write error")
	}

	quiet := NewQuietSocket()
	done := make(chan string, 1)
	go func() {
		b, _ := quiet.Read(ctx)
		done <- string(b)
	}()
	quiet.Push("pushed")
	if got := <-done; got != "pushed" {
		t.Fatalf("quiet Read = %q", got)
	}
	readFail := errors.New("read-fail")
	quiet.SetReadError(readFail)
	if _, err := quiet.Read(ctx); !errors.Is(err, readFail) {
		t.Fatalf("SetReadError = %v", err)
	}
	_ = quiet.Close()
	<-quiet.ClosedChan()
	if !quiet.Closed() {
		t.Fatal("expected Closed() == true")
	}
	quiet.Push("ignored")
	if _, err := quiet.Read(ctx); err == nil {
		t.Fatal("expected error reading closed socket")
	}
	if err := quiet.Write(ctx, []byte("x")); err == nil {
		t.Fatal("expected error writing closed socket")
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := NewQuietSocket().Read(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal("expected canceled Read")
	}
	blockCtx, blockCancel := context.WithCancel(ctx)
	waiting := NewQuietSocket()
	waitDone := make(chan error, 1)
	go func() {
		_, err := waiting.Read(blockCtx)
		waitDone <- err
	}()
	blockCancel()
	if !errors.Is(<-waitDone, context.Canceled) {
		t.Fatal("expected blocked Read cancellation")
	}
	closing := NewQuietSocket()
	closeDone := make(chan error, 1)
	go func() {
		_, err := closing.Read(ctx)
		closeDone <- err
	}()
	_ = closing.Close()
	if <-closeDone == nil {
		t.Fatal("expected blocked Read to fail on Close")
	}
	if err := NewSocket().Write(canceled, []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatal("expected canceled Write")
	}

	dialer := &SocketDialer{
		Steps: []DialStep{
			{Socket: sock},
			{Run: func(_ context.Context, _ string, _ port.Header) (port.Socket, error) { return sock, nil }},
		},
	}
	h := port.Header{"User-Agent": {"agent"}}
	if s, err := dialer.Dial(ctx, "wss://example.com/socket.io/", h); err != nil || s != sock {
		t.Fatalf("Dial 1 = (%v, %v)", s, err)
	}
	if s, err := dialer.Dial(ctx, "wss://example.com/socket.io/", h); err != nil || s != sock {
		t.Fatalf("Dial 2 = (%v, %v)", s, err)
	}
	if _, err := dialer.Dial(ctx, "wss://example.com/socket.io/", h); err == nil {
		t.Fatal("expected exhausted dialer error")
	}
	if len(dialer.Calls()) != 3 {
		t.Fatalf("Calls = %d", len(dialer.Calls()))
	}
}
