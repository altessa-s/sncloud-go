#!/usr/bin/env bash
# Copyright 2026 ALTESSA SOLUTIONS INC.
# Use of this source code is governed by the MIT license that can be found in
# the LICENSE file.
# Include generated files, which the pinned checker otherwise skips.
set -euo pipefail
header=$(head -n 3 options.go)
if [[ ${1:-} == --generated ]]; then
  { printf '%s\n\n' "$header"; cat options_gen.go; } > options_gen.go.tmp
  mv options_gen.go.tmp options_gen.go
fi
status=0
while IFS= read -r -d '' file; do
  if [[ $(head -n 10 "$file") != *"$header"* ]]; then
    echo "Missing copyright header: $file" >&2
    status=1
  fi
done < <(find . -type d \( -name .git -o -name bin -o -name .scratch \) -prune -o -name '*.go' -print0)
exit "$status"
