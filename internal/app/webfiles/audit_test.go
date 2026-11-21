// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package webfiles

import (
	"errors"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func TestPagesCeiling(t *testing.T) {
	for _, complete := range []bool{false, true} {
		calls := 0
		got, err := pages(func(page int) ([]int, *domain.WireInt64, *domain.WireInt64, error) {
			calls++
			if complete && page == pageCeiling {
				return nil, nil, nil, nil
			}
			return make([]int, pageSize), nil, nil, nil
		})
		if calls != pageCeiling {
			t.Fatalf("calls=%d", calls)
		}
		if complete {
			if err != nil || len(got) != (pageCeiling-1)*pageSize {
				t.Fatalf("completed: len=%d, %v", len(got), err)
			}
		} else if !errors.Is(err, domain.ErrMalformedResponse) || got != nil {
			t.Fatalf("truncated listing: len=%d, %v", len(got), err)
		}
	}
}
