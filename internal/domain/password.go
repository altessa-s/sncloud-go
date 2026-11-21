// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"crypto/md5" // #nosec G501 -- Required by the server's SHA256(MD5-hex + salt) login protocol.
	"crypto/sha256"
	"encoding/hex"
)

// PasswordDigest is the service's single-use SHA256(MD5-hex + salt).
func PasswordDigest(password, salt string) string {
	m := md5.Sum([]byte(password)) // #nosec G401 -- Protocol compatibility; the salted SHA256 result is transmitted, never this digest.
	s := sha256.Sum256([]byte(hex.EncodeToString(m[:]) + salt))
	return hex.EncodeToString(s[:])
}
