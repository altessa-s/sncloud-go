// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"net/http"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

func TestNewValidation(t *testing.T) {
	if _, err := sncloud.New(nil); err == nil {
		t.Fatal("nil store")
	}
	var typed *porttest.Secrets
	if _, err := sncloud.New(typed); err == nil {
		t.Fatal("typed nil store")
	}
	if _, err := sncloud.New(&porttest.Secrets{}, sncloud.WithEndpoints(sncloud.PrivateCloud("bad", false))); err == nil {
		t.Fatal("bad endpoint")
	}
	var rt http.RoundTripper
	if c, err := sncloud.New(
		&porttest.Secrets{},
		sncloud.WithRoundTripper(rt),
		sncloud.WithLogger(nil),
		sncloud.WithRetryPolicy(sncloud.DefaultRetryPolicy()),
	); err != nil {
		t.Fatal(err)
	} else {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
