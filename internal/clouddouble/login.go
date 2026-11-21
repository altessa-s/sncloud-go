// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"encoding/base64"
	"fmt"
	"net/http"
)

func (c *Cloud) issue(w http.ResponseWriter) {
	c.serial++
	token := fmt.Sprintf("synthetic-private-%d", c.serial)
	if c.flavor == Official {
		payload := fmt.Sprintf(`{"exp":4102444800,"serial":%d}`, c.serial)
		token = "synthetic." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".unsigned"
	}
	c.tokens[token] = true
	Reply(w, map[string]any{"token": token})
}
