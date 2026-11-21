// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import "net/http"

// Note rendering and upload processing share a bounded five-minute header
// budget. Reuse that pool rather than changing a live transport's timers.
func (t *Transport) metadataClient(longRunning bool) *http.Client {
	if longRunning {
		return t.upload
	}
	return t.client
}
