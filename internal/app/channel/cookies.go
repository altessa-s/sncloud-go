// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package channel

import (
	"sort"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func (c *Channel) cookieHeader() string {
	keys := make([]string, 0, len(c.cookies))
	for k := range c.cookies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+c.cookies[k])
	}
	return strings.Join(parts, "; ")
}
func (c *Channel) absorb(r port.Response) {
	for _, cookie := range r.Cookies {
		if cookie.Delete || cookie.Name == domain.XSRFCookie && cookie.Value == "" || !cookie.Expires.IsZero() && cookie.Expires.Before(c.clock.Now()) {
			delete(c.cookies, cookie.Name)
			if cookie.Name == domain.XSRFCookie {
				c.xsrf = ""
			}
			continue
		}
		c.cookies[cookie.Name] = cookie.Value
		if cookie.Name == domain.XSRFCookie && cookie.Value != "" {
			c.xsrf = cookie.Value
		}
	}
}
