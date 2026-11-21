// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

// ByteRange is inclusive at both ends.
type ByteRange struct{ First, Last int64 }

// DownloadLimit bounds transfers whose size the service did not declare.
const DownloadLimit int64 = 2 * 1024 * 1024 * 1024
