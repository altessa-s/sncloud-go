// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package channel

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestIsolatesTheCookiesOfAnOvertakenLogin(t *testing.T) {
	step := porttest.OK("")
	step.Response.Cookies = []port.Cookie{{Name: "SESSION", Value: "from-app"}}
	c, tr := makeChannel(step, handshake("web"), porttest.OK(""), porttest.OK(""))
	for _, web := range []bool{false, true, false} {
		if err := c.Call(context.Background(), port.Call{Path: "x", Web: web}, nil); err != nil {
			t.Fatal(err)
		}
	}
	calls := tr.Calls()
	if calls[1].Header.Get("Cookie") != "" || calls[3].Header.Get("Cookie") != "" {
		t.Fatal("app cookies crossed hosts")
	}
}
func TestCookieExpiryAndDeletion(t *testing.T) {
	c, _ := makeChannel()
	c.mu.Lock()
	c.absorb(port.Response{
		Cookies: []port.Cookie{{Name: domain.XSRFCookie, Value: "old"}, {Name: "SESSION", Value: "old"}},
	})
	c.absorb(port.Response{
		Cookies: []port.Cookie{
			{Name: domain.XSRFCookie, Value: "old", Delete: true},
			{Name: "SESSION", Value: "old", Expires: time.Unix(1, 0)},
		},
	})
	if c.xsrf != "" || len(c.cookies) != 0 {
		t.Fatal(c.xsrf, c.cookies)
	}
	c.mu.Unlock()
}
