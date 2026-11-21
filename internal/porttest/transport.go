// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package porttest supplies concurrency-safe port doubles.
package porttest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Synthetic downloaded files remain private to their owner.
const privateFileMode = 0600

// Step is one scripted exchange; Run may hold it on a test gate.
type Step struct {
	Response port.Response
	Err      error
	Run      func(context.Context, port.Request) (port.Response, error)
}

// Transport records requests and consumes its script under one lock.
type Transport struct {
	mu    sync.Mutex
	Steps []Step
	calls []port.Request
}

// RoundTrip claims a step before running it so held calls cannot reorder replies.
func (t *Transport) RoundTrip(ctx context.Context, r port.Request) (port.Response, error) {
	t.mu.Lock()
	r.Header = r.Header.Clone()
	r.Body = append([]byte(nil), r.Body...)
	t.calls = append(t.calls, r)
	if len(t.Steps) == 0 {
		t.mu.Unlock()
		return port.Response{}, fmt.Errorf("porttest: unexpected request %s", safeURL(r.URL))
	}
	s := t.Steps[0]
	t.Steps = t.Steps[1:]
	t.mu.Unlock()
	if s.Run != nil {
		return s.Run(ctx, r)
	}
	return s.Response, s.Err
}

// Calls returns a detached snapshot.
func (t *Transport) Calls() []port.Request {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := append([]port.Request(nil), t.calls...)
	for i := range out {
		out[i].Header = out[i].Header.Clone()
		out[i].Body = append([]byte(nil), out[i].Body...)
	}
	return out
}

// OK constructs a flat success envelope from extra JSON fields.
func OK(fields string) Step {
	if fields != "" {
		fields = "," + fields
	}
	return Step{Response: port.Response{Status: http.StatusOK, Body: []byte(`{"success":true` + fields + `}`)}}
}

// Refused constructs a service refusal.
func Refused(code string) Step {
	return Step{
		Response: port.Response{Status: http.StatusOK, Body: []byte(fmt.Sprintf(`{"success":false,"errorCode":%q}`, code))},
	}
}

// Upload makes the file's bytes visible to a scripted step. Real transfers use the streaming HTTP adapter.
func (t *Transport) Upload(ctx context.Context, r port.Request) (port.Response, error) {
	b, err := os.ReadFile(r.File)
	if err != nil {
		return port.Response{}, err
	}
	if r.Section != nil {
		offset, length := r.Section.Offset, r.Section.Length
		if offset < 0 || length < 0 {
			return port.Response{}, fmt.Errorf("invalid upload section %d+%d: %w", offset, length, os.ErrInvalid)
		}
		if offset > int64(len(b)) || length > int64(len(b))-offset {
			return port.Response{}, fmt.Errorf("upload source shorter than section %d+%d: %w", offset, length, io.ErrUnexpectedEOF)
		}
		b = b[offset : offset+length]
	}
	r.Body = b
	out, err := t.RoundTrip(ctx, r)
	if err == nil && r.Progress != nil {
		r.Progress(int64(len(b)), int64(len(b)))
	}
	return out, err
}

// Download materializes a scripted response and hands ownership to the caller.
func (t *Transport) Download(ctx context.Context, r port.Request) (port.Response, error) {
	out, err := t.RoundTrip(ctx, r)
	if err != nil {
		return out, err
	}
	if r.Limit > 0 && int64(len(out.Body)) > r.Limit {
		return out, &domain.OversizedResponseError{Bytes: int64(len(out.Body)), Limit: r.Limit}
	}
	var f *os.File
	if r.Dest == "" {
		f, err = os.CreateTemp("", "sncloud-double-*")
	} else {
		f, err = os.OpenFile(r.Dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, privateFileMode)
	}
	if err != nil {
		return out, err
	}
	_, writeErr := f.Write(out.Body)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		// Best effort cleanup preserves the original write or close failure.
		_ = os.Remove(f.Name())
		if writeErr != nil {
			return out, writeErr
		}
		return out, closeErr
	}
	out.File = f.Name()
	if r.Progress != nil {
		r.Progress(int64(len(out.Body)), int64(len(out.Body)))
	}
	return out, nil
}

var _ port.Transport = (*Transport)(nil)
