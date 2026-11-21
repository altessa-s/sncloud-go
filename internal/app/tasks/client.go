// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package tasks implements the to-do list and task protocol use cases.
//
// Three server behaviors shape the client:
//   - Task updates overwrite title, detail, status, importance, due and
//     completion timestamps, links and list whether omitted or present, so
//     Update sends the complete Task read from All.
//   - Listings are always read whole from the first page. The server also
//     offers a delta since a nextSyncToken, and this client does not ask for
//     it, for two reasons. A task's or a list's delete removes the row, so a
//     delta never shows a deletion and a full listing is needed anyway. And
//     in service images before 26.10.09 the delta's filter was an
//     unparenthesized OR that matched other accounts' rows, which carry no
//     userId to tell them apart by; the protocol names no server version, so
//     a client cannot know which filter it is talking to.
//   - Single-list GET/DELETE and task DELETE interpolate caller identifiers
//     directly into the URL path, so identifiers are restricted to single-segment
//     ASCII tokens before building the path.
package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

const (
	// pageSize requests 500 rows per page rather than the server's default 20 so
	// most accounts fit on one page, minimizing offset drift when a concurrent
	// edit moves an entry to the end of the modification-time order.
	pageSize       = "500"
	defaultPageMax = 10000
	methodPost     = "POST"
	methodPut      = "PUT"
	methodGet      = "GET"
	methodDelete   = "DELETE"
)

// Session supplies credentials and app headers without coupling to the session implementation.
type Session interface {
	Pin() *domain.CredentialPin
	AccessToken(expected *domain.CredentialPin) (string, error)
	AppHeaders(token string, withChannel bool) (port.Header, error)
	Invalidate(refused string)
}

// Client operates on to-do lists and tasks over the app API host.
type Client struct {
	channel port.Channel
	session Session
	clock   port.Clock
	pageMax int
}

// New constructs a to-do client sharing the session's protocol channel and clock.
func New(ch port.Channel, s Session, clock port.Clock) *Client {
	return &Client{
		channel: ch,
		session: s,
		clock:   clock,
		pageMax: defaultPageMax,
	}
}

func (c *Client) call(ctx context.Context, path, method string, body any, changes bool, out any) error {
	return c.callPinned(ctx, path, method, body, changes, nil, out)
}

func (c *Client) callPinned(ctx context.Context, path, method string, body any, changes bool, pin *domain.CredentialPin, out any) error {
	token, headers, err := c.credentials(pin)
	if err != nil {
		return err
	}
	req := port.Call{Path: path, Method: method, Token: token, Changes: changes, Header: headers, Body: body}
	if err := c.channel.Call(ctx, req, out); err != nil {
		if errors.Is(err, domain.ErrSessionExpired) {
			c.session.Invalidate(token)
		}
		return err
	}
	return nil
}

func (c *Client) credentials(pin *domain.CredentialPin) (string, port.Header, error) {
	token, err := c.session.AccessToken(pin)
	if err != nil {
		return "", nil, err
	}
	// Including channel tags mutations with this installation's identity so the
	// Socket.IO change feed suppresses self-originated echoes.
	headers, err := c.session.AppHeaders(token, true)
	if err != nil {
		return "", nil, err
	}
	return token, headers, nil
}

type pagedRows[T any] interface {
	rows() ([]domain.Marked[T], *string)
	pageRequest(token string) any
	itemID(T) string
}

func fetchPaged[T any, Ans pagedRows[T]](c *Client, ctx context.Context, path string) ([]T, error) {
	pin := c.session.Pin()
	var all []domain.Marked[T]
	var token string
	var zero Ans
	seen := make(map[string]struct{})
	for range c.pageMax {
		var ans Ans
		if err := c.callPinned(ctx, path, methodPost, zero.pageRequest(token), false, pin, &ans); err != nil {
			return nil, err
		}
		rows, next := ans.rows()
		all = append(all, rows...)
		if next == nil || *next == "" {
			// Concurrent edits move a modified item to a later page; keeping the
			// last occurrence retains its freshest state.
			all = domain.UniqueLast(all, func(r domain.Marked[T]) string { return zero.itemID(r.Value) })
			out := make([]T, 0, len(all))
			for _, r := range all {
				if !r.Deleted {
					out = append(out, r.Value)
				}
			}
			return out, nil
		}
		if _, dup := seen[*next]; dup {
			return nil, &domain.MalformedResponseError{What: fmt.Sprintf("the to-do listing named page %s twice", *next)}
		}
		seen[*next] = struct{}{}
		token = *next
	}
	return nil, &domain.MalformedResponseError{What: fmt.Sprintf("the to-do listing ran past %d pages", c.pageMax)}
}

var _ port.TaskService = (*Client)(nil)
