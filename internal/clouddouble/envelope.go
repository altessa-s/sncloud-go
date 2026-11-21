// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"encoding/json"
	"maps"
	"net/http"
)

// Reply writes a flat envelope with additional payload fields.
func Reply(w http.ResponseWriter, fields map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	out := map[string]any{"success": true}
	maps.Copy(out, fields)
	// The client may have canceled; a failed response write needs no retry.
	_ = json.NewEncoder(w).Encode(out)
}

// Refuse writes a service rejection, normally at HTTP 200.
func Refuse(w http.ResponseWriter, code string) {
	Reply(w, map[string]any{"success": false, "errorCode": code, "errorMsg": "synthetic refusal"})
}
