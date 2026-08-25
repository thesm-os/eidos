// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"os"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/lang/golang/frontend"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// loadWithRegistry runs the Go frontend against src with a
// directive registry pre-populated with schemas. The helper is
// localised to validate_test.go because no other suite needs a
// registry-bound load.
func loadWithRegistry(t *testing.T, src map[string]string, schemas ...directive.Schema) (*store.Store, *diag.Sink) {
	t.Helper()
	reg := directive.NewRegistry()
	for _, sc := range schemas {
		if err := reg.Register(sc); err != nil {
			t.Fatalf("registry.Register(%v): %v", sc.Name, err)
		}
	}
	dir := materialiseGoSource(t, src)
	chdirMu.Lock()
	defer chdirMu.Unlock()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil { //nolint:usetesting // t.Chdir() rejects parallel tests
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(prev) }() //nolint:usetesting // t.Chdir() rejects parallel tests

	s := store.New()
	d := diag.New()
	fe := frontend.New()
	if err := fe.Load(&plugin.FrontendContext{
		Store:    s,
		Diag:     d,
		Parser:   directive.DefaultParser(),
		Registry: reg,
		Pattern:  "./...",
	}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s, d
}

// TestValidateDirectives covers the per-kind walk that hands every
// directive-bearing node to [directive.Validate]. We pre-register
// schemas with different AppliesTo restrictions so violations
// surface as positioned diagnostics.
func TestValidateDirectives(t *testing.T) {
	t.Parallel()
	t.Run("wrong-kind directive emits a positioned diagnostic", func(t *testing.T) {
		t.Parallel()
		schema := directive.NewSchema("repo").On(node.KindStruct).Build()
		_, d := loadWithRegistry(t, map[string]string{
			"a.go": "package a\n\n// +gen:repo\ntype I interface{ M() }\n",
		}, schema)
		if !d.HasErrors() {
			t.Fatalf("expected an Error diagnostic for repo on interface")
		}
	})

	t.Run("correct-kind directive passes validation", func(t *testing.T) {
		t.Parallel()
		schema := directive.NewSchema("repo").On(node.KindStruct).Build()
		_, d := loadWithRegistry(t, map[string]string{
			"a.go": "package a\n\n// +gen:repo\ntype S struct{}\n",
		}, schema)
		for _, dg := range d.Diagnostics() {
			if dg.Severity == diag.Error {
				t.Fatalf("unexpected error: %v", dg)
			}
		}
	})

	t.Run("unregistered directive parses without validation", func(t *testing.T) {
		t.Parallel()
		_, d := loadWithRegistry(t, map[string]string{
			"a.go": "package a\n\n// +gen:nosuch\ntype S struct{}\n",
		})
		for _, dg := range d.Diagnostics() {
			if dg.Severity == diag.Error {
				t.Fatalf("unregistered directive should not be flagged: %v", dg)
			}
		}
	})

	t.Run("nil registry skips validation entirely", func(t *testing.T) {
		t.Parallel()
		// loadFromSource passes Registry: nil through helpers_test.
		_, d := loadFromSource(t, map[string]string{
			"a.go": "package a\n\n// +gen:repo\ntype S struct{}\n",
		})
		for _, dg := range d.Diagnostics() {
			if dg.Severity == diag.Error {
				t.Fatalf("nil registry must skip validation: %v", dg)
			}
		}
	})

	t.Run("required-key violation surfaces with the key name", func(t *testing.T) {
		t.Parallel()
		schema := directive.NewSchema("repo").
			On(node.KindStruct).
			RequiredKeys("target").
			Build()
		_, d := loadWithRegistry(t, map[string]string{
			"a.go": "package a\n\n// +gen:repo\ntype S struct{}\n",
		}, schema)
		hit := false
		for _, dg := range d.Diagnostics() {
			if dg.Severity == diag.Error && strings.Contains(dg.Message, "target") {
				hit = true
			}
		}
		if !hit {
			t.Fatalf("expected diagnostic mentioning the missing 'target' key, got %+v", d.Diagnostics())
		}
	})
}

// TestValidate_PerKind asserts validateDirectives flags AppliesTo
// violations on each node kind the walk visits. Each subtest pairs
// a schema scoped to a single kind with a source that attaches the
// directive to a different kind — the validator must surface a
// positioned Error diagnostic for every such mismatch.
func TestValidate_PerKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		schemaKind kind.Kind
		violatedOn string // human-readable shape carrying the directive in the source
		source     string
	}{
		{
			name:       "package-kind schema rejects directive on a struct",
			schemaKind: node.KindPackage,
			violatedOn: "struct",
			source:     "package a\n\n// +gen:scoped\ntype S struct{}\n",
		},
		{
			name:       "file-kind schema rejects directive on a struct",
			schemaKind: node.KindFile,
			violatedOn: "struct",
			source:     "package a\n\n// +gen:scoped\ntype S struct{}\n",
		},
		{
			name:       "struct-kind schema rejects directive on an interface",
			schemaKind: node.KindStruct,
			violatedOn: "interface",
			source:     "package a\n\n// +gen:scoped\ntype I interface{ M() }\n",
		},
		{
			name:       "interface-kind schema rejects directive on a struct",
			schemaKind: node.KindInterface,
			violatedOn: "struct",
			source:     "package a\n\n// +gen:scoped\ntype S struct{}\n",
		},
		{
			name:       "method-kind schema rejects directive on a struct",
			schemaKind: node.KindMethod,
			violatedOn: "struct",
			source:     "package a\n\n// +gen:scoped\ntype S struct{}\n",
		},
		{
			name:       "field-kind schema rejects directive on a struct",
			schemaKind: node.KindField,
			violatedOn: "struct",
			source:     "package a\n\n// +gen:scoped\ntype S struct{}\n",
		},
		{
			name:       "function-kind schema rejects directive on a struct",
			schemaKind: node.KindFunction,
			violatedOn: "struct",
			source:     "package a\n\n// +gen:scoped\ntype S struct{}\n",
		},
		{
			name:       "variable-kind schema rejects directive on a struct",
			schemaKind: node.KindVariable,
			violatedOn: "struct",
			source:     "package a\n\n// +gen:scoped\ntype S struct{}\n",
		},
		{
			name:       "constant-kind schema rejects directive on a struct",
			schemaKind: node.KindConstant,
			violatedOn: "struct",
			source:     "package a\n\n// +gen:scoped\ntype S struct{}\n",
		},
		{
			name:       "enum-kind schema rejects directive on a struct",
			schemaKind: node.KindEnum,
			violatedOn: "struct",
			source:     "package a\n\n// +gen:scoped\ntype S struct{}\n",
		},
		{
			name:       "alias-kind schema rejects directive on a struct",
			schemaKind: node.KindAlias,
			violatedOn: "struct",
			source:     "package a\n\n// +gen:scoped\ntype S struct{}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			schema := directive.NewSchema("scoped").On(tc.schemaKind).Build()
			_, d := loadWithRegistry(t, map[string]string{"a.go": tc.source}, schema)
			hits := 0
			for _, dg := range d.Diagnostics() {
				if dg.Severity == diag.Error {
					hits++
				}
			}
			if hits == 0 {
				t.Fatalf("expected at least one Error diagnostic when schema kind %q is violated on %s, got %+v",
					tc.schemaKind, tc.violatedOn, d.Diagnostics())
			}
		})
	}
}

