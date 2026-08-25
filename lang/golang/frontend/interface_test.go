// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
)

// TestConvertInterface covers the per-interface conversion path:
// methods, embeds, generic params, and method-body shapes.
func TestConvertInterface(t *testing.T) {
	t.Parallel()
	t.Run("interface carries name and package path", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Op interface{ Do() }\n",
		})
		i := pkg.InterfaceByName("Op")
		if i == nil || i.Name != "Op" {
			t.Fatalf("Op interface missing")
		}
	})

	t.Run("explicit methods carry params and returns", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"context\"\n\ntype Reader interface{\n\tRead(ctx context.Context, key string) ([]byte, error)\n}\n",
		})
		i := pkg.InterfaceByName("Reader")
		if i == nil {
			t.Fatalf("Reader missing")
		}
		m := i.Methods[0]
		if m.Name != "Read" {
			t.Fatalf("method name = %q", m.Name)
		}
		if len(m.Params) != 2 {
			t.Fatalf("expected 2 params, got %d", len(m.Params))
		}
		if m.Params[0].Name != "ctx" || m.Params[1].Name != "key" {
			t.Fatalf("param names = %q,%q", m.Params[0].Name, m.Params[1].Name)
		}
		if len(m.Returns) != 2 {
			t.Fatalf("expected 2 returns, got %d", len(m.Returns))
		}
	})

	t.Run("embedded interface surfaces as Embed", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Base interface{ Close() error }\ntype Ext interface{ Base; Do() }\n",
		})
		i := pkg.InterfaceByName("Ext")
		if i == nil {
			t.Fatalf("Ext missing")
		}
		if len(i.Embeds) != 1 || i.Embeds[0].Type.Name != "Base" {
			t.Fatalf("expected one embed of Base, got %+v", i.Embeds)
		}
	})

	t.Run("variadic last param records Variadic and element type", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Op interface{ Do(args ...int) }\n",
		})
		m := pkg.InterfaceByName("Op").Methods[0]
		last := m.Params[len(m.Params)-1]
		if !last.Variadic {
			t.Fatalf("expected last param to be variadic")
		}
		if last.Type == nil || last.Type.Name != "int" {
			t.Fatalf("variadic element type = %+v, want int", last.Type)
		}
	})

	t.Run("generic interface carries type parameters", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Op[T any] interface{ Do(v T) T }\n",
		})
		i := pkg.InterfaceByName("Op")
		if len(i.TypeParams) != 1 || i.TypeParams[0].Name != "T" {
			t.Fatalf("expected one type-param T, got %+v", i.TypeParams)
		}
	})

	t.Run("alias of an interface populates methods via the type-only path", func(t *testing.T) {
		t.Parallel()
		// `type Alias = Original` of an interface surfaces as a
		// fresh Interface whose body is built by
		// [populateInterfaceFromTypeOnly] because the AST type
		// expression is an Ident rather than an InterfaceType.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Original interface{ Do() }\n\ntype Alias = Original\n",
		})
		alias := pkg.InterfaceByName("Alias")
		if alias == nil {
			t.Fatalf("alias of interface did not surface; the type-only path is what this test exists to exercise")
		}
		if len(alias.Methods) == 0 {
			t.Fatalf("Alias must carry methods populated by the type-only path")
		}
	})

	t.Run("alias of an interface preserves rich method signatures", func(t *testing.T) {
		t.Parallel()
		// Drives [methodFromSignature] through every body branch:
		// params, variadic, and returns.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Original interface{\n\tDo(id string, parts ...int) (bool, error)\n}\n\ntype Alias = Original\n",
		})
		alias := pkg.InterfaceByName("Alias")
		if alias == nil {
			t.Fatalf("alias of interface did not surface; the type-only path is what this test exists to exercise")
		}
		if len(alias.Methods) != 1 {
			t.Fatalf("expected 1 method, got %d", len(alias.Methods))
		}
		m := alias.Methods[0]
		if len(m.Params) != 2 || len(m.Returns) != 2 {
			t.Fatalf("expected 2 params + 2 returns, got %d/%d", len(m.Params), len(m.Returns))
		}
		if !m.Params[1].Variadic {
			t.Fatalf("expected last param to be variadic")
		}
	})

	t.Run("alias of an interface with embeds carries the embed via type-only path", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Base interface{ Close() error }\n\ntype Original interface{ Base; Do() }\n\ntype Alias = Original\n",
		})
		alias := pkg.InterfaceByName("Alias")
		if alias == nil {
			t.Fatalf("alias of interface did not surface; the type-only path is what this test exists to exercise")
		}
		if len(alias.Embeds) == 0 {
			t.Fatalf("expected at least one embed on the aliased interface")
		}
	})

	t.Run("constraint-interface type-set surfaces as embedded ref", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Numeric interface{ ~int | ~float64 }\n",
		})
		i := pkg.InterfaceByName("Numeric")
		if i == nil {
			t.Fatalf("Numeric missing")
		}
		// Type-set unions surface through go.constraintTerms meta
		// when carried on a type-param; on the interface itself we
		// only assert the structural shape and constraint meta.
		if len(i.Methods) != 0 {
			t.Fatalf("constraint-only interface should have no methods, got %d", len(i.Methods))
		}
	})

	t.Run("embedded type the type-checker could not resolve does not crash the converter", func(t *testing.T) {
		t.Parallel()
		// Drives typeRefForInterfaceEmbed's nil-typeinfo return; the
		// converter must surface a diagnostic and continue.
		_, d := loadFromSource(t, map[string]string{
			"a.go": "package a\n\ntype I interface{ Missing }\n",
		})
		if !d.HasErrors() {
			t.Fatalf("expected an Error diagnostic for unresolved interface embed")
		}
	})
}

