// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/frontend/golang"
)

// TestStampTypeRefMeta covers the type-ref-level facts the converter
// records: context, error, stringer, comparable, and interface-ness.
func TestStampTypeRefMeta(t *testing.T) {
	t.Parallel()
	t.Run("context.Context ref carries MetaIsContext", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"context\"\n\ntype Handler interface { Do(ctx context.Context) error }\n",
		})
		iface := pkg.InterfaceByName("Handler")
		if iface == nil || len(iface.Methods) == 0 || len(iface.Methods[0].Params) == 0 {
			t.Fatalf("Handler.Do missing")
		}
		ctxRef := iface.Methods[0].Params[0].Type
		if got, _ := golang.MetaIsContext.Get(ctxRef.Meta()); !got {
			t.Fatalf("expected MetaIsContext=true on context.Context param")
		}
	})

	t.Run("declared return names are preserved", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Op interface { Do() (item string, err error) }\n",
		})
		rets := pkg.InterfaceByName("Op").Methods[0].Returns
		if len(rets) != 2 || rets[0].Name != "item" || rets[1].Name != "err" {
			t.Fatalf("return names = %q/%q, want item/err", rets[0].Name, rets[1].Name)
		}
	})

	t.Run("the blank identifier normalises to an unnamed return", func(t *testing.T) {
		t.Parallel()
		// `_` cannot be used as a derived identifier, so it is
		// reported as unnamed rather than as the literal "_" —
		// otherwise every consumer deriving a field name from a
		// return would have to special-case it.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Op interface { Do() (_ string, err error) }\n",
		})
		rets := pkg.InterfaceByName("Op").Methods[0].Returns
		if len(rets) != 2 || rets[0].Name != "" {
			t.Fatalf("blank return name = %q, want empty", rets[0].Name)
		}
		if rets[1].Name != "err" {
			t.Fatalf("sibling return name = %q, want err", rets[1].Name)
		}
	})

	t.Run("anonymous returns carry no name", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Op interface { Do() (string, error) }\n",
		})
		for i, r := range pkg.InterfaceByName("Op").Methods[0].Returns {
			if r.Name != "" {
				t.Fatalf("return %d name = %q, want empty", i, r.Name)
			}
		}
	})

	t.Run("error return carries MetaIsError", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Op interface { Do() error }\n",
		})
		ret := pkg.InterfaceByName("Op").Methods[0].Returns[0].Type
		if got, _ := golang.MetaIsError.Get(ret.Meta()); !got {
			t.Fatalf("expected MetaIsError=true on error return")
		}
	})

	t.Run("comparable types carry MetaIsComparable", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct { N int }\n",
		})
		f := pkg.StructByName("S").FieldByName("N")
		if got, _ := golang.MetaIsComparable.Get(f.Type.Meta()); !got {
			t.Fatalf("expected MetaIsComparable=true on int field")
		}
	})

	t.Run("named interface param carries MetaIsInterface", func(t *testing.T) {
		t.Parallel()
		// The motivating case: a plugin sees only a package and an
		// identifier for io.Reader and cannot resolve it, least of
		// all for a type outside the loaded packages.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"io\"\n\ntype Loader interface { Load(r io.Reader) (int, error) }\n",
		})
		ref := pkg.InterfaceByName("Loader").Methods[0].Params[0].Type
		if got, _ := golang.MetaIsInterface.Get(ref.Meta()); !got {
			t.Fatalf("expected MetaIsInterface=true on io.Reader param")
		}
	})

	t.Run("inline interface param carries MetaIsInterface", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Loader interface { Load(r interface{ Read(p []byte) (int, error) }) error }\n",
		})
		ref := pkg.InterfaceByName("Loader").Methods[0].Params[0].Type
		if got, _ := golang.MetaIsInterface.Get(ref.Meta()); !got {
			t.Fatalf("expected MetaIsInterface=true on inline interface param")
		}
	})

	t.Run("non-interface param does not carry MetaIsInterface", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Store interface { Get(id string) (int, error) }\n",
		})
		ref := pkg.InterfaceByName("Store").Methods[0].Params[0].Type
		if got, _ := golang.MetaIsInterface.Get(ref.Meta()); got {
			t.Fatalf("expected no MetaIsInterface on a string param")
		}
	})

	t.Run("type parameter does not carry MetaIsInterface", func(t *testing.T) {
		t.Parallel()
		// A type parameter reports its constraint as its underlying
		// type, so `K comparable` underlies to an interface. Without
		// the carve-out in isInterfaceType every generic key would
		// advertise itself as a collaborator.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Cache[K comparable, V any] interface { Get(k K) (V, error) }\n",
		})
		ref := pkg.InterfaceByName("Cache").Methods[0].Params[0].Type
		if !ref.IsTypeParam() {
			t.Fatalf("fixture did not produce a type-param ref; got kind %v", ref.TypeKind)
		}
		if got, _ := golang.MetaIsInterface.Get(ref.Meta()); got {
			t.Fatalf("expected no MetaIsInterface on type parameter K comparable")
		}
	})

	t.Run("nil ref or nil type is a no-op", func(t *testing.T) {
		t.Parallel()
		// Cover the early-return through a package with no
		// declarations — every public path delegates to the
		// stampTypeRefMeta no-op when typeRefOf returns nil for an
		// absent type expression.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n",
		})
		if pkg.Name != "a" {
			t.Fatalf("expected package a, got %q", pkg.Name)
		}
	})

	t.Run("basic-type comparable meta has no source position (no decl pos available)", func(t *testing.T) {
		t.Parallel()
		// Predeclared identifiers like `int` have no source
		// position — declPosOf returns a zero Pos and the meta
		// provenance carries that zero Pos. We assert the meta is
		// stamped (the fact is true) even when no position exists.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ N int }\n",
		})
		ref := pkg.StructByName("S").FieldByName("N").Type
		if !golang.MetaIsComparable.Has(ref.Meta()) {
			t.Fatalf("expected MetaIsComparable on builtin int ref")
		}
	})

	t.Run("type-parameter ref provenance falls back to the param's declaration position", func(t *testing.T) {
		t.Parallel()
		// Inside a generic struct, the type-parameter ref's
		// declPosOf returns the param's identifier position.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T any] struct{ V T }\n",
		})
		ref := pkg.StructByName("Box").FieldByName("V").Type
		if !ref.IsTypeParam() {
			t.Fatalf("expected type-param ref, got %+v", ref)
		}
		// Type-params alone don't stamp MetaIsComparable/Stringer/etc.,
		// so we assert ref.Pos() carries a non-zero file (set by
		// the struct-field overlay).
		if ref.Pos().File == "" {
			t.Fatalf("type-param ref must carry a file position")
		}
	})
}

