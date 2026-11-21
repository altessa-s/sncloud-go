#!/usr/bin/env bash
# Copyright 2026 ALTESSA SOLUTIONS INC.
# Use of this source code is governed by the MIT license that can be found in
# the LICENSE file.
# Exercise refusals with a read-only Git double; never create refs or repositories.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
scratch=$(mktemp -d "$root/.release-check.XXXXXX")
trap 'rm -rf "$scratch"' EXIT
cat > "$scratch/git" <<'GIT'
#!/usr/bin/env bash
set -eu
case "$*" in
 'status --porcelain') [[ $CASE != dirty ]] || echo ' M README.md';;
 'symbolic-ref --quiet --short HEAD') if [[ $CASE == branch ]]; then echo develop; else echo main; fi;;
 'show-ref --verify --quiet refs/tags/v0.1.0') [[ $CASE == tag ]]; exit $?;;
 'rev-parse HEAD') echo aaaa;;
 'rev-parse --verify refs/remotes/origin/main')
   if [[ $CASE == missing ]]; then exit 1; elif [[ $CASE == behind ]]; then echo bbbb; else echo aaaa; fi;;
 'ls-remote --exit-code origin refs/heads/main')
   if [[ $CASE == network ]]; then exit 1; elif [[ $CASE == stale ]]; then printf 'bbbb\trefs/heads/main\n'; else printf 'aaaa\trefs/heads/main\n'; fi;;
 'ls-remote origin refs/tags/v0.1.0') [[ $CASE != remote-tag ]] || printf 'cccc\trefs/tags/v0.1.0\n';;
 *) echo "Unexpected Git command: $*" >&2; exit 99;;
esac
exit 0
GIT
chmod +x "$scratch/git"
export PATH="$scratch:$PATH"
for CASE in dirty branch tag missing behind network stale remote-tag; do
 export CASE
 if VERSION=v0.1.0 bash "$root/.github/scripts/release-check.sh" > "$scratch/output" 2>&1; then
   echo "Expected refusal: $CASE" >&2; exit 1
 fi
 grep -q 'Release refused:' "$scratch/output"
done
export CASE=clean
for VERSION in '' 0.1.0 v01.2.3 v1.2 v1.2.3-rc1 'v1.2.3;false'; do
 export VERSION
 if bash "$root/.github/scripts/release-check.sh" > "$scratch/output" 2>&1; then
  echo "Accepted invalid version: $VERSION" >&2; exit 1
 fi
done
VERSION=v0.1.0 bash "$root/.github/scripts/release-check.sh"
echo 'Release guard cases passed'
