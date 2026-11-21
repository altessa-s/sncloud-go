#!/bin/bash
# Copyright 2026 ALTESSA SOLUTIONS INC.
# Use of this source code is governed by the MIT license that can be found in
# the LICENSE file.
# Mirror the workflow's total floor locally without maintaining a second value.
set -e -o pipefail

PROFILE="${1:-coverage.out}"
WORKFLOW=".github/workflows/ci.yml"

# Accept exactly one literal numeric COVERAGE_MIN. Fail closed if the workflow
# changes shape, rather than silently using a stale default or skipping the gate.
minimum=$(awk '/^[[:space:]]*COVERAGE_MIN:/ { sub(/^[[:space:]]*COVERAGE_MIN:[[:space:]]*/, ""); gsub(/"/, ""); print }' "$WORKFLOW")
if [[ ! "$minimum" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    echo "Cannot read a unique numeric COVERAGE_MIN from $WORKFLOW" >&2
    exit 1
fi

# CI compares the one-decimal total emitted by make coverage-total and skips
# profiles with no executable statements. Keep those exact semantics here.
awk -v minimum="$minimum" '
    NR > 1 { statements += $2; if ($3 > 0) covered += $2 }
    END {
        if (statements == 0) {
            print "No executable statements in coverage.out yet; skipping floor check"
            exit 0
        }
        actual = sprintf("%.1f", covered * 100 / statements)
        printf "Total coverage: %s%% (minimum %s%%)\n", actual, minimum
        exit !(actual + 0 >= minimum)
    }
' "$PROFILE"
