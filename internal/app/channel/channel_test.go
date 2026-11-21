// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package channel

import (
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func makeChannel(steps ...porttest.Step) (*Channel, *porttest.Transport) {
	tr := &porttest.Transport{Steps: steps}
	return New(domain.Official(), tr, &porttest.Clock{Time: time.Unix(100, 0)}, domain.DefaultRetryPolicy()), tr
}
func handshake(token string) porttest.Step {
	return porttest.Step{
		Response: port.Response{
			Status:  200,
			Header:  port.Header{domain.XSRFHeader: {"header-" + token}},
			Cookies: []port.Cookie{{Name: domain.XSRFCookie, Value: token}, {Name: "SESSION", Value: "synthetic"}},
		},
	}
}
