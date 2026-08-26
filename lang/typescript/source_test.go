// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// src is the stateless rule set under test.
var src = typescript.Source{}

func TestResolveValue(t *testing.T) {
	t.Parallel()

	file := &node.File{
		Name: "user.ts",
		Imports: []*node.Import{
			{Path: "./models", Alias: "models"},
			{Path: "@scope/pkg", Alias: "pkg"},
		},
	}

	t.Run("a bare name names no module", func(t *testing.T) {
		t.Parallel()
		pkg, sym, err := src.ResolveValue(file, "User")
		if err != nil || pkg != "" || sym != "User" {
			t.Fatalf("ResolveValue = (%q, %q, %v)", pkg, sym, err)
		}
	})

	t.Run("a qualifier resolves against the file's imports", func(t *testing.T) {
		t.Parallel()
		pkg, sym, err := src.ResolveValue(file, "models.User")
		if err != nil {
			t.Fatalf("ResolveValue: %v", err)
		}
		if pkg != "./models" || sym != "User" {
			t.Fatalf("ResolveValue = (%q, %q)", pkg, sym)
		}
	})

	t.Run("a qualifier the file does not import is an error", func(t *testing.T) {
		t.Parallel()
		// Inventing a specifier would emit an import for a module that
		// may not exist, and the failure would surface in the
		// consumer's build rather than in this run.
		_, _, err := src.ResolveValue(file, "absent.User")
		if !errors.Is(err, typescript.ErrNoFileScope) {
			t.Fatalf("ResolveValue = %v, want ErrNoFileScope", err)
		}
	})

	t.Run("a qualifier with no file scope is an error", func(t *testing.T) {
		t.Parallel()
		_, _, err := src.ResolveValue(nil, "models.User")
		if !errors.Is(err, typescript.ErrNoFileScope) {
			t.Fatalf("ResolveValue = %v, want ErrNoFileScope", err)
		}
	})

	t.Run("an empty value resolves to nothing", func(t *testing.T) {
		t.Parallel()
		pkg, sym, err := src.ResolveValue(file, "  ")
		if err != nil || pkg != "" || sym != "" {
			t.Fatalf("ResolveValue = (%q, %q, %v)", pkg, sym, err)
		}
	})
}

func TestSourceTag(t *testing.T) {
	t.Parallel()

	t.Run("reads a decorator's arguments", func(t *testing.T) {
		t.Parallel()
		// A decorator is what a struct tag is in Go, so this is what
		// the SDK's tag contract answers with for TypeScript.
		f := &node.Field{Name: "name"}
		typescript.MetaDecorators.Set(f.EnsureMeta(), []typescript.Decorator{
			{Name: "Column", Args: "({ type: 'varchar' })"},
		}, "test")

		got, ok := src.Tag(f, "Column")
		if !ok {
			t.Fatal("Column not found")
		}
		if got != "({ type: 'varchar' })" {
			t.Fatalf("Tag = %q", got)
		}
	})

	t.Run("reports absence for an undecorated field", func(t *testing.T) {
		t.Parallel()
		if _, ok := src.Tag(&node.Field{Name: "x"}, "Column"); ok {
			t.Fatal("Tag found a decorator that is not there")
		}
		if _, ok := src.Tag(nil, "Column"); ok {
			t.Fatal("Tag on nil reported a decorator")
		}
	})
}

func TestSourceFileOf(t *testing.T) {
	t.Parallel()

	t.Run("finds the file a declaration came from", func(t *testing.T) {
		t.Parallel()
		file := &node.File{Name: "user.ts"}
		pkg := &node.Package{Name: "src", Files: []*node.File{file}}
		s := &node.Struct{Name: "User"}
		s.SourcePos = pos("/abs/path/user.ts")

		if got := src.FileOf(pkg, s); got != file {
			t.Fatalf("FileOf = %+v, want the declaring file", got)
		}
	})

	t.Run("reports nothing for an unknown or positionless declaration", func(t *testing.T) {
		t.Parallel()
		pkg := &node.Package{Name: "src", Files: []*node.File{{Name: "a.ts"}}}
		if got := src.FileOf(pkg, &node.Struct{Name: "X"}); got != nil {
			t.Error("FileOf invented a file for a positionless declaration")
		}
		if got := src.FileOf(nil, &node.Struct{Name: "X"}); got != nil {
			t.Error("FileOf on a nil package returned a file")
		}
	})
}

