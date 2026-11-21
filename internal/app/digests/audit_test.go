// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package digests

import (
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestLaterDigestDeletionWins(t *testing.T) {
	for _, groups := range []bool{false, true} {
		tr := &porttest.Transport{Steps: []porttest.Step{
			porttest.OK(`"totalPages":2,"currentPage":1,"summaryDOList":[{"id":1,"isDeleted":"N"},{"id":2,"isDeleted":"Y"}]`),
			porttest.OK(`"totalPages":2,"currentPage":2,"summaryDOList":[{"id":1,"isDeleted":"Y"},{"id":2,"isDeleted":"N"}]`),
		}}
		c, _, _ := newTestDigestClient(domain.Official(), tr)
		if groups {
			got, err := c.Groups(t.Context())
			if err != nil || len(got) != 1 || got[0].ID != 2 {
				t.Fatalf("groups=%+v, %v", got, err)
			}
		} else {
			got, err := c.List(t.Context(), "")
			if err != nil || len(got) != 1 || got[0].ID != 2 {
				t.Fatalf("digests=%+v, %v", got, err)
			}
		}
	}
}
