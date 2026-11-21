// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"errors"
	"go/build"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const module = "github.com/altessa-s/sncloud-go/internal/"

// Frameworks the inner layers must not reach: they are the adapters' business.
var adapterFrameworks = []string{
	"github.com/altessa-s/go-atlas-net",
	"github.com/altessa-s/go-tlsutils",
	"github.com/coder/websocket",
}

// useCase is the rule every package under internal/app keeps: domain, ports,
// and explicitly listed lower-waist app packages, go-atlas-core, and no HTTP
// or WebSocket stack. Scratch files in transfers use os directly; there is no
// port.Files in this module.
func useCase(allow ...string) rule {
	return rule{
		allow:  append([]string{"domain", "port"}, allow...),
		forbid: append([]string{"net/http"}, adapterFrameworks...),
	}
}

type rule struct {
	allow  []string
	forbid []string
}

// layers is the dependency rule this module keeps. Each package may import
// the standard library and modules, minus what forbid names (a path or a
// module prefix), and the internal packages allow names. Tests are not
// checked: a test may reach for any double.
var layers = map[string]rule{
	// The protocol's pure types: no I/O, no logging, no network.
	"domain": {
		forbid: append([]string{"os", "syscall", "log/slog", "net", "net/http", "io/fs"}, adapterFrameworks...),
	},
	// What the use cases ask of the outside: contracts only.
	"port": {
		allow:  []string{"domain"},
		forbid: append([]string{"os", "syscall", "net", "net/http"}, adapterFrameworks...),
	},
	// The use cases and protocol waists. They reach the network only through
	// ports; the composition root hands them every adapter.
	"app/retry": useCase(),
	// Session takes port.Channel; channel never reaches session state.
	"app/channel": useCase("app/retry"),
	"app/session": useCase(),
	// No use case imports the session: each asks for it through an interface
	// (its own or a port), so none can reach the login around that contract.
	"app/transfers":   useCase("app/retry", "app/channel"),
	"app/webfiles":    useCase("app/transfers"),
	"app/devicefiles": useCase("app/transfers"),
	"app/tasks":       useCase(),
	"app/digests":     useCase("app/transfers"),
	"app/feed":        useCase(),
	// The adapters implement ports and know nothing of the use cases.
	"adapter/httpapi": {allow: []string{"domain", "port", "adapter/neterr"}},
	"adapter/ws":      {allow: []string{"domain", "port", "adapter/neterr"}},
	"adapter/neterr":  {allow: []string{"domain", "port"}},
	// The composition root: the one package that knows every adapter and
	// every use case.
	"wiring": {allow: []string{
		"domain", "port",
		"app/retry", "app/channel", "app/session", "app/transfers",
		"app/webfiles", "app/devicefiles", "app/tasks", "app/digests", "app/feed",
		"adapter/httpapi", "adapter/ws", "adapter/neterr",
	}},
	// Test doubles and the emulated Supernote cloud over httptest.
	"testutil":    {},
	"porttest":    {allow: []string{"domain", "port"}},
	"clouddouble": {allow: []string{"domain", "port"}},
}

func forbidden(forbid []string, imp string) bool {
	// A module path forbids its packages too; a standard-library path is
	// only itself, so net does not forbid net/url.
	return slices.ContainsFunc(forbid, func(f string) bool {
		return imp == f || (strings.Contains(f, ".") && strings.HasPrefix(imp, f+"/"))
	})
}

func TestLayers(t *testing.T) {
	for name, rule := range layers {
		dir := filepath.Join("internal", filepath.FromSlash(name))
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			continue
		}
		pkg, err := build.ImportDir(dir, 0)
		if err != nil {
			if _, ok := errors.AsType[*build.NoGoError](err); ok {
				continue
			}
			t.Fatalf("%s: %v", name, err)
		}
		for _, imp := range pkg.Imports {
			if internal, ok := strings.CutPrefix(imp, module); ok {
				if !slices.Contains(rule.allow, internal) {
					t.Errorf("%s imports internal/%s", name, internal)
				}
				continue
			}
			if forbidden(rule.forbid, imp) {
				t.Errorf("%s imports %s", name, imp)
			}
		}
	}
}

// TestFacadeSeesOnlyTheRoot keeps the public package off the adapters and
// the use cases: it reads options, calls wiring, and holds ports.
func TestFacadeSeesOnlyTheRoot(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	allow := []string{"domain", "port", "wiring"}
	for _, imp := range pkg.Imports {
		if internal, ok := strings.CutPrefix(imp, module); ok && !slices.Contains(allow, internal) {
			t.Errorf("the facade imports internal/%s", internal)
		}
		if forbidden(adapterFrameworks, imp) {
			t.Errorf("the facade imports %s", imp)
		}
	}
}

func TestEveryInternalPackageHasALayer(t *testing.T) {
	var dirs []string
	for _, pattern := range []string{"*", "app/*", "adapter/*"} {
		found, err := filepath.Glob(filepath.Join("internal", filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, found...)
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		name := filepath.ToSlash(strings.TrimPrefix(dir, "internal"+string(filepath.Separator)))
		if name == "app" || name == "adapter" {
			continue
		}
		if _, ok := layers[name]; !ok {
			t.Errorf("%s has no rule in layers", dir)
		}
	}
}