// TestAppendInterfaceMethod_BlankNameDoesNotPanic pins that an
// interface method the type-checker refused to record degrades to
// the loader's own diagnostic rather than taking the converter down.
//
// go/types rejects a blank method name and omits it from
// ExplicitMethods, while the AST field survives — so the name scan
// in appendInterfaceMethod completes with no match. Dereferencing
// that nil unwound out of converter.run, discarding every
// declaration in the package, not just the malformed interface: the
// user saw a valid syntax error with a framework panic stapled to
// it, and no output for a package that was otherwise convertible.
// Direct Load callers took an uncontained crash.
func TestAppendInterfaceMethod_BlankNameDoesNotPanic(t *testing.T) {
	t.Parallel()

	cases := map[string]map[string]string{
		"blank method name": {
			"a.go": "package p\n\ntype I interface {\n\t_()\n}\n",
		},
		// The blank name sits beside a valid method and a valid
		// type, so a fix that skipped the whole interface — or the
		// whole file — still fails this case.
		"blank name beside valid declarations": {
			"a.go": "package p\n\ntype I interface {\n\t_()\n\tOK() error\n}\n\ntype Fine struct{ N int }\n",
		},
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// The type error itself must still surface; the panic
			// must not. Asserting only on completion would pass
			// against a converter that panicked, since the pipeline
			// recovers a panicking plugin into a diagnostic.
			_, d := loadFromSource(t, src)
			if !d.HasErrors() {
				t.Fatalf("a blank interface method name must be reported; got no errors")
			}
			assertNoPanicDiagnostic(t, d)
		})
	}
}

// TestAppendInterfaceMethod_BlankNameKeepsSiblings pins the point of
// the fix rather than merely its absence of a crash: the rest of the
// package converts. A guard that returned early from the enclosing
// file or package walk would satisfy the no-panic assertion above
// while still discarding everything.
func TestAppendInterfaceMethod_BlankNameKeepsSiblings(t *testing.T) {
	t.Parallel()

	load := func(t *testing.T) *node.Package {
		t.Helper()
		st, _ := loadFromSource(t, map[string]string{
			"a.go": "package p\n\ntype I interface {\n\t_()\n\tOK() error\n}\n\ntype Fine struct{ N int }\n",
		})
		pkg := firstPackageIn(st, "")
		if pkg == nil {
			t.Fatalf("a package with one malformed interface must still convert")
		}
		return pkg
	}

	t.Run("an unrelated declaration in the same file survives", func(t *testing.T) {
		t.Parallel()
		// The panic unwound out of converter.run, so the whole
		// package was discarded. This is the assertion that fails
		// against a fix which bails out of the file or package walk.
		for _, s := range load(t).Structs {
			if s.Name == "Fine" {
				return
			}
		}
		t.Fatalf("struct Fine was discarded; only the malformed method should be skipped")
	})

	t.Run("the interface keeps its valid method and drops only the blank one", func(t *testing.T) {
		t.Parallel()
		for _, i := range load(t).Interfaces {
			if i.Name != "I" {
				continue
			}
			names := make([]string, 0, len(i.Methods))
			for _, m := range i.Methods {
				names = append(names, m.Name)
			}
			if len(names) != 1 || names[0] != "OK" {
				t.Fatalf("interface I methods = %v, want just [OK]", names)
			}
		}
	})
}

