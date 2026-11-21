#!/usr/bin/env bash
# Copyright 2026 ALTESSA SOLUTIONS INC.
# Use of this source code is governed by the MIT license that can be found in
# the LICENSE file.
# Validate only; the Make target creates the tag after this succeeds.
set -euo pipefail
refuse() { echo "Release refused: $*" >&2; exit 1; }
version=${VERSION:-}
[[ $version =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || refuse 'VERSION must be vMAJOR.MINOR.PATCH (no leading zeroes)'
[[ -z $(git status --porcelain) ]] || refuse 'checkout must be clean, including untracked files'
[[ $(git symbolic-ref --quiet --short HEAD) == main ]] || refuse 'branch must be main'
! git show-ref --verify --quiet "refs/tags/$version" || refuse "tag $version already exists"
head=$(git rev-parse HEAD)
tracking=$(git rev-parse --verify refs/remotes/origin/main) || refuse 'origin/main is missing; fetch origin'
[[ $head == "$tracking" ]] || refuse 'HEAD differs from origin/main'
remote=$(git ls-remote --exit-code origin refs/heads/main) || refuse 'cannot resolve main on origin'
[[ ${remote%%$'\t'*} == "$head" ]] || refuse 'HEAD differs from main on origin; fetch origin'
tags=$(git ls-remote origin "refs/tags/$version") || refuse 'cannot check tags on origin'
[[ -z $tags ]] || refuse "tag $version already exists on origin"
echo "Release prerequisites passed for $version"
