// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package devtools pins the development tools of sncloud-go in a module of
// their own, so their dependencies never reach the main go.mod. No code
// imports it.
//
// # Pinned tools
//
// tools.go (build tag tools) blank-imports the commands the Makefile runs:
// golangci-lint, gci, optgen, gosec, govulncheck,
// dupl, benchstat and the copyright checker.
// Their versions are the ones this module's go.mod requires.
//
// # Usage
//
// Make targets build or install each tool from this module into bin/ before
// running it, so a different version on PATH never takes part:
//
//	go -C devtools build -o "$PWD/bin/gci" github.com/daixiang0/gci
//
//	GOBIN="$PWD/bin" go -C devtools install github.com/golangci/golangci-lint/v2/cmd/golangci-lint
//	"$PWD/bin/golangci-lint" run ./...
//
// Upgrade a tool with go get in this directory, then go mod tidy.
package devtools
