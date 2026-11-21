// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package channel

import (
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestEnvelopeLeadingWhitespace(t *testing.T) {
	if _, err := decodeEnvelope([]byte(" \r\n\t{}")); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{" \t", "\u2003{}"} {
		if _, err := decodeEnvelope([]byte(data)); err == nil {
			t.Fatalf("invalid JSON accepted: %q", data)
		}
	}
}
func BenchmarkEnvelopeLargeNonObject(b *testing.B) {
	data := []byte("<html>" + strings.Repeat("x", 1<<20))
	b.ReportAllocs()
	for b.Loop() {
		_, _ = decodeEnvelope(data)
	}
}

func TestRetryAfterDateUsesClockAndCap(t *testing.T) {
	for _, seconds := range []int{12, 120} {
		clk := &porttest.Clock{Time: time.Date(2026, time.November, 18, 10, 0, 0, 0, time.UTC)}
		date := clk.Now().Add(time.Duration(seconds) * time.Second).Format("Mon, 02 Jan 2006 15:04:05 GMT")
		tr := &porttest.Transport{Steps: []porttest.Step{{Response: port.Response{Status: 429, Header: port.Header{"Retry-After": {date}}}}, porttest.OK("")}}
		c := New(domain.Official(), tr, clk, domain.DefaultRetryPolicy())
		if err := c.Call(t.Context(), port.Call{Path: "synthetic"}, nil); err != nil {
			t.Fatal(err)
		}
		if got := clk.Delays(); len(got) != 1 || got[0] != time.Duration(min(seconds, 20))*time.Second {
			t.Fatalf("delays=%v", got)
		}
	}
}
