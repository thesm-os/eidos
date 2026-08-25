// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/protobuf/frontend"
	"go.thesmos.sh/eidos/node"
)

// TestConvert_StampsFileOptions covers the file-level option
// channel: every file-level option (standard plus custom) lands
// on the producing [node.Package] under `proto.option.<full-name>`.
// Standard options use the option's well-known short name (e.g.
// `go_package`, `deprecated`, `optimize_for`); custom options use
// the extension's proto FQN. Values carry their natural Go type
// per the documented value-type mapping — strings as `string`,
// booleans as `bool`, enums as the variant `Name` string.
func TestConvert_StampsFileOptions(t *testing.T) {
	t.Parallel()

	t.Run("standard and custom file options stamp under the documented keys", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "fileoptions", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkg := requireSinglePackage(t, env)

		t.Run("go_package stamps as string under proto.option.go_package", func(t *testing.T) {
			t.Parallel()
			key := meta.EnsureKey(frontend.MetaOptionPrefix+"go_package", meta.StringParser)
			got, ok := key.Get(pkg.Meta())
			if !ok {
				t.Fatalf("expected proto.option.go_package stamp; not present")
			}
			const want = "github.com/example/fileoptions"
			if got != want {
				t.Fatalf("proto.option.go_package = %q, want %q", got, want)
			}
		})

		t.Run("deprecated stamps as bool under proto.option.deprecated", func(t *testing.T) {
			t.Parallel()
			key := meta.EnsureKey(frontend.MetaOptionPrefix+"deprecated", meta.BoolParser)
			got, ok := key.Get(pkg.Meta())
			if !ok {
				t.Fatalf("expected proto.option.deprecated stamp; not present")
			}
			if !got {
				t.Fatalf("proto.option.deprecated = false, want true")
			}
		})

		t.Run("optimize_for stamps as the variant name string", func(t *testing.T) {
			t.Parallel()
			key := meta.EnsureKey(frontend.MetaOptionPrefix+"optimize_for", meta.StringParser)
			got, ok := key.Get(pkg.Meta())
			if !ok {
				t.Fatalf("expected proto.option.optimize_for stamp; not present")
			}
			const want = "SPEED"
			if got != want {
				t.Fatalf("proto.option.optimize_for = %q, want %q", got, want)
			}
		})

		t.Run("custom string-typed extension stamps under its proto FQN", func(t *testing.T) {
			t.Parallel()
			const name = "eidos.protobuf.testdata.fileoptions.audience"
			key := meta.EnsureKey(frontend.MetaOptionPrefix+name, meta.StringParser)
			got, ok := key.Get(pkg.Meta())
			if !ok {
				t.Fatalf("expected proto.option.%s stamp; not present", name)
			}
			const want = "public"
			if got != want {
				t.Fatalf("proto.option.%s = %q, want %q", name, got, want)
			}
		})
	})
}

// TestConvert_StampsFileOptions_CrossFileCollision covers the
// silent-last-writer-wins guard: two .proto files contributing to
// the same proto package set conflicting values for the same
// file-level option. The converter stamps the second writer's
// value and surfaces a positioned diag.Warn so the override is
// observable rather than mysterious.
func TestConvert_StampsFileOptions_CrossFileCollision(t *testing.T) {
	t.Parallel()

	t.Run("conflicting go_package across two sibling files surfaces a positioned diag.Warn", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "fileoptions-collision", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		var matched *diag.Diag
		for i := range env.diag.Diagnostics() {
			d := &env.diag.Diagnostics()[i]
			if d.Plugin != frontend.FrontendName {
				continue
			}
			if d.Severity != diag.Warn {
				continue
			}
			if !strings.Contains(d.Message, "go_package") || !strings.Contains(d.Message, "overwrites prior value") {
				continue
			}
			matched = d
			break
		}
		if matched == nil {
			t.Fatalf(
				"expected a protobuf-frontend Warn mentioning go_package overwrite; got %+v",
				env.diag.Diagnostics(),
			)
		}
		if matched.Pos.File == "" {
			t.Fatalf("overwrite diagnostic should carry the offending file path; got %+v", matched.Pos)
		}
	})
}

// requireSinglePackage asserts that exactly one [node.Package]
// landed under env's NodeView and returns it. Used by tests whose
// fixtures emit a single package — the helper centralises the
// shape assertion so each test focuses on its option-specific
// readback.
func requireSinglePackage(t *testing.T, env fixtureEnv) *node.Package {
	t.Helper()
	pkgs := collectPackages(t, env)
	if got := len(pkgs); got != 1 {
		t.Fatalf("expected exactly 1 package; got %d (%+v)", got, packagePaths(pkgs))
	}
	return pkgs[0]
}
