// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"errors"
	"strconv"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/node"
)

// TestPackageBuilder_AllDeclKindsLand covers every decl kind the
// [builder.PackageBuilder] exposes: each appends to the underlying
// package, inherits the context target, and surfaces through
// [builder.PackageBuilder.Build].
func TestPackageBuilder_AllDeclKindsLand(t *testing.T) {
	t.Parallel()

	t.Run("each decl method appends one entity to the package", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		pkg, err := c.Package("users", "example.com/users").
			Struct("S", nil).
			Interface("I", nil).
			Function("F", nil).
			Enum("E", emit.Builtin("int"), nil).
			Alias("A", emit.Builtin("string"), nil).
			NamedType("N", emit.Builtin("int"), nil).
			Variable("V", emit.Builtin("int"), nil, nil).
			Constant("C", emit.Builtin("int"),
				&emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"}, nil).
			Build()
		if err != nil {
			t.Fatalf("Build returned %v", err)
		}
		if pkg.Name != "users" || pkg.Path != "example.com/users" {
			t.Fatalf("pkg identity wrong; got %q / %q", pkg.Name, pkg.Path)
		}
		if len(pkg.Structs) != 1 || pkg.Structs[0].Name != "S" {
			t.Fatalf("expected one struct S; got %+v", pkg.Structs)
		}
		if len(pkg.Interfaces) != 1 || pkg.Interfaces[0].Name != "I" {
			t.Fatalf("expected one interface I; got %+v", pkg.Interfaces)
		}
		if len(pkg.Functions) != 1 || pkg.Functions[0].Name != "F" {
			t.Fatalf("expected one function F; got %+v", pkg.Functions)
		}
		if len(pkg.Enums) != 1 || pkg.Enums[0].Name != "E" {
			t.Fatalf("expected one enum E; got %+v", pkg.Enums)
		}
		if len(pkg.Aliases) != 2 {
			t.Fatalf("expected one Alias + one NamedType; got %d", len(pkg.Aliases))
		}
		if !pkg.Aliases[0].IsAlias {
			t.Fatalf("first alias should be a true alias")
		}
		if pkg.Aliases[1].IsAlias {
			t.Fatalf("second alias (NamedType) should not be a true alias")
		}
		if len(pkg.Variables) != 1 || pkg.Variables[0].Name != "V" {
			t.Fatalf("expected one variable V; got %+v", pkg.Variables)
		}
		if len(pkg.Constants) != 1 || pkg.Constants[0].Name != "C" {
			t.Fatalf("expected one constant C; got %+v", pkg.Constants)
		}
	})

	t.Run("decls inherit the context target", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		pkg, err := c.Package("users", "example.com/users").
			Struct("S", nil).
			Function("F", nil).
			Build()
		if err != nil {
			t.Fatalf("Build returned %v", err)
		}
		if pkg.Structs[0].Target != defaultTarget {
			t.Fatalf("struct target = %v, want %v", pkg.Structs[0].Target, defaultTarget)
		}
		if pkg.Functions[0].Target != defaultTarget {
			t.Fatalf("function target = %v, want %v", pkg.Functions[0].Target, defaultTarget)
		}
	})
}

// TestPackageBuilder_DocsAndNode covers the [builder.PackageBuilder]
// docs and Node accessors.
func TestPackageBuilder_DocsAndNode(t *testing.T) {
	t.Parallel()

	t.Run("Docs appends lines on the underlying package", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		pkg, err := c.Package("p", "p").Docs("package docs").Build()
		if err != nil {
			t.Fatalf("Build returned %v", err)
		}
		if len(pkg.DocLines) != 1 || pkg.DocLines[0] != "package docs" {
			t.Fatalf("package docs not appended; got %+v", pkg.DocLines)
		}
	})

	t.Run("Node returns the underlying *emit.Package", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		b := c.Package("p", "p")
		if b.Node() == nil {
			t.Fatalf("Node returned nil")
		}
	})

	t.Run("Err mirrors Build's error component", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		// A clean Package returns no error.
		b := c.Package("p", "p")
		if got := b.Err(); got != nil {
			t.Fatalf("Err on clean package should be nil; got %v", got)
		}
	})
}