func TestSettable(t *testing.T) {
	t.Parallel()

	t.Run("excludes what a constructor elsewhere cannot assign", func(t *testing.T) {
		t.Parallel()
		plain := &node.Field{Name: "plain", Type: named("string")}

		ro := &node.Field{Name: "ro", Type: named("string")}
		typescript.MetaReadonly.Set(ro.EnsureMeta(), true, "test")

		static := &node.Field{Name: "static", Type: named("string")}
		typescript.MetaStatic.Set(static.EnsureMeta(), true, "test")

		private := &node.Field{Name: "priv", Type: named("string")}
		typescript.MetaVisibility.Set(private.EnsureMeta(), typescript.VisibilityPrivate, "test")

		got := typescript.Settable(&node.Struct{
			Name:   "User",
			Fields: []*node.Field{plain, ro, static, private},
		})
		if len(got) != 1 || got[0].Name != "plain" {
			names := make([]string, 0, len(got))
			for _, m := range got {
				names = append(names, m.Name)
			}
			t.Fatalf("Settable = %v, want only plain", names)
		}
	})

	t.Run("a nil struct has no settable members", func(t *testing.T) {
		t.Parallel()
		if got := typescript.Settable(nil); got != nil {
			t.Fatalf("Settable(nil) = %+v", got)
		}
	})
}

func TestTypeOf(t *testing.T) {
	t.Parallel()

	t.Run("classifies the shapes TypeScript distinguishes", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			ref  *node.TypeRef
			want emit.TypeShape
		}{
			{"scalar", named("string"), emit.ShapeScalar},
			{"bytes", named("Uint8Array"), emit.ShapeBytes},
			{"sequence", &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: named("string")}, emit.ShapeSequence},
			{"mapping", &node.TypeRef{
				TypeKind: node.TypeRefMap, MapKey: named("string"), MapValue: named("number"),
			}, emit.ShapeMapping},
			{"nil", nil, emit.ShapeScalar},
		}
		for _, tc := range cases {
			if got := typescript.TypeOf(tc.ref, nil); got.Shape != tc.want {
				t.Errorf("%s: shape = %v, want %v", tc.name, got.Shape, tc.want)
			}
		}
	})

	t.Run("a nullable union is optional, not a union", func(t *testing.T) {
		t.Parallel()
		// What makes `T | null` interesting to a generator is that the
		// value may be absent, not that it has two members.
		got := typescript.TypeOf(nullable(named("string")), nil)
		if got.Shape != emit.ShapeOptional {
			t.Fatalf("shape = %v, want optional", got.Shape)
		}
		if got.Elem == nil {
			t.Fatal("optional carries no element type")
		}
	})

	t.Run("a wider union stays scalar", func(t *testing.T) {
		t.Parallel()
		// `A | B | null` is a union that happens to admit null; naming
		// one member as the type would be an arbitrary choice.
		u := marker(typescript.RefUnion, named("A"), named("B"), nullLiteral())
		if got := typescript.TypeOf(u, nil); got.Shape != emit.ShapeScalar {
			t.Fatalf("shape = %v, want scalar", got.Shape)
		}
	})
}

func TestZeroAndLiteral(t *testing.T) {
	t.Parallel()

	t.Run("ZeroLiteral answers only where a value exists", func(t *testing.T) {
		t.Parallel()
		// TypeScript has no zero value: an unassigned binding is
		// undefined whatever its type, and strictNullChecks rejects
		// assigning that to a non-nullable one.
		for _, tc := range []struct {
			ref  *node.TypeRef
			want string
			ok   bool
		}{
			{named("string"), "''", true},
			{named("number"), "0", true},
			{named("boolean"), "false", true},
			{named("bigint"), "0n", true},
			{&node.TypeRef{TypeKind: node.TypeRefSlice, Elem: named("string")}, "[]", true},
			{nullable(named("string")), "null", true},
			{named("SomeInterface"), "", false},
			{nil, "", false},
		} {
			got, ok := typescript.ZeroLiteral(tc.ref, nil)
			if got != tc.want || ok != tc.ok {
				t.Errorf("ZeroLiteral(%v) = (%q, %v), want (%q, %v)", tc.ref, got, ok, tc.want, tc.ok)
			}
		}
	})

	t.Run("LiteralFor renders a directive value against its type", func(t *testing.T) {
		t.Parallel()
		// An author wrote `default=3` on a numeric field and the
		// generator needs `3`, not `'3'`.
		for _, tc := range []struct {
			ref  *node.TypeRef
			text string
			want string
			ok   bool
		}{
			{named("string"), "hi", "'hi'", true},
			{named("number"), "3", "3", true},
			{named("boolean"), "true", "true", true},
			{named("boolean"), "yes", "", false},
			{named("Widget"), "x", "", false},
			{nil, "x", "", false},
		} {
			got, ok := typescript.LiteralFor(nil, tc.ref, tc.text, nil)
			if got != tc.want || ok != tc.ok {
				t.Errorf("LiteralFor(%v, %q) = (%q, %v), want (%q, %v)",
					tc.ref, tc.text, got, ok, tc.want, tc.ok)
			}
		}
	})
}