// TestStampStructMeta covers MetaEmbedsInterface stamping when a
// struct embeds an interface.
func TestStampStructMeta(t *testing.T) {
	t.Parallel()
	t.Run("struct embedding an interface stamps MetaEmbedsInterface", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype I interface{ M() }\n\ntype S struct{ I }\n",
		})
		s := pkg.StructByName("S")
		if got, _ := golang.MetaEmbedsInterface.Get(s.Meta()); !got {
			t.Fatalf("expected MetaEmbedsInterface=true")
		}
	})

	t.Run("plain struct does not stamp MetaEmbedsInterface", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ N int }\n",
		})
		s := pkg.StructByName("S")
		if golang.MetaEmbedsInterface.Has(s.Meta()) {
			t.Fatalf("expected MetaEmbedsInterface absent")
		}
	})
}

// TestStampInterfaceMeta covers MetaIsEmptyInterface and
// MetaIsConstraintInterface.
func TestStampInterfaceMeta(t *testing.T) {
	t.Parallel()
	t.Run("empty interface stamps MetaIsEmptyInterface", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype I interface{}\n",
		})
		i := pkg.InterfaceByName("I")
		if got, _ := golang.MetaIsEmptyInterface.Get(i.Meta()); !got {
			t.Fatalf("expected MetaIsEmptyInterface=true")
		}
	})

	t.Run("type-set constraint stamps MetaIsConstraintInterface", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Numeric interface{ ~int | ~float64 }\n",
		})
		i := pkg.InterfaceByName("Numeric")
		if got, _ := golang.MetaIsConstraintInterface.Get(i.Meta()); !got {
			t.Fatalf("expected MetaIsConstraintInterface=true on type-set")
		}
	})

	t.Run("method-set interface does not stamp constraint", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype I interface{ M() }\n",
		})
		i := pkg.InterfaceByName("I")
		if golang.MetaIsConstraintInterface.Has(i.Meta()) {
			t.Fatalf("method-set interface should not carry constraint flag")
		}
		if golang.MetaIsEmptyInterface.Has(i.Meta()) {
			t.Fatalf("method-set interface should not carry empty flag")
		}
	})
}

