// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package channel

import (
	"context"
	"errors"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestEndsACallFromAnEndedConversation(t *testing.T) {
	c, tr := makeChannel()
	old := c.Epoch()
	c.Forget()
	if err := c.Call(context.Background(), port.Call{Path: "x", Epoch: &old}, nil); !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatal(err)
	}
	if len(tr.Calls()) != 0 {
		t.Fatal("stale call sent")
	}
}
func TestRetriesReadButNotChange(t *testing.T) {
	for _, change := range []bool{false, true} {
		c, tr := makeChannel(porttest.Step{Response: port.Response{Status: 503}}, porttest.OK(""))
		err := c.Call(context.Background(), port.Call{Path: "x", Changes: change}, nil)
		if change {
			if !errors.Is(err, domain.ErrHTTP) || len(tr.Calls()) != 1 {
				t.Fatal(err)
			}
		} else if err != nil || len(tr.Calls()) != 2 {
			t.Fatal(err)
		}
	}
}
func TestNoBodyAndMethods(t *testing.T) {
	c, tr := makeChannel(porttest.OK(""))
	if err := c.Call(context.Background(), port.Call{Path: "x", Method: "DELETE"}, nil); err != nil {
		t.Fatal(err)
	}
	r := tr.Calls()[0]
	if r.Method != "DELETE" || r.Body != nil {
		t.Fatal(r)
	}
}