// TestValidate_VisitsEveryKind exercises the per-kind iterations in
// validateDirectives that the AppliesTo-only test cannot reach: the
// iteration body only runs when the corresponding c.out slice is
// non-empty AND a registry is configured. Each subtest attaches a
// matching schema on a real directive-bearing declaration of the
// kind under test so the iteration runs end-to-end.
func TestValidate_VisitsEveryKind(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		kind   kind.Kind
		source string
	}{
		{
			name:   "package directive",
			kind:   node.KindPackage,
			source: "// +gen:scoped\npackage a\n",
		},
		{
			name:   "import directive",
			kind:   node.KindImport,
			source: "package a\n\nimport (\n\t// +gen:scoped\n\t\"fmt\"\n)\n\nvar _ = fmt.Sprintf\n",
		},
		{
			name:   "variable directive",
			kind:   node.KindVariable,
			source: "package a\n\n// +gen:scoped\nvar X = 1\n",
		},
		{
			name:   "constant directive (untyped, not enum-promoted)",
			kind:   node.KindConstant,
			source: "package a\n\n// +gen:scoped\nconst Pi = 3.14\n",
		},
		{
			name:   "alias directive",
			kind:   node.KindAlias,
			source: "package a\n\n// +gen:scoped\ntype ID string\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			schema := directive.NewSchema("scoped").On(tc.kind).Build()
			_, d := loadWithRegistry(t, map[string]string{"a.go": tc.source}, schema)
			for _, dg := range d.Diagnostics() {
				if dg.Severity == diag.Error {
					t.Fatalf("matching schema for %q should pass; got %v", tc.kind, dg)
				}
			}
		})
	}

	t.Run("enum and variant directives", func(t *testing.T) {
		t.Parallel()
		// Enum-on-enum and enum-on-variant both need their own
		// matching schemas so each iteration body fires cleanly.
		enumSchema := directive.NewSchema("enumdir").On(node.KindEnum).Build()
		variantSchema := directive.NewSchema("variantdir").On(node.KindEnumVariant).Build()
		_, d := loadWithRegistry(t, map[string]string{
			"a.go": "package a\n\n// +gen:enumdir\ntype Status int\n\nconst (\n\t// +gen:variantdir\n\tStatusActive Status = iota\n\tStatusInactive\n)\n",
		}, enumSchema, variantSchema)
		for _, dg := range d.Diagnostics() {
			if dg.Severity == diag.Error {
				t.Fatalf("matching enum/variant schemas should pass; got %v", dg)
			}
		}
	})
}

