// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package feed

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

const (
	socketIOPath    = "/socket.io/"
	engineIOVersion = "3"
	transportWS     = "websocket"
)

// handshakeURL builds the signed Engine.IO 3 WebSocket URL on socketHost. Any
// literal '+' in encoded query parameters is replaced with %2B so servers that
// treat '+' as a space do not corrupt handshake inputs.
func handshakeURL(socketHost, token, kind, random string) (string, error) {
	u, err := url.Parse(socketHost)
	if err != nil || u.Host == "" {
		return "", &domain.MalformedResponseError{What: fmt.Sprintf("not a socket host: %s", socketHost)}
	}
	if strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "ws") {
		u.Scheme = "ws"
	} else {
		u.Scheme = "wss"
	}
	u.Path = socketIOPath
	sign := domain.FeedSignature(token + "_" + kind + "_" + random)

	pairs := [][2]string{
		{"token", token},
		{"type", kind},
		{"random", random},
		{"sign", sign},
		{"EIO", engineIOVersion},
		{"transport", transportWS},
	}
	var q strings.Builder
	for i, kv := range pairs {
		if i > 0 {
			q.WriteByte('&')
		}
		q.WriteString(kv[0])
		q.WriteByte('=')
		escaped := url.QueryEscape(kv[1])
		q.WriteString(strings.ReplaceAll(escaped, "+", "%2B"))
	}
	u.RawQuery = q.String()
	return u.String(), nil
}
