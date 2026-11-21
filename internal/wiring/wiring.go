// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package wiring assembles the use cases and adapters for the facade.
package wiring

import (
	"net/http"
	"os"

	"github.com/altessa-s/sncloud-go/internal/adapter/httpapi"
	"github.com/altessa-s/sncloud-go/internal/adapter/ws"
	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/app/devicefiles"
	"github.com/altessa-s/sncloud-go/internal/app/digests"
	"github.com/altessa-s/sncloud-go/internal/app/feed"
	"github.com/altessa-s/sncloud-go/internal/app/session"
	"github.com/altessa-s/sncloud-go/internal/app/tasks"
	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/app/webfiles"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Config is the composition root's input.
type Config struct {
	Secrets      port.SecretStore
	Endpoints    domain.Endpoints
	RoundTripper http.RoundTripper
	Logger       port.Logger
	Policy       domain.RetryPolicy
}

// Parts exposes assembled use cases through ports.
type Parts struct {
	Files     port.WebFiles
	Device    port.DeviceFiles
	Session   port.AccountSession
	Tasks     port.TaskService
	Digests   port.DigestService
	Feed      port.ChangeFeed
	Transport port.IdleCloser
}

// Build creates the graph without network I/O.
func Build(c Config) Parts {
	t := httpapi.New(httpapi.Config{
		PrivateCloud: c.Endpoints.PrivateCloud,
		RoundTripper: c.RoundTripper,
		Logger:       c.Logger,
		WireLevel:    httpapi.ParseWireLevel(os.Getenv("SNCLOUD_WIRE_LOG")),
	})
	clock := port.SystemClock{}
	w := ws.New(ws.Config{
		Clock:        clock,
		PrivateCloud: c.Endpoints.PrivateCloud,
		RoundTripper: c.RoundTripper,
	})
	ch := channel.New(c.Endpoints, t, clock, c.Policy)
	s := session.New(ch, c.Secrets, clock)
	b := transfers.New(t, clock, c.Policy)
	return Parts{
		Files:     webfiles.New(ch, s, b, c.Endpoints.WebHost, clock),
		Device:    devicefiles.New(ch, s, b, c.Endpoints.AppHost),
		Session:   s,
		Tasks:     tasks.New(ch, s, clock),
		Digests:   digests.New(c.Endpoints, ch, s, b, clock),
		Feed:      feed.New(c.Endpoints, s, w, clock, c.Policy),
		Transport: idleCloser{t, w},
	}
}

type idleCloser [2]port.IdleCloser

func (c idleCloser) CloseIdleConnections() {
	for _, closer := range c {
		closer.CloseIdleConnections()
	}
}
