// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package porttest

import (
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestUnexpectedCallsRedactURL(t *testing.T) {
	const raw = "wss://user:password@feed.invalid/socket.io/?token=secret&sign=signed#fragment"
	_, dialErr := (&SocketDialer{}).Dial(t.Context(), raw, nil)
	_, requestErr := (&Transport{}).RoundTrip(t.Context(), port.Request{URL: raw})
	for _, err := range []error{dialErr, requestErr} {
		if err == nil || !strings.HasSuffix(err.Error(), "wss://feed.invalid/socket.io/") {
			t.Fatalf("diagnostic = %v", err)
		}
	}
	if got := safeURL("%zz"); got != "<invalid URL>" {
		t.Fatal(got)
	}
}
