// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

func TestHashesAndSlices(t *testing.T) {
	const payload = "hello supernote"
	const wantMD5 = "5b5778d47e5d7cc586801850e65f0d0b"

	if got := MD5Hex([]byte(payload)); got != wantMD5 {
		t.Fatalf("MD5Hex = %q, want %q", got, wantMD5)
	}
	if got := SHA256Hex([]byte("abc")); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("SHA256Hex = %q", got)
	}

	sum, size, err := MD5Reader(strings.NewReader(payload))
	if err != nil || sum != wantMD5 || size != int64(len(payload)) {
		t.Fatalf("MD5Reader = (%q, %d, %v)", sum, size, err)
	}

	r := bytes.NewReader([]byte(payload))
	partSum, partSize, err := MD5Slice(r, 6, 9)
	if err != nil || partSum != MD5Hex([]byte("supernote")) || partSize != 9 {
		t.Fatalf("MD5Slice = (%q, %d, %v)", partSum, partSize, err)
	}

	pastSum, pastSize, err := MD5Slice(r, 6, 100)
	if err != nil || pastSum != partSum || pastSize != 9 {
		t.Fatalf("MD5Slice past EOF = (%q, %d, %v)", pastSum, pastSize, err)
	}

	if _, _, err := MD5Slice(r, -1, 4); err == nil {
		t.Fatal("expected error on negative offset")
	}
	if _, _, err := MD5Slice(r, 0, -1); err == nil {
		t.Fatal("expected error on negative length")
	}
	if _, _, err := MD5Reader(iotest.ErrReader(errors.New("boom"))); err == nil {
		t.Fatal("expected read error")
	}
}