// TestPackageBuilder_File covers the sub-context that
// [builder.PackageBuilder.File] returns: decls built through
// `pkg.File(tag)` stamp BaseEmit.OutputTag = tag while sharing
// the same underlying [emit.Package], context, and Anchor
// default-origin. The sub-context is memoised per tag; identity
// for the empty tag; nested calls overwrite rather than compose.
func TestPackageBuilder_File(t *testing.T) {
	t.Parallel()

	t.Run("decls built through File(tag) stamp OutputTag = tag", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		pkg, err := c.Package("p", "example.com/p").
			Struct("Production", nil).
			File("test").Struct("Helper", nil).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if pkg.Structs[0].OutputTag() != "" {
			t.Errorf("Production struct OutputTag = %q, want empty", pkg.Structs[0].OutputTag())
		}
		if pkg.Structs[1].OutputTag() != "test" {
			t.Errorf("Helper struct OutputTag = %q, want %q", pkg.Structs[1].OutputTag(), "test")
		}
	})

	t.Run("File(tag) shares the underlying emit.Package with the parent", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		root := c.Package("p", "example.com/p")
		sub := root.File("test")
		if root.Node() != sub.Node() {
			t.Fatalf("sub-context should share the parent's *emit.Package; got distinct pointers")
		}
	})

	t.Run("File(\"\") is the identity form (returns the parent unchanged)", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		root := c.Package("p", "example.com/p")
		if root.File("") != root {
			t.Fatalf("File(\"\") should return the same *PackageBuilder; got a distinct pointer")
		}
	})

	t.Run("File(tag) is memoised — same tag returns the same sub-context", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		root := c.Package("p", "example.com/p")
		first := root.File("test")
		second := root.File("test")
		if first != second {
			t.Fatalf("two File(\"test\") calls returned distinct sub-contexts")
		}
	})

	t.Run("nested File call overwrites the tag (not composed)", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		root := c.Package("p", "example.com/p")
		nested := root.File("a").File("b")
		// The sub-context for "b" should equal the sub-context
		// reachable directly off the root — File("a").File("b")
		// resolves to root's "b" sub-context, not a synthesised
		// "a.b" composite.
		if nested != root.File("b") {
			t.Fatalf("File(\"a\").File(\"b\") returned a non-root \"b\" sub-context")
		}
	})

	t.Run("decls built through nested File chain stamp the outer tag", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		pkg, err := c.Package("p", "example.com/p").
			File("a").File("b").Struct("B", nil).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if pkg.Structs[0].OutputTag() != "b" {
			t.Errorf("nested-chain Struct OutputTag = %q, want %q", pkg.Structs[0].OutputTag(), "b")
		}
	})

	t.Run("every decl kind on the sub-context stamps OutputTag", func(t *testing.T) {
		t.Parallel()
		anchor := &node.Enum{Name: "Status", Package: "example.com/p"}
		pkg, err := builder.For("test").Anchor(anchor).
			File("test").Struct("S", nil).
			File("test").Interface("I", nil).
			File("test").Function("F", nil).
			File("test").Enum("E", emit.Builtin("int"), nil).
			File("test").Alias("A", emit.Builtin("string"), nil).
			File("test").NamedType("N", emit.Builtin("int"), nil).
			File("test").Variable("V", emit.Builtin("int"), nil, nil).
			File("test").Constant("C", emit.Builtin("int"),
			&emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"}, nil).
			File("test").Method("M", nil).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		checks := []struct {
			name string
			tag  string
		}{
			{"Struct", pkg.Structs[0].OutputTag()},
			{"Interface", pkg.Interfaces[0].OutputTag()},
			{"Function", pkg.Functions[0].OutputTag()},
			{"Enum", pkg.Enums[0].OutputTag()},
			{"Alias (true)", pkg.Aliases[0].OutputTag()},
			{"Alias (NamedType)", pkg.Aliases[1].OutputTag()},
			{"Variable", pkg.Variables[0].OutputTag()},
			{"Constant", pkg.Constants[0].OutputTag()},
			{"Method", pkg.Methods[0].OutputTag()},
		}
		for _, c := range checks {
			if c.tag != "test" {
				t.Errorf("%s OutputTag = %q, want %q", c.name, c.tag, "test")
			}
		}
	})

	t.Run("errors recorded by a sub-context surface through the root's Build", func(t *testing.T) {
		t.Parallel()
		// True alias accepting a method body is the recorded
		// structural violation we test against. Building a true
		// alias and attempting a Method on it records the error
		// via the package builder's recordErr; that record must
		// be visible from the root's Build() even when the
		// violation originated from a sub-context call.
		c := builder.For("test").WithTarget(defaultTarget)
		_, err := c.Package("p", "example.com/p").
			File("test").Alias("A", emit.Builtin("int"), func(ab *builder.AliasBuilder) {
			ab.Method("Bad", nil)
		}).
			Build()
		if err == nil {
			t.Fatalf("Build should report the true-alias method violation recorded from a File sub-context")
		}
	})
}

