# GitHub Actions scripts

## Commit messages

`validate-commit-message.sh` reads a complete message from stdin. The local `commit-msg` hook and CI use this same validator. It checks
Conventional Commits (`type[(scope)][!]: description`), types, lowercase descriptions and scopes from [`commit_scopes.txt`](../../commit_scopes.txt).
Only the scope lines of that file count; comment and blank lines name no scope. The `root` scope is forbidden; omit the scope for repository-wide
changes. Breaking changes may use `!` before the colon.

Messages must describe the concrete change without filler, test results, coverage figures or validation summaries. Omit generation
signatures and tool attribution. Use co-author trailers for people. The validator matches tool names as whole words.
`BREAKING CHANGE:` explanations are allowed.

`validate-commits.sh` validates the complete message of every commit in a range. All failures fail CI. The only exemption is the subject of a
merge git generates itself (`Merge branch|tag|remote-tracking branch '…'`, `Merge pull request #N from …`), as git flow merges releases and
hotfixes; its body is checked like any other. Automated and revert commits use a conventional subject: Dependabot writes `build(deps): bump …`
(see [`dependabot.yml`](../dependabot.yml)), which the `deps` scope covers, so it needs no exemption. PR titles use the same validator.

## Usage

```sh
# Enable the local hook.
git config core.hooksPath .githooks

# Exercise validation rules and the local hook without creating commits.
bash .github/scripts/test-commit-validation.sh

# Validate one message.
bash .github/scripts/validate-commit-message.sh < /tmp/commit-message

# Validate a specific range.
bash .github/scripts/validate-commits.sh origin/develop..HEAD
```

Without a range, local runs compare against the merge base with `origin/develop`. CI supplies `BASE_SHA` and `HEAD_SHA` for the event. A new branch
push without a base checks all commits reachable from the head. Missing references and invalid messages fail the check; history is never fetched
implicitly or replaced with a shorter range.

## Examples

```text
fix(session): drop the access token on E0712
feat(webfiles): list recycle bin entries with paging
build(deps): bump golang.org/x/sys from 0.48.0 to 0.49.0
build: pin the options generator
```

Exit status is zero only when every message is valid (or the selected range is empty).

## Benchmark regressions

`bench-regression.sh BASE_REF` checks the base out into a temporary worktree, runs every benchmark of both trees `BENCH_COUNT` times with
`-benchtime` `BENCH_TIME` and compares them with the `benchstat` pinned in [`devtools`](../../devtools). It fails when a benchmark shows a
statistically significant increase in `sec/op` or `allocs/op` above `BENCH_THRESHOLD` percent; changes benchstat marks as insignificant (`~`),
benchmarks present on one side only, and trees without benchmarks never fail. `BENCH_REPORT` names a file that also receives the report. The defaults
of these variables are set and documented in the script's header. The [`bench.yml`](../workflows/bench.yml) workflow runs it on pull requests against
the base branch; locally use `make bench-compare BASE=<ref>`.

## Coverage gate

`coverage-gate.sh [PROFILE]` reads the threshold and the package list from [`.coverage.yml`](../../.coverage.yml) and fails when any listed
package's statement-weighted coverage in `PROFILE` (default `coverage.out`) is below it. Subpackages are gated on their own entries; a package
with no statements yet is reported as skipped. CI runs it on the profile `make coverage` writes; locally use `make coverage-gate`.

## Release and generated files

`make changelog-check` tests Go's regular expressions against scoped and breaking subjects, then reads `git log --format=%s` and reports each subject's release-note group. The Go program in `changelog/` reads expressions directly from `.goreleaser.yml`; it fails if anything except docs, ci, chore or test subjects is excluded or falls into Other.

`release-check.sh` is the read-only prerequisite check used by `make release`. `make release-check-test` exercises it with a Git double, including dirty trees, wrong branches, missing/stale remote refs, duplicate tags and invalid versions. No test creates a Git repository or tag.

`make copyright-check` includes generated files. `make generate` restores the header after optgen runs, since the pinned generator has no configurable file header. `copyright.sh --generated` is the post-generation step, not a standalone repeated header fixer.