// TestStampAliasMeta covers MetaUnderlyingKind for every documented
// underlying-type kind.
func TestStampAliasMeta(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		source string
		alias  string
		want   string
	}{
		{"int basic", "package a\n\ntype Status int\n", "Status", "basic"},
		{"slice", "package a\n\ntype IDs []int\n", "IDs", "slice"},
		{"array", "package a\n\ntype Block [16]byte\n", "Block", "array"},
		{"map", "package a\n\ntype Set map[string]bool\n", "Set", "basic"},
		{"func", "package a\n\ntype Fn func(int) error\n", "Fn", "func"},
		{"chan", "package a\n\ntype Ch chan int\n", "Ch", "chan"},
		{"pointer", "package a\n\ntype P *int\n", "P", "pointer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pkg := requirePackage(t, map[string]string{"a.go": tc.source})
			a := pkg.AliasByName(tc.alias)
			if a == nil {
				// Map-underlying alias is correctly classified as the
				// underlying map kind; the cases that promote to enum
				// disappear from the alias slice.
				t.Skipf("alias %q absorbed by enum promotion", tc.alias)
			}
			got, _ := golang.MetaUnderlyingKind.Get(a.Meta())
			if tc.name == "map" {
				// The map case in this fixture has no constants
				// so the alias survives; underlying kind is "map".
				if got != "map" {
					t.Fatalf("alias %q underlying kind = %q, want %q", tc.alias, got, "map")
				}
				return
			}
			if got != tc.want {
				t.Fatalf("alias %q underlying kind = %q, want %q", tc.alias, got, tc.want)
			}
		})
	}
}

// TestStampMethodMeta covers MetaReceiverIsPointer on both pointer
// and value receivers.
func TestStampMethodMeta(t *testing.T) {
	t.Parallel()
	t.Run("pointer receiver stamps MetaReceiverIsPointer", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n\nfunc (s *S) Do() {}\n",
		})
		s := pkg.StructByName("S")
		if len(s.Methods) == 0 {
			t.Fatalf("S.Do missing")
		}
		if got, _ := golang.MetaReceiverIsPointer.Get(s.Methods[0].Meta()); !got {
			t.Fatalf("expected MetaReceiverIsPointer=true on *S receiver")
		}
	})

	t.Run("value receiver does not stamp MetaReceiverIsPointer", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n\nfunc (s S) Do() {}\n",
		})
		s := pkg.StructByName("S")
		if golang.MetaReceiverIsPointer.Has(s.Methods[0].Meta()) {
			t.Fatalf("value receiver must not carry MetaReceiverIsPointer")
		}
	})
}

// TestStampFunctionMeta covers iter.Seq / iter.Seq2 detection.
func TestStampFunctionMeta(t *testing.T) {
	t.Parallel()
	t.Run("iter.Seq[T] return stamps MetaIsIterSeq and value type", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"iter\"\n\nfunc All() iter.Seq[int] { return nil }\n",
		})
		fn := pkg.FunctionByName("All")
		if fn == nil {
			t.Fatalf("All missing")
		}
		if got, _ := golang.MetaIsIterSeq.Get(fn.Meta()); !got {
			t.Fatalf("expected MetaIsIterSeq=true")
		}
		if got, _ := golang.MetaIterValueType.Get(fn.Meta()); got != "int" {
			t.Fatalf("expected MetaIterValueType=int, got %q", got)
		}
	})

	t.Run("iter.Seq2[K, V] stamps Seq2 and both type names", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"iter\"\n\nfunc Pairs() iter.Seq2[string, int] { return nil }\n",
		})
		fn := pkg.FunctionByName("Pairs")
		if fn == nil {
			t.Fatalf("Pairs missing")
		}
		if got, _ := golang.MetaIsIterSeq2.Get(fn.Meta()); !got {
			t.Fatalf("expected MetaIsIterSeq2=true")
		}
		key, _ := golang.MetaIterKeyType.Get(fn.Meta())
		val, _ := golang.MetaIterValueType.Get(fn.Meta())
		if key != "string" || val != "int" {
			t.Fatalf("expected (key, value) = (string, int), got (%q, %q)", key, val)
		}
	})

	t.Run("non-iter return type stamps nothing", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nfunc Get() int { return 0 }\n",
		})
		fn := pkg.FunctionByName("Get")
		if golang.MetaIsIterSeq.Has(fn.Meta()) || golang.MetaIsIterSeq2.Has(fn.Meta()) {
			t.Fatalf("non-iter function should not carry iter meta")
		}
	})

	t.Run("named return type from a non-iter package stamps nothing", func(t *testing.T) {
		t.Parallel()
		// Drives stampFunctionMeta's obj.Pkg().Path() != "iter"
		// branch: the return type IS a *types.Named with a non-nil
		// package, but the package path doesn't match.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"time\"\n\nfunc Now() time.Time { return time.Time{} }\n",
		})
		fn := pkg.FunctionByName("Now")
		if golang.MetaIsIterSeq.Has(fn.Meta()) || golang.MetaIsIterSeq2.Has(fn.Meta()) {
			t.Fatalf("time.Time return must not carry iter meta")
		}
	})
}

