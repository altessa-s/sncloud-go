// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"math"
	"strconv"
	"strings"
	"time"
)

// JWTs consist of header, payload and signature.
const jwtParts = 3

// Larger expiry values use milliseconds rather than seconds.
const millisecondExpiryThreshold = 1e11

// Leave five minutes for an authenticated request to finish.
const tokenExpiryMargin = 5 * time.Minute

// TokenExpiry reads exp without authenticating it; the server owns validation.
// Opaque private-cloud tokens have no local expiry.
func TokenExpiry(token string) *time.Time {
	parts := strings.Split(token, ".")
	if len(parts) != jwtParts {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil
	}
	var obj map[string]jsontext.Value
	if json.Unmarshal(b, &obj) != nil {
		return nil
	}
	raw := obj["exp"]
	var s string
	if json.Unmarshal(raw, &s) != nil {
		s = string(raw)
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	if n > millisecondExpiryThreshold {
		n /= 1000
	}
	if n >= float64(math.MaxInt64) {
		return nil
	}
	sec, frac := math.Modf(n)
	t := time.Unix(int64(sec), int64(frac*float64(time.Second)))
	return &t
}

// TokenUsable leaves five minutes for a request to finish.
func TokenUsable(token string, now time.Time) bool {
	e := TokenExpiry(token)
	return e == nil || e.Sub(now) > tokenExpiryMargin
}
