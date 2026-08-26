// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/lang/typescript/typescripttest/tsfixture"
	"go.thesmos.sh/eidos/node"
)

func TestTypeConstructors(t *testing.T) {
	t.Parallel()

	t.Run("a named type carries its module or does not", func(t *testing.T) {
		t.Parallel()
		// `./models/person` and `models/person` resolve differently, so
		// neither is normalised.
		local := tsfixture.Named("string")
		if local.TypeKind != node.TypeRefNamed || local.Package != "" {
			t.Errorf("local = %+v", local)
		}
		imported := tsfixture.ModNamed("./models/person", "Person")
		if imported.Package != "./models/person" || imported.Name != "Person" {
			t.Errorf("imported = %+v", imported)
		}
	})

	t.Run("a generic parameter is told apart from a named type", func(t *testing.T) {
		t.Parallel()
		// The two are indistinguishable in the model and not in
		// meaning: a consumer substituting arguments has to know which
		// names are bound.
		if got := tsfixture.TypeParamRef("T"); got.TypeKind != node.TypeRefTypeParam {
			t.Fatalf("kind = %v, want a type parameter", got.TypeKind)
		}
	})

	t.Run("WithArgs leaves its base alone", func(t *testing.T) {
		t.Parallel()
		// So a fixture may apply different arguments to one base.
		base := tsfixture.Named("Box")
		first := tsfixture.WithArgs(base, tsfixture.Named("string"))
		second := tsfixture.WithArgs(base, tsfixture.Named("number"))

		if len(base.TypeArgs) != 0 {
			t.Fatalf("the base gained %d arguments", len(base.TypeArgs))
		}
		if first.TypeArgs[0].Name == second.TypeArgs[0].Name {
			t.Fatal("two applications share one argument list")
		}
		if tsfixture.WithArgs(nil) != nil {
			t.Error("WithArgs(nil) built a reference")
		}
	})

	t.Run("the container shapes map onto the model", func(t *testing.T) {
		t.Parallel()
		for name, tc := range map[string]struct {
			ref  *node.TypeRef
			want node.TypeRefKind
		}{
			"array":  {tsfixture.Array(tsfixture.Named("string")), node.TypeRefSlice},
			"record": {tsfixture.Record(tsfixture.Named("string"), tsfixture.Named("number")), node.TypeRefMap},
			"func":   {tsfixture.Func(nil, nil), node.TypeRefFunc},
			"object": {tsfixture.Object(), node.TypeRefAnonStruct},
		} {
			if tc.ref.TypeKind != tc.want {
				t.Errorf("%s kind = %v, want %v", name, tc.ref.TypeKind, tc.want)
			}
		}
	})

	t.Run("the structural shapes build the markers the frontend does", func(t *testing.T) {
		t.Parallel()
		// A fixture and a frontend produce a shape nothing downstream
		// can tell apart, which is the whole point of building the
		// marker rather than a metadata flag.
		u := tsfixture.Union(tsfixture.Named("A"), tsfixture.Named("B"))
		if !typescript.IsUnion(u) {
			t.Error("Union did not build a union")
		}
		if got := typescript.Members(u); len(got) != 2 {
			t.Errorf("union members = %d, want 2", len(got))
		}
		if !typescript.IsIntersection(tsfixture.Intersection(tsfixture.Named("A"))) {
			t.Error("Intersection did not build one")
		}
		if !typescript.IsTuple(tsfixture.Tuple(tsfixture.Named("A"))) {
			t.Error("Tuple did not build one")
		}
	})

	t.Run("an operator carries its source text", func(t *testing.T) {
		t.Parallel()
		op := tsfixture.Operator("keyof T")
		if !typescript.IsOperator(op) {
			t.Fatal("Operator did not build one")
		}
		if text, _ := typescript.MetaTypeText.Get(op.Meta()); text != "keyof T" {
			t.Fatalf("text = %q", text)
		}
	})

	t.Run("a literal type is its own text", func(t *testing.T) {
		t.Parallel()
		lit := tsfixture.Literal("'admin'")
		if got, _ := typescript.MetaLiteralType.Get(lit.Meta()); got != "'admin'" {
			t.Fatalf("literal = %q", got)
		}
	})

	t.Run("the two absent values stay distinct", func(t *testing.T) {
		t.Parallel()
		// strictNullChecks makes the difference load-bearing, so a
		// fixture that could only spell one could not reproduce the bug
		// where a generator conflates them.
		null := typescript.Members(tsfixture.Nullable(tsfixture.Named("string")))
		undef := typescript.Members(tsfixture.Undefinable(tsfixture.Named("string")))
		if null[1].Name == undef[1].Name {
			t.Fatalf("both spelled %q", null[1].Name)
		}
		if null[1].Name != typescript.TypeNull {
			t.Errorf("Nullable member = %q", null[1].Name)
		}
		if undef[1].Name != typescript.TypeUndefined {
			t.Errorf("Undefinable member = %q", undef[1].Name)
		}
	})

	t.Run("an inline object holds its members", func(t *testing.T) {
		t.Parallel()
		obj := tsfixture.Object(
			tsfixture.Prop("a", tsfixture.Named("string")),
			tsfixture.Prop("b", tsfixture.Named("number")),
		)
		if len(obj.Fields) != 2 || obj.Fields[0].Name != "a" {
			t.Fatalf("object = %+v", obj.Fields)
		}
	})

	t.Run("a constraint holds its bounds and none is nil", func(t *testing.T) {
		t.Parallel()
		c := tsfixture.Constraint(tsfixture.Named("A"), tsfixture.Named("B"))
		if len(c.Embedded) != 2 {
			t.Fatalf("bounds = %d, want 2", len(c.Embedded))
		}
		if tsfixture.Constraint() != nil {
			t.Error("an empty bound list built a constraint")
		}
	})
}

func TestConstraintForms(t *testing.T) {
	t.Parallel()

	t.Run("a bound with no structure carries its text", func(t *testing.T) {
		t.Parallel()
		// `T extends keyof U` is a bound nothing can reconstruct from
		// references, so the source text is the only faithful record.
		c := tsfixture.Bound("keyof U")
		if c.Raw != "keyof U" || len(c.Embedded) != 0 {
			t.Fatalf("Bound = %+v", c)
		}
	})

	t.Run("a bound carries its references alongside the text", func(t *testing.T) {
		t.Parallel()
		c := tsfixture.Bound("A & B", tsfixture.Named("A"), tsfixture.Named("B"))
		if c.Raw != "A & B" || len(c.Embedded) != 2 {
			t.Fatalf("Bound = %+v", c)
		}
	})

	t.Run("the empty object type is not the empty object", func(t *testing.T) {
		t.Parallel()
		// `{}` accepts a string and `object` does not, so a generator
		// emitting one where it meant the other widened a contract
		// without changing a name.
		if got := tsfixture.AnonObject(); got.TypeKind != node.TypeRefAnonInterface {
			t.Fatalf("AnonObject kind = %v", got.TypeKind)
		}
		if got := tsfixture.Object(); got.TypeKind != node.TypeRefAnonStruct {
			t.Fatalf("Object kind = %v", got.TypeKind)
		}
	})
}