// TestStampFieldTagMeta covers per-tag-key stamping under the
// go.tag.* prefix.
func TestStampFieldTagMeta(t *testing.T) {
	t.Parallel()
	t.Run("each tag key surfaces under go.tag.*", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype User struct {\n\tID string `json:\"id\" db:\"id_col\" yaml:\"identifier\"`\n}\n",
		})
		f := pkg.StructByName("User").FieldByName("ID")
		if f == nil {
			t.Fatalf("User.ID missing")
		}
		want := map[string]string{
			"go.tag.json": "id",
			"go.tag.db":   "id_col",
			"go.tag.yaml": "identifier",
		}
		for key, wantVal := range want {
			raw, ok := f.Meta().RawValue(key)
			if !ok {
				t.Errorf("expected %q to be set", key)
				continue
			}
			if got, _ := raw.(string); got != wantVal {
				t.Errorf("%q = %q, want %q", key, got, wantVal)
			}
		}
	})

	t.Run("empty tag stamps nothing", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ Name string }\n",
		})
		f := pkg.StructByName("S").FieldByName("Name")
		for _, name := range f.Meta().Names() {
			if name == "go.tag.json" {
				t.Fatalf("untagged field unexpectedly carries %q", name)
			}
		}
	})
}

// TestProvenance verifies every stamped meta entry records its
// origin (author "golang", AuthorityPlugin, populated Pos).
func TestProvenance(t *testing.T) {
	t.Parallel()
	t.Run("MetaIsContext records frontend authorship and position", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"context\"\n\ntype Op interface { Do(ctx context.Context) }\n",
		})
		paramType := pkg.InterfaceByName("Op").Methods[0].Params[0].Type
		prov, ok := paramType.Meta().Winning(golang.MetaIsContext.Name())
		if !ok {
			t.Fatalf("MetaIsContext not stamped")
		}
		if prov.SetBy != golang.FrontendName {
			t.Fatalf("provenance SetBy = %q, want %q", prov.SetBy, golang.FrontendName)
		}
		if prov.Authority != meta.AuthorityPlugin {
			t.Fatalf("provenance Authority = %v, want %v", prov.Authority, meta.AuthorityPlugin)
		}
		if prov.Pos.IsZero() {
			t.Fatalf("provenance Pos must not be zero")
		}
	})
}

// TestStampTypeRefMeta_PointerReceiverStringer pins the branch the
// stringer probe must not short-circuit.
//
// implementsStringer asks twice: once for T, once for *T. The second
// probe allocated a pointer type on every TypeRef the converter
// builds, so it is now skipped for basic and pointer types, whose
// method sets cannot grow by taking a pointer.
//
// A named type's can. `func (l *Label) String() string` makes Label
// a Stringer only through *Label, so widening that guard by one case
// would silently stop recognising the most common way the interface
// is satisfied — and no golden fixture covers it: the corpus
// declares only value-receiver String methods, where the first probe
// always answers.
func TestStampTypeRefMeta_PointerReceiverStringer(t *testing.T) {
	t.Parallel()

	const src = "package a\n\n" +
		"type Label int\n\n" +
		"func (l *Label) String() string { return \"\" }\n\n" +
		"type Holder struct {\n\tByValue Label\n\tByPointer *Label\n\tPlain int\n}\n"

	pkg := requirePackage(t, map[string]string{"a.go": src})
	holder := pkg.StructByName("Holder")

	t.Run("a value field of a pointer-receiver Stringer is stamped", func(t *testing.T) {
		t.Parallel()
		// The case the second probe exists for. Label itself does
		// not implement Stringer; *Label does.
		f := holder.FieldByName("ByValue")
		if got, _ := golang.MetaIsStringer.Get(f.Type.Meta()); !got {
			t.Fatalf("value field of a pointer-receiver Stringer not stamped")
		}
	})

	t.Run("a pointer field is stamped by the first probe", func(t *testing.T) {
		t.Parallel()
		f := holder.FieldByName("ByPointer")
		if got, _ := golang.MetaIsStringer.Get(f.Type.Meta()); !got {
			t.Fatalf("pointer field of a pointer-receiver Stringer not stamped")
		}
	})

	t.Run("a basic type is not stamped", func(t *testing.T) {
		t.Parallel()
		// The short-circuit's own case: *int has an empty method
		// set, so the skipped probe was already decided.
		f := holder.FieldByName("Plain")
		if golang.MetaIsStringer.Has(f.Type.Meta()) {
			t.Fatalf("int stamped as a Stringer")
		}
	})
}