// BenchmarkContext_Package measures the generator hot path: one
// plugin pass building a whole package of entities — n structs, each
// with six fields and one method — and closing it with Build.
//
// This is the allocation profile every generator pays per run, and it
// is dominated by graph construction rather than by any single
// builder call, so the interesting number is per-struct cost. The
// scaling axis exists to prove that cost is flat: the builder appends
// to slices and wires Owner back-pointers as callbacks return, all of
// which should be linear in the number of decls. A per-decl scan of
// the accumulated package would surface here as quadratic.
//
// Deliberately outside the timed region: the struct and field name
// strings, and the shared type refs. Formatting names inside the loop
// would measure strconv, and refs are immutable values a real plugin
// would equally hoist. Everything from [builder.For] onwards is
// inside, because a plugin pays for the Context and the accumulating
// package on every run.
func BenchmarkContext_Package(b *testing.B) {
	const fieldsPerStruct = 6

	for _, structs := range []int{1, 10, 100, 1000} {
		b.Run(strconv.Itoa(structs), func(b *testing.B) {
			b.ReportAllocs()

			structNames := make([]string, structs)
			for i := range structNames {
				structNames[i] = "Entity" + strconv.Itoa(i)
			}
			fieldNames := make([]string, fieldsPerStruct)
			for i := range fieldNames {
				fieldNames[i] = "field" + strconv.Itoa(i)
			}
			stringRef := emit.Builtin("string")
			ctxRef := emit.External("context", "Context")
			errRef := emit.Builtin("error")

			for b.Loop() {
				pb := builder.For("bench").Package("bench", "example.com/bench")
				for _, name := range structNames {
					pb.Struct(name, func(s *builder.StructBuilder) {
						for _, field := range fieldNames {
							s.Field(field, stringRef, nil)
						}
						s.Method("Save", func(m *builder.MethodBuilder) {
							m.Receiver("e", emit.Ptr(emit.Internal(s.Node())))
							m.Param("ctx", ctxRef)
							m.Return(errRef)
						})
					})
				}
				if _, err := pb.Build(); err != nil {
					b.Fatalf("Build: %v", err)
				}
			}
		})
	}
}

// TestPackageBuilder_Err pins the error sink's sharing rule. A
// sub-context created by File(tag) shares the root's error slice,
// so a violation recorded anywhere in the chain is visible from
// every builder in it — a caller probing Err on the sub-context it
// happens to hold must not see a clean bill because the violation
// was recorded elsewhere.
func TestPackageBuilder_Err(t *testing.T) {
	t.Parallel()

	t.Run("a clean builder reports no error", func(t *testing.T) {
		t.Parallel()
		if err := builder.For("mg").Package("x", "example.com/x").Err(); err != nil {
			t.Fatalf("Err = %v, want nil", err)
		}
	})

	t.Run("surfaces a structural violation recorded on the root", func(t *testing.T) {
		t.Parallel()
		b := builder.For("mg").Package("x", "example.com/x")
		b.Alias("ID", emit.Builtin("string"), func(ab *builder.AliasBuilder) {
			ab.Method("String", nil)
		})
		if err := b.Err(); !errors.Is(err, builder.ErrAliasMethodForbidden) {
			t.Fatalf("Err = %v, want ErrAliasMethodForbidden", err)
		}
	})

	t.Run("a sub-context reports the root's error", func(t *testing.T) {
		t.Parallel()
		b := builder.For("mg").Package("x", "example.com/x")
		b.Alias("ID", emit.Builtin("string"), func(ab *builder.AliasBuilder) {
			ab.Method("String", nil)
		})
		if err := b.File("extra").Err(); !errors.Is(err, builder.ErrAliasMethodForbidden) {
			t.Fatalf("sub-context Err = %v, want the root's ErrAliasMethodForbidden", err)
		}
	})

	t.Run("a clean sub-context reports no error", func(t *testing.T) {
		t.Parallel()
		b := builder.For("mg").Package("x", "example.com/x")
		if err := b.File("extra").Err(); err != nil {
			t.Fatalf("sub-context Err = %v, want nil", err)
		}
	})
}
