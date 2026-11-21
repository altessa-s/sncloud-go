// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"crypto/md5" // #nosec G501 -- Supernote protocol requires MD5 for digest and file checksums.
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

const hashChunkBytes = 64 * 1024

// MD5Hex returns the lowercase hexadecimal MD5 digest of data.
func MD5Hex(data []byte) string {
	// #nosec G401 -- Supernote protocol requires MD5 for digest and file checksums.
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

// SHA256Hex returns the lowercase hexadecimal SHA-256 digest of data.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// MD5Reader hashes r in fixed-size chunks without buffering the entire stream
// in memory, returning the lowercase hexadecimal MD5 sum and byte count.
func MD5Reader(r io.Reader) (sum string, size int64, err error) {
	// #nosec G401 -- Supernote protocol requires MD5 for digest and file checksums.
	h := md5.New()
	buf := make([]byte, hashChunkBytes)
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			size += int64(n)
			_, _ = h.Write(buf[:n])
		}
		if readErr == io.EOF {
			return hex.EncodeToString(h.Sum(nil)), size, nil
		}
		if readErr != nil {
			return "", 0, readErr
		}
	}
}

// MD5Slice hashes up to length bytes from r starting at offset, stopping at EOF
// and returning the MD5 sum alongside the actual number of bytes hashed.
func MD5Slice(r io.ReaderAt, offset, length int64) (sum string, size int64, err error) {
	if offset < 0 || length < 0 {
		return "", 0, fmt.Errorf("invalid slice range offset=%d length=%d", offset, length)
	}
	return MD5Reader(io.NewSectionReader(r, offset, length))
}
