// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package transfers

import (
	"crypto/md5" // #nosec G501 -- The service mandates MD5 for content integrity.
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

// Original records the source and checksum before upload/apply.
type Original struct {
	File, Name, MD5 string
	Size            int64
}

// Measure hashes a file with bounded memory. An empty name uses its base name.
func Measure(file, name string) (Original, error) {
	if name == "" {
		name = filepath.Base(file)
	}
	// #nosec G304 -- The caller explicitly selects the local upload source.
	f, err := os.Open(file)
	if err != nil {
		return Original{}, err
	}
	defer func() { _ = f.Close() }() // Read-only handle; closing cannot lose buffered output.
	h := md5.New()                   // #nosec G401 -- Required by the storage protocol, not used for authentication.
	n, err := io.Copy(h, f)
	if err != nil {
		return Original{}, err
	}
	return Original{File: file, Name: name, MD5: hex.EncodeToString(h.Sum(nil)), Size: n}, nil
}

// Scratch writes bytes to a private temporary file owned by the caller.
func Scratch(data []byte) (string, error) {
	return scratch(func(f *os.File) error { _, err := f.Write(data); return err })
}

// scratch transfers ownership only after both writing and closing succeed.
func scratch(write func(*os.File) error) (string, error) {
	f, err := os.CreateTemp("", "sncloud-transfer-*")
	if err != nil {
		return "", err
	}
	err = write(f)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(f.Name()) // Best-effort cleanup; preserve the transfer or integrity result.
		return "", err
	}
	return f.Name(), nil
}

// copySlice bounds the source read without retaining a whole part in memory.
func copySlice(dst io.Writer, source string, offset, length int64) error {
	if offset < 0 || length < 0 {
		return malformed("invalid source slice")
	}
	// #nosec G304 -- The source is the caller-selected upload file, not a server path.
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // Read-only handle; closing cannot lose buffered output.
	_, err = io.CopyN(dst, io.NewSectionReader(f, offset, length), length)
	return err
}
