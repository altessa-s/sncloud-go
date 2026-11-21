// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package channel

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestXSRFRotationAndOne403Refresh(t *testing.T) {
	c, tr := makeChannel(handshake("first"), porttest.Step{Response: port.Response{Status: 403}}, handshake("second"), porttest.OK(""))
	if err := c.Call(context.Background(), port.Call{Path: "x", Web: true, Body: struct{}{}}, nil); err != nil {
		t.Fatal(err)
	}
	calls := tr.Calls()
	if len(calls) != 4 || calls[1].Header.Get(domain.XSRFHeader) != "first" || calls[3].Header.Get(domain.XSRFHeader) != "second" {
		t.Fatal(calls)
	}
	if calls[3].Header.Get("Cookie") != "SESSION=synthetic; XSRF-TOKEN=second" {
		t.Fatal(calls[3].Header)
	}
	c, tr = makeChannel(
		handshake("a"),
		porttest.Step{Response: port.Response{Status: 403}},
		handshake("b"),
		porttest.Step{Response: port.Response{Status: 403}},
	)
	if err := c.Call(context.Background(), port.Call{Path: "x", Web: true}, nil); !errors.Is(err, domain.ErrHTTP) {
		t.Fatal(err)
	}
	if len(tr.Calls()) != 4 {
		t.Fatal("403 loop")
	}
}
func TestCoalescesConcurrentHandshakes(t *testing.T) {
	arrived := make(chan struct{})
	release := make(chan struct{})
	step := handshake("shared")
	step.Run = func(ctx context.Context, r port.Request) (port.Response, error) {
		close(arrived)
		<-release
		return step.Response, nil
	}
	steps := []porttest.Step{step}
	for range 20 {
		steps = append(steps, porttest.OK(""))
	}
	c, tr := makeChannel(steps...)
	errs := make(chan error, 20)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { errs <- c.Call(context.Background(), port.Call{Path: "x", Web: true}, nil) })
	}
	<-arrived
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	handshakes := 0
	for _, r := range tr.Calls() {
		if strings.HasSuffix(r.URL, "csrf") {
			handshakes++
		}
	}
	if handshakes != 1 {
		t.Fatal(handshakes)
	}
}
func TestEndsAHandshakeFromAnEndedConversation(t *testing.T) {
	arrived := make(chan struct{})
	release := make(chan struct{})
	step := handshake("stale")
	step.Run = func(ctx context.Context, r port.Request) (port.Response, error) {
		close(arrived)
		<-release
		return step.Response, nil
	}
	c, tr := makeChannel(step)
	done := make(chan error, 1)
	go func() { done <- c.Call(context.Background(), port.Call{Path: "x", Web: true}, nil) }()
	<-arrived
	c.Forget()
	close(release)
	if err := <-done; !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.xsrf != "" || len(c.cookies) != 0 || len(tr.Calls()) != 1 {
		t.Fatal("stale browser state retained")
	}
}
func TestHandshakeFailuresAndHeaderFallback(t *testing.T) {
	for _, tc := range []struct {
		steps []porttest.Step
		want  error
	}{
		{[]porttest.Step{{Response: port.Response{Status: 200}}}, domain.ErrMalformedResponse},
		{[]porttest.Step{{Err: domain.ErrInvalidCredentials}}, domain.ErrInvalidCredentials},
		{[]porttest.Step{{Response: port.Response{Status: 401}}}, domain.ErrSessionExpired},
		{
			[]porttest.Step{
				{Response: port.Response{Status: 200, Header: port.Header{domain.XSRFHeader: {"header-only"}}}},
				porttest.OK(""),
			},
			nil,
		},
	} {
		c, _ := makeChannel(tc.steps...)
		err := c.Call(context.Background(), port.Call{Path: "x", Web: true}, nil)
		if !errors.Is(err, tc.want) {
			t.Fatal(err, tc.want)
		}
	}
}
func TestCanceledXSRFWaiterDoesNotCancelLeader(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		arrived := make(chan struct{})
		release := make(chan struct{})
		step := handshake("shared")
		step.Run = func(ctx context.Context, r port.Request) (port.Response, error) {
			close(arrived)
			<-release
			return step.Response, nil
		}
		c, tr := makeChannel(step, porttest.OK(""))
		leader := make(chan error, 1)
		go func() { leader <- c.Call(context.Background(), port.Call{Path: "x", Web: true}, nil) }()
		<-arrived
		ctx, cancel := context.WithCancel(context.Background())
		waiter := make(chan error, 1)
		go func() { waiter <- c.Call(ctx, port.Call{Path: "y", Web: true}, nil) }()
		synctest.Wait()
		cancel()
		if err := <-waiter; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		close(release)
		if err := <-leader; err != nil {
			t.Fatal(err)
		}
		if len(tr.Calls()) != 2 {
			t.Fatal(tr.Calls())
		}
	})
}
func TestHandshakeWaitersShareFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		arrived := make(chan struct{})
		release := make(chan struct{})
		c, _ := makeChannel(porttest.Step{Run: func(context.Context, port.Request) (port.Response, error) {
			close(arrived)
			<-release
			return port.Response{}, domain.ErrInvalidCredentials
		}})
		done := make(chan error, 2)
		go func() { done <- c.Call(context.Background(), port.Call{Path: "x", Web: true}, nil) }()
		<-arrived
		go func() { done <- c.Call(context.Background(), port.Call{Path: "y", Web: true}, nil) }()
		synctest.Wait()
		close(release)
		for range 2 {
			if err := <-done; !errors.Is(err, domain.ErrInvalidCredentials) {
				t.Fatal(err)
			}
		}
	})
}

func TestCanceledXSRFLeaderDoesNotFailWaiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		arrived := make(chan struct{})
		c, tr := makeChannel(porttest.Step{Run: func(ctx context.Context, _ port.Request) (port.Response, error) {
			close(arrived)
			<-ctx.Done()
			return port.Response{}, ctx.Err()
		}}, handshake("fresh"), porttest.OK(""))
		leaderCtx, cancel := context.WithCancel(t.Context())
		defer cancel()
		leader, waiter := make(chan error, 1), make(chan error, 1)
		go func() { leader <- c.Call(leaderCtx, port.Call{Web: true, Path: "x"}, nil) }()
		<-arrived
		go func() { waiter <- c.Call(t.Context(), port.Call{Web: true, Path: "x"}, nil) }()
		synctest.Wait()
		cancel()
		if err := <-leader; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if err := <-waiter; err != nil {
			t.Fatal(err)
		}
		if len(tr.Calls()) != 3 {
			t.Fatal(tr.Calls())
		}
	})
}
func TestEmptyXSRFCookieClearsPair(t *testing.T) {
	c, _ := makeChannel()
	c.xsrf = "old"
	c.cookies[domain.XSRFCookie] = "old"
	c.absorb(port.Response{Cookies: []port.Cookie{{Name: domain.XSRFCookie, Value: ""}}})
	if c.xsrf != "" || c.cookieHeader() != "" {
		t.Fatal(c.xsrf, c.cookieHeader())
	}
}
