// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"testing"
	"time"
)

func TestConversionUsesProcessingBudget(t *testing.T) {
	tr := New(Config{})
	defer tr.CloseIdleConnections()
	if tr.metadataClient(false) != tr.client || tr.metadataClient(true) != tr.upload {
		t.Fatal("wrong metadata pool")
	}
	if tr.upBase.ResponseHeaderTimeout != 5*time.Minute || tr.memBase.ResponseHeaderTimeout != time.Minute {
		t.Fatal("wrong header budgets")
	}
}