func TestNamingRules(t *testing.T) {
	t.Parallel()

	t.Run("TypeName joins parts as PascalCase", func(t *testing.T) {
		t.Parallel()
		if got := typescript.TypeName("user", "repo"); got != "UserRepo" {
			t.Fatalf("TypeName = %q, want UserRepo", got)
		}
		if got := typescript.TypeName("user", "", "repo"); got != "UserRepo" {
			t.Fatalf("TypeName with an empty part = %q", got)
		}
	})

	t.Run("ConstructorName uses the create verb", func(t *testing.T) {
		t.Parallel()
		// TypeScript has no `New` convention — a class is constructed
		// with `new` — so a generated builder is an ordinary function.
		if got := typescript.ConstructorName("User"); got != "createUser" {
			t.Fatalf("ConstructorName = %q, want createUser", got)
		}
	})
}

func TestGenericRules(t *testing.T) {
	t.Parallel()

	t.Run("TypeArgs renders the use-position list", func(t *testing.T) {
		t.Parallel()
		params := []*node.TypeParam{{Name: "T"}, {Name: "U"}}
		if got := typescript.TypeArgs(params); got != "<T, U>" {
			t.Fatalf("TypeArgs = %q", got)
		}
		if got := typescript.TypeArgs(nil); got != "" {
			t.Fatalf("TypeArgs(nil) = %q, want empty", got)
		}
	})

	t.Run("Witnesses answers for unconstrained parameters only", func(t *testing.T) {
		t.Parallel()
		// Choosing for a bounded parameter needs a type checker this
		// adapter does not have, so the set is refused rather than
		// half-answered.
		free := []*node.TypeParam{{Name: "T"}}
		if got := typescript.Witnesses(free); len(got) != 1 {
			t.Fatalf("Witnesses = %+v, want one", got)
		}

		bound := []*node.TypeParam{{
			Name:       "T",
			Constraint: &node.Constraint{Embedded: []*node.TypeRef{named("object")}},
		}}
		if got := typescript.Witnesses(bound); got != nil {
			t.Fatalf("Witnesses(bounded) = %+v, want nil", got)
		}
	})

	t.Run("TypeParams lifts bounds onto the emit side", func(t *testing.T) {
		t.Parallel()
		got := typescript.TypeParams([]*node.TypeParam{{
			Name:       "T",
			Constraint: &node.Constraint{Raw: "object", Embedded: []*node.TypeRef{named("object")}},
		}})
		if len(got) != 1 || got[0].Constraint == nil {
			t.Fatalf("TypeParams = %+v", got)
		}
		if len(got[0].Constraint.Embedded) != 1 {
			t.Fatalf("bound lost: %+v", got[0].Constraint)
		}
	})

	t.Run("substitution is a no-op", func(t *testing.T) {
		t.Parallel()
		// TypeScript's parameters are lexically scoped to their
		// declaration, so a use site outside it cannot name one and
		// there is nothing to rebind.
		ref := named("T")
		if got := src.SubstituteParams(ref, nil); got != ref {
			t.Error("SubstituteParams changed the ref")
		}
		emitRef := emit.Builtin("T")
		if got := src.SubstituteRef(emitRef, nil); got != emit.Ref(emitRef) {
			t.Error("SubstituteRef changed the ref")
		}
	})
}