// TestValidate_DescendsIntoChildren asserts the per-host walks
// (validateStructDirectives, validateInterfaceDirectives,
// validateFunctionDirectives) reach every directive-bearing child:
// struct fields, struct embeds, interface embeds, function params.
func TestValidate_DescendsIntoChildren(t *testing.T) {
	t.Parallel()

	t.Run("validateStructDirectives flags a field-kind violation on an embed", func(t *testing.T) {
		t.Parallel()
		// A field-only schema is violated when the directive
		// appears on an embed inside a struct.
		schema := directive.NewSchema("fieldonly").On(node.KindField).Build()
		_, d := loadWithRegistry(t, map[string]string{
			"a.go": "package a\n\ntype I interface{ M() }\n\ntype S struct {\n\t// +gen:fieldonly\n\tI\n}\n",
		}, schema)
		if !d.HasErrors() {
			t.Fatalf("expected Error for embed carrying a field-only schema, got %+v", d.Diagnostics())
		}
	})

	t.Run("validateFunctionDirectives flags a struct-kind violation on a function", func(t *testing.T) {
		t.Parallel()
		// A struct-only schema is violated when the directive
		// appears on a top-level function — drives the
		// validateFunctionDirectives entry.
		schema := directive.NewSchema("structonly").On(node.KindStruct).Build()
		_, d := loadWithRegistry(t, map[string]string{
			"a.go": "package a\n\n// +gen:structonly\nfunc Do() {}\n",
		}, schema)
		if !d.HasErrors() {
			t.Fatalf("expected Error for function carrying a struct-only schema, got %+v", d.Diagnostics())
		}
	})

	t.Run("validateFunctionDirectives flags a struct-kind violation on a function param", func(t *testing.T) {
		t.Parallel()
		// A struct-only schema is violated when the directive
		// appears on a function parameter — drives the param
		// loop inside validateFunctionDirectives.
		schema := directive.NewSchema("structonly").On(node.KindStruct).Build()
		_, d := loadWithRegistry(t, map[string]string{
			"a.go": "package a\n\nfunc Do(\n\t// +gen:structonly\n\tid string,\n) {}\n",
		}, schema)
		if !d.HasErrors() {
			t.Fatalf("expected Error for param carrying a struct-only schema, got %+v", d.Diagnostics())
		}
	})

	t.Run("validateInterfaceDirectives flags a method-kind violation on an embed", func(t *testing.T) {
		t.Parallel()
		// A method-only schema is violated when the directive
		// appears on an interface embed.
		schema := directive.NewSchema("methodonly").On(node.KindMethod).Build()
		_, d := loadWithRegistry(t, map[string]string{
			"a.go": "package a\n\ntype Base interface{ Close() error }\n\ntype Ext interface{\n\t// +gen:methodonly\n\tBase\n\tDo()\n}\n",
		}, schema)
		if !d.HasErrors() {
			t.Fatalf("expected Error for interface embed carrying a method-only schema, got %+v", d.Diagnostics())
		}
	})

	t.Run("validateStructDirectives walks fields, embeds, and methods", func(t *testing.T) {
		t.Parallel()
		// Three matching schemas drive every child iteration in
		// validateStructDirectives. We assert no Error fires — the
		// goal is statement coverage of each Validate call body, not
		// validation failure.
		schemas := []directive.Schema{
			directive.NewSchema("fielddir").On(node.KindField).Build(),
			directive.NewSchema("embeddir").On(node.KindEmbed).Build(),
			directive.NewSchema("methoddir").On(node.KindMethod).Build(),
		}
		_, d := loadWithRegistry(t, map[string]string{
			"a.go": "package a\n\ntype Base struct{}\n\ntype S struct {\n\t// +gen:fielddir\n\tID string\n\t// +gen:embeddir\n\tBase\n}\n\n// +gen:methoddir\nfunc (S) Do() {}\n",
		}, schemas...)
		for _, dg := range d.Diagnostics() {
			if dg.Severity == diag.Error {
				t.Fatalf("matching field/embed/method schemas should pass; got %v", dg)
			}
		}
	})
}
