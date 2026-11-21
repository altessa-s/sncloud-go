// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestInvalidRequestsAndMissingFiles(t *testing.T) {
	tr := newTestTransport(t, Config{})
	if _, err := tr.RoundTrip(context.Background(), port.Request{URL: "%"}); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatal(err)
	}
	if _, err := tr.Download(context.Background(), port.Request{URL: "%"}); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatal(err)
	}
	missing := port.Request{URL: "https://example.invalid", File: filepath.Join(t.TempDir(), "absent")}
	if _, err := tr.Upload(context.Background(), missing); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(f, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Upload(context.Background(), port.Request{URL: "%", File: f}); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatal(err)
	}
}