// TestInterfaceMethod_TrailingDirective is the interface-method twin
// of [TestStructField_TrailingDirective].
//
// It matters independently: a method-scoped directive written on the
// signature line is the natural place for it, and dropping it
// silently produces a generated double missing whatever the directive
// asked for, with nothing to explain the absence.
func TestInterfaceMethod_TrailingDirective(t *testing.T) {
	t.Parallel()

	t.Run("a directive after the method attaches to it", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Store interface {\n\tGet(key string) (string, error) // +gen:nonzero\n}\n",
		})
		m := pkg.InterfaceByName("Store").MethodByName("Get")
		if m == nil {
			t.Fatalf("Get method missing")
		}
		if len(m.DirectiveList) != 1 || m.DirectiveList[0].Name != "nonzero" {
			t.Fatalf("expected one +gen:nonzero directive, got %+v", m.DirectiveList)
		}
	})

	t.Run("both positions contribute, in source order", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Store interface {\n\t// +gen:nonzero\n\tGet(key string) (string, error) // +gen:secret\n}\n",
		})
		m := pkg.InterfaceByName("Store").MethodByName("Get")
		if len(m.DirectiveList) != 2 {
			t.Fatalf("expected both directives, got %+v", m.DirectiveList)
		}
		if m.DirectiveList[0].Name != "nonzero" || m.DirectiveList[1].Name != "secret" {
			t.Fatalf("directive order = %+v, want [nonzero secret]", m.DirectiveList)
		}
	})
}

// TestConvertInterface_ForeignEmbedProjection covers the shape that
// could not be generated for at all: an interface embedding a
// standard-library one.
//
// The method set is type-checked by the time conversion runs, and
// recording only a type reference discarded it, so the walk reported
// ReasonUnresolved and every consumer's correct response was to emit
// nothing.
func TestConvertInterface_ForeignEmbedProjection(t *testing.T) {
	t.Parallel()

	const src = "package a\n\nimport (\n\t\"context\"\n\t\"io\"\n)\n\n" +
		"type Stream interface {\n\tio.Closer\n\n" +
		"\tRead(ctx context.Context, key string) (string, error)\n}\n"

	t.Run("a standard-library embed carries its projection", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{"a.go": src})
		i := pkg.InterfaceByName("Stream")
		if i == nil || len(i.Embeds) != 1 {
			t.Fatalf("Stream missing or has %d embeds", len(i.Embeds))
		}
		resolved := i.Embeds[0].Resolved
		if resolved == nil {
			t.Fatal("io.Closer embed carries no projection")
		}
		if resolved.Package != "io" || resolved.Name != "Closer" {
			t.Errorf("projection = %s.%s, want io.Closer", resolved.Package, resolved.Name)
		}
		if len(resolved.Methods) != 1 || resolved.Methods[0].Name != "Close" {
			t.Errorf("projected methods = %+v, want Close", resolved.Methods)
		}
	})

	t.Run("the method set completes without an issue", func(t *testing.T) {
		t.Parallel()
		// The whole point: a consumer that refused this declaration
		// because a method was missing now has every one of them.
		pkg := requirePackage(t, map[string]string{"a.go": src})
		got := node.MethodSet(pkg.InterfaceByName("Stream"), nil)
		if len(got.Issues) != 0 {
			t.Fatalf("Issues = %+v, want none", got.Issues)
		}
		names := map[string]bool{}
		for _, m := range got.Methods {
			names[m.Name] = true
		}
		if !names["Close"] || !names["Read"] {
			t.Errorf("method set = %v, want both Close and Read", names)
		}
	})

	t.Run("a same-package embed carries no projection", func(t *testing.T) {
		t.Parallel()
		// Its own conversion is the better answer, and duplicating it
		// would put two versions of one declaration in the graph.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Base interface{ Do() }\n\ntype Op interface{ Base }\n",
		})
		i := pkg.InterfaceByName("Op")
		if i == nil || len(i.Embeds) != 1 {
			t.Fatalf("Op missing or has %d embeds", len(i.Embeds))
		}
		if i.Embeds[0].Resolved != nil {
			t.Error("a same-package embed carries a projection it does not need")
		}
	})
}