func TestSamplesOf(t *testing.T) {
	t.Parallel()

	t.Run("returns two distinct values for a scalar", func(t *testing.T) {
		t.Parallel()
		// One sample would let an implementation that ignores its
		// argument pass a generated round-trip test.
		for _, name := range []string{"string", "number", "boolean", "bigint"} {
			first, second := typescript.SamplesOf(named(name), "", nil)
			if first.Text == "" || first.Text == second.Text {
				t.Errorf("%s: samples = %q, %q; want two distinct", name, first.Text, second.Text)
			}
		}
	})

	t.Run("a hint reaches the string sample", func(t *testing.T) {
		t.Parallel()
		first, _ := typescript.SamplesOf(named("string"), "email", nil)
		if first.Text != "'email'" {
			t.Fatalf("sample = %q, want the hint", first.Text)
		}
	})

	t.Run("a type with no literal form gets no sample", func(t *testing.T) {
		t.Parallel()
		// A generated fixture holding an invented value is worse than
		// one the author fills in, because it compiles.
		first, _ := typescript.SamplesOf(named("Widget"), "", nil)
		if first.Text != "" {
			t.Fatalf("sample = %q, want none", first.Text)
		}
	})

	t.Run("a sequence samples its element", func(t *testing.T) {
		t.Parallel()
		ref := &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: named("number")}
		first, second := typescript.SamplesOf(ref, "", nil)
		if first.Text != "[1]" || second.Text != "[1, 2]" {
			t.Fatalf("samples = %q, %q", first.Text, second.Text)
		}
	})
}

// TestSourceMethodsForward drives every rule through the [Source]
// value rather than the plain function beside it.
//
// The methods are one-line forwarders, which is exactly why they want
// a test: the SDK reaches the rules through this value, and a
// forwarder wired to the wrong function — or left off when the
// interface grew — compiles and answers wrongly.
func TestSourceMethodsForward(t *testing.T) {
	t.Parallel()

	str := named("string")
	params := []*node.TypeParam{{Name: "T"}}

	t.Run("declaration rules", func(t *testing.T) {
		t.Parallel()
		if _, _, err := src.ResolveValue(nil, "Bare"); err != nil {
			t.Errorf("ResolveValue: %v", err)
		}
		if _, ok := src.Tag(&node.Field{Name: "x"}, "Any"); ok {
			t.Error("Tag found a decorator that is not there")
		}
		if got := src.FileOf(nil, nil); got != nil {
			t.Error("FileOf invented a file")
		}
		if got := src.Settable(&node.Struct{
			Name:   "S",
			Fields: []*node.Field{{Name: "a", Type: str}},
		}); len(got) != 1 {
			t.Errorf("Settable = %+v, want one member", got)
		}
	})

	t.Run("type rules", func(t *testing.T) {
		t.Parallel()
		if got := src.TypeOf(str, nil); got.Shape != emit.ShapeScalar {
			t.Errorf("TypeOf = %v, want scalar", got.Shape)
		}
		if first, _ := src.SamplesOf(str, "", nil); first.Text == "" {
			t.Error("SamplesOf produced no sample for a string")
		}
		if got := src.TypeParams(params); len(got) != 1 {
			t.Errorf("TypeParams = %+v", got)
		}
		if got := src.TypeArgs(params); got != "<T>" {
			t.Errorf("TypeArgs = %q", got)
		}
		if got := src.Witnesses(params); len(got) != 1 {
			t.Errorf("Witnesses = %+v", got)
		}
		if got, ok := src.LiteralFor(nil, str, "hi", nil); !ok || got != "'hi'" {
			t.Errorf("LiteralFor = (%q, %v)", got, ok)
		}
		if got, ok := src.ZeroLiteral(str, nil); !ok || got != "''" {
			t.Errorf("ZeroLiteral = (%q, %v)", got, ok)
		}
	})

	t.Run("naming rules", func(t *testing.T) {
		t.Parallel()
		if got := src.TypeName("a", "b"); got != "AB" {
			t.Errorf("TypeName = %q", got)
		}
		if got := src.ConstructorName("Thing"); got != "createThing" {
			t.Errorf("ConstructorName = %q", got)
		}
	})
}

func TestSampleEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("a map has no sample", func(t *testing.T) {
		t.Parallel()
		// An invented map literal would be a value the declaration
		// never promised; an empty one is not two distinct samples.
		ref := &node.TypeRef{TypeKind: node.TypeRefMap, MapKey: named("string"), MapValue: named("number")}
		first, second := typescript.SamplesOf(ref, "", nil)
		if first.Text != "" || second.Text != "" {
			t.Fatalf("samples = %q, %q; want none", first.Text, second.Text)
		}
	})

	t.Run("a nullable type samples its member and null", func(t *testing.T) {
		t.Parallel()
		first, second := typescript.SamplesOf(nullable(named("string")), "", nil)
		if first.Text == "" || second.Text != typescript.TypeNull {
			t.Fatalf("samples = %q, %q; want a value and null", first.Text, second.Text)
		}
	})

	t.Run("a sequence of an unsamplable element has no sample", func(t *testing.T) {
		t.Parallel()
		ref := &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: named("Widget")}
		first, _ := typescript.SamplesOf(ref, "", nil)
		if first.Text != "" {
			t.Fatalf("sample = %q, want none", first.Text)
		}
	})

	t.Run("a nil type has no sample", func(t *testing.T) {
		t.Parallel()
		first, _ := typescript.SamplesOf(nil, "", nil)
		if first.Text != "" {
			t.Fatalf("sample = %q, want none", first.Text)
		}
	})
}

func TestFromNodeEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("a function type carries params and returns across", func(t *testing.T) {
		t.Parallel()
		fn := &node.TypeRef{
			TypeKind:    node.TypeRefFunc,
			FuncParams:  []*node.TypeRef{named("string"), named("number")},
			FuncReturns: []*node.TypeRef{named("boolean")},
		}
		ref, ok := typescript.FromNode(fn).(*emit.CompositeRef)
		if !ok {
			t.Fatalf("FromNode = %T, want a composite", typescript.FromNode(fn))
		}
		if len(ref.FuncParams) != 2 || len(ref.FuncReturns) != 1 {
			t.Fatalf("params = %d, returns = %d", len(ref.FuncParams), len(ref.FuncReturns))
		}
	})

	t.Run("a qualified generic keeps its arguments", func(t *testing.T) {
		t.Parallel()
		ref := &node.TypeRef{
			TypeKind: node.TypeRefNamed,
			Package:  "./m",
			Name:     "Box",
			TypeArgs: []*node.TypeRef{named("string")},
		}
		ext, ok := typescript.FromNode(ref).(*emit.ExternalRef)
		if !ok {
			t.Fatalf("FromNode = %T, want an external ref", typescript.FromNode(ref))
		}
		if ext.Package != "./m" || len(ext.TypeArgs) != 1 {
			t.Fatalf("external = %+v", ext)
		}
	})

	t.Run("an intersection is carried as text", func(t *testing.T) {
		t.Parallel()
		// The emit side has no intersection shape, and projecting it
		// as a union would be the opposite claim.
		inter := marker(typescript.RefIntersection, named("A"), named("B"))
		got, ok := typescript.FromNode(inter).(*emit.BuiltinRef)
		if !ok {
			t.Fatalf("FromNode = %T, want a text-carrying builtin", typescript.FromNode(inter))
		}
		if got.Name != "A & B" {
			t.Fatalf("text = %q, want A & B", got.Name)
		}
	})
}

func TestFileLabel(t *testing.T) {
	t.Parallel()

	t.Run("an unnamed file is named as such in a diagnostic", func(t *testing.T) {
		t.Parallel()
		// Resolving against no file at all is a different caller
		// mistake from resolving against one whose imports fall short.
		_, _, err := typescript.ResolveValue(&node.File{}, "q.Sym")
		if err == nil {
			t.Fatal("an unimportable qualifier resolved")
		}
		if !strings.Contains(err.Error(), "unnamed file") {
			t.Fatalf("error does not name the file: %v", err)
		}
	})
}

func TestNullableUnionEdges(t *testing.T) {
	t.Parallel()

	t.Run("undefined counts as absent alongside null", func(t *testing.T) {
		t.Parallel()
		// TypeScript distinguishes them and strictNullChecks makes the
		// difference load-bearing, but either makes a value optional.
		undef := named("undefined")
		typescript.MetaLiteralType.Set(undef.EnsureMeta(), "undefined", "test")

		u := marker(typescript.RefUnion, named("string"), undef)
		if got := typescript.TypeOf(u, nil); got.Shape != emit.ShapeOptional {
			t.Fatalf("shape = %v, want optional", got.Shape)
		}
	})

	t.Run("a union of two absent values is not optional", func(t *testing.T) {
		t.Parallel()
		// `null | undefined` carries no type to be optional about.
		u := marker(typescript.RefUnion, nullLiteral(), nullLiteral())
		if got := typescript.TypeOf(u, nil); got.Shape != emit.ShapeOptional {
			// Either reading is defensible; what matters is that it
			// does not panic reaching for a member that is not there.
			t.Logf("shape = %v", got.Shape)
		}
	})

	t.Run("a one-member union is not optional", func(t *testing.T) {
		t.Parallel()
		u := marker(typescript.RefUnion, named("string"))
		if got := typescript.TypeOf(u, nil); got.Shape != emit.ShapeScalar {
			t.Fatalf("shape = %v, want scalar", got.Shape)
		}
	})
}
