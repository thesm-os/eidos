// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// namedTypeRef returns a named reference carrying a fresh meta bag.
func namedTypeRef(pkg, name string) *node.TypeRef {
	r := &node.TypeRef{TypeKind: node.TypeRefNamed, Package: pkg, Name: name}
	r.EnsureMeta()
	return r
}

// TestAccessors_AbsentStamp pins the answer every reader gives for
// a fact nobody stamped.
//
// This is the half that makes the readers worth having. Left to
// call Get themselves, each consumer decides what an absent stamp
// means, and the ones that find the value-plus-ok dance awkward
// re-derive the fact structurally instead — which is how a consumer
// ends up disagreeing with the frontend that stamped it.
//
// A run whose frontend never stamped these must not have every type
// report as an interface, so absent reads as false throughout.
func TestAccessors_AbsentStamp(t *testing.T) {
	t.Parallel()

	t.Run("an unstamped ref reports false for every boolean", func(t *testing.T) {
		t.Parallel()
		r := namedTypeRef("time", "Duration")
		for name, got := range map[string]bool{
			"IsError":      golang.IsError(r),
			"IsContext":    golang.IsContext(r),
			"IsStringer":   golang.IsStringer(r),
			"IsComparable": golang.IsComparable(r),
			"IsInterface":  golang.IsInterface(r),
			"IsChannel":    golang.IsChannel(r),
		} {
			if got {
				t.Errorf("%s on an unstamped ref = true, want false", name)
			}
		}
	})

	t.Run("a nil node reports false rather than panicking", func(t *testing.T) {
		t.Parallel()
		// Readers are called from templates and per-node loops where a
		// nil is a data gap, not a programming error; a panic there
		// surfaces as a framework fault against the caller's plugin.
		if golang.IsError(nil) || golang.IsChannel(nil) || golang.IsInterface(nil) {
			t.Fatalf("a nil ref must read as unstamped")
		}
		if golang.EmbedsInterface(nil) || golang.IsEmptyInterface(nil) {
			t.Fatalf("a nil declaration must read as unstamped")
		}
		if golang.ReceiverIsPointer(nil) {
			t.Fatalf("a nil method must read as unstamped")
		}
	})

	t.Run("an unstamped string reader returns empty", func(t *testing.T) {
		t.Parallel()
		if got := golang.UnderlyingKind(nil); got != "" {
			t.Fatalf("UnderlyingKind(nil) = %q, want empty", got)
		}
		if got := golang.ChanDir(namedTypeRef("", "int")); got != "" {
			t.Fatalf("ChanDir on a non-channel = %q, want empty", got)
		}
	})
}

// TestAccessors_ReadTheStamp pins that each reader resolves the key
// the frontend writes. A reader bound to the wrong key reports a
// fact that is silently always absent.
func TestAccessors_ReadTheStamp(t *testing.T) {
	t.Parallel()

	t.Run("boolean readers resolve their own key", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			key  meta.Key[bool]
			read func(*node.TypeRef) bool
		}{
			{"IsError", golang.MetaIsError, golang.IsError},
			{"IsContext", golang.MetaIsContext, golang.IsContext},
			{"IsStringer", golang.MetaIsStringer, golang.IsStringer},
			{"IsComparable", golang.MetaIsComparable, golang.IsComparable},
			{"IsInterface", golang.MetaIsInterface, golang.IsInterface},
			{"IsChannel", golang.MetaIsChannel, golang.IsChannel},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				r := namedTypeRef("", "x")
				tc.key.Set(r.EnsureMeta(), true, "test")
				if !tc.read(r) {
					t.Fatalf("%s does not read %s", tc.name, tc.key.Name())
				}
			})
		}
	})

	t.Run("EmbedsInterface reads the struct's stamp", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "Record"}
		golang.MetaEmbedsInterface.Set(s.EnsureMeta(), true, "test")
		if !golang.EmbedsInterface(s) {
			t.Fatalf("EmbedsInterface does not read go.embedsInterface")
		}
	})

	t.Run("ReceiverIsPointer reads the method's stamp", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "Save"}
		golang.MetaReceiverIsPointer.Set(m.EnsureMeta(), true, "test")
		if !golang.ReceiverIsPointer(m) {
			t.Fatalf("ReceiverIsPointer does not read go.receiverIsPointer")
		}
	})

	t.Run("UnderlyingKind reads the alias's stamp", func(t *testing.T) {
		t.Parallel()
		a := &node.Alias{Name: "ID"}
		golang.MetaUnderlyingKind.Set(a.EnsureMeta(), "basic", "test")
		if got := golang.UnderlyingKind(a); got != "basic" {
			t.Fatalf("UnderlyingKind = %q, want basic", got)
		}
	})

	t.Run("IotaValue distinguishes a zero value from an absent one", func(t *testing.T) {
		t.Parallel()
		// Zero is where an iota block starts, so a bare int return
		// would make the first variant indistinguishable from a
		// constant that is not an integer at all.
		c := &node.Constant{Name: "Active"}
		golang.MetaIotaValue.Set(c.EnsureMeta(), 0, "test")
		got, ok := golang.IotaValue(c)
		if !ok || got != 0 {
			t.Fatalf("IotaValue = (%d, %v), want (0, true)", got, ok)
		}
		if _, ok := golang.IotaValue(&node.Constant{Name: "Other"}); ok {
			t.Fatalf("an unstamped constant reported a value")
		}
	})
}

func TestChannelAccessors(t *testing.T) {
	t.Parallel()

	// chanRef builds a channel ref with the supplied direction and
	// element, as the frontend records one.
	chanRef := func(dir string, elem *node.TypeRef) *node.TypeRef {
		r := namedTypeRef("go", "chan")
		golang.MetaIsChannel.Set(r.EnsureMeta(), true, "test")
		golang.MetaChanDir.Set(r.EnsureMeta(), dir, "test")
		if elem != nil {
			r.TypeArgs = []*node.TypeRef{elem}
		}
		return r
	}

	t.Run("a bidirectional channel is recognised", func(t *testing.T) {
		t.Parallel()
		if !golang.IsBidirectionalChan(chanRef("both", nil)) {
			t.Fatalf("a both-direction channel must read as bidirectional")
		}
	})

	t.Run("a directional channel is not bidirectional", func(t *testing.T) {
		t.Parallel()
		// make() is not legal on one, which is what a caller asks this
		// in order to write.
		for _, dir := range []string{"send", "recv"} {
			if golang.IsBidirectionalChan(chanRef(dir, nil)) {
				t.Errorf("a %s channel must not read as bidirectional", dir)
			}
		}
	})

	t.Run("an unrecognised direction is not bidirectional", func(t *testing.T) {
		t.Parallel()
		// Matched positively: a direction this does not know is
		// likelier one make rejects than one it accepts.
		if golang.IsBidirectionalChan(chanRef("sideways", nil)) {
			t.Fatalf("an unknown direction must not read as bidirectional")
		}
	})

	t.Run("a non-channel is not bidirectional", func(t *testing.T) {
		t.Parallel()
		if golang.IsBidirectionalChan(namedTypeRef("", "int")) {
			t.Fatalf("a plain type must not read as a channel")
		}
	})

	t.Run("the element type comes from the structured type argument", func(t *testing.T) {
		t.Parallel()
		// A caller receives something it can lift into an emit.Ref
		// rather than the printed form it would have to parse.
		elem := namedTypeRef("", "int")
		if got := golang.ChanElem(chanRef("both", elem)); got != elem {
			t.Fatalf("ChanElem = %v, want the type argument", got)
		}
	})

	t.Run("a channel with no element reports nil", func(t *testing.T) {
		t.Parallel()
		if got := golang.ChanElem(chanRef("both", nil)); got != nil {
			t.Fatalf("ChanElem = %v, want nil", got)
		}
	})

	t.Run("a non-channel has no element", func(t *testing.T) {
		t.Parallel()
		r := namedTypeRef("", "Container")
		r.TypeArgs = []*node.TypeRef{namedTypeRef("", "int")}
		if got := golang.ChanElem(r); got != nil {
			t.Fatalf("ChanElem on a generic type = %v, want nil", got)
		}
	})
}

func TestIteratorAccessors(t *testing.T) {
	t.Parallel()

	t.Run("an unstamped function is not an iterator", func(t *testing.T) {
		t.Parallel()
		// The zero value reads as "not one" rather than as an
		// unhandled case.
		if got := golang.IteratorOf(&node.Function{Name: "Get"}); got != golang.NotIterator {
			t.Fatalf("IteratorOf = %q, want NotIterator", got)
		}
	})

	t.Run("a Seq is classified", func(t *testing.T) {
		t.Parallel()
		f := &node.Function{Name: "All"}
		golang.MetaIsIterSeq.Set(f.EnsureMeta(), true, "test")
		if got := golang.IteratorOf(f); got != golang.SeqIterator {
			t.Fatalf("IteratorOf = %q, want SeqIterator", got)
		}
	})

	t.Run("a Seq2 is classified", func(t *testing.T) {
		t.Parallel()
		f := &node.Function{Name: "All"}
		golang.MetaIsIterSeq2.Set(f.EnsureMeta(), true, "test")
		if got := golang.IteratorOf(f); got != golang.Seq2Iterator {
			t.Fatalf("IteratorOf = %q, want Seq2Iterator", got)
		}
	})

	t.Run("Seq2 wins when both are stamped", func(t *testing.T) {
		t.Parallel()
		// Seq2 is the more specific shape; reporting Seq for one would
		// lose the key type a caller collects.
		f := &node.Function{Name: "All"}
		golang.MetaIsIterSeq.Set(f.EnsureMeta(), true, "test")
		golang.MetaIsIterSeq2.Set(f.EnsureMeta(), true, "test")
		if got := golang.IteratorOf(f); got != golang.Seq2Iterator {
			t.Fatalf("IteratorOf = %q, want Seq2Iterator", got)
		}
	})

	t.Run("the key and value types read back", func(t *testing.T) {
		t.Parallel()
		f := &node.Function{Name: "All"}
		golang.MetaIterKeyType.Set(f.EnsureMeta(), "string", "test")
		golang.MetaIterValueType.Set(f.EnsureMeta(), "int", "test")
		if golang.IterKeyType(f) != "string" || golang.IterValueType(f) != "int" {
			t.Fatalf("iterator types = (%q, %q), want (string, int)",
				golang.IterKeyType(f), golang.IterValueType(f))
		}
	})
}

func TestTagAccessors(t *testing.T) {
	t.Parallel()

	// tagged returns a field carrying the supplied struct-tag entries.
	tagged := func(entries map[string]string) *node.Field {
		f := &node.Field{Name: "ID"}
		for name, value := range entries {
			meta.EnsureKey(golang.MetaTagPrefix+name, meta.StringParser).
				Set(f.EnsureMeta(), value, "test")
		}
		return f
	}

	t.Run("reads one entry by name", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.Tag(tagged(map[string]string{"json": "id"}), "json")
		if !ok || got != "id" {
			t.Fatalf("Tag(json) = (%q, %v), want (id, true)", got, ok)
		}
	})

	t.Run("reports an absent entry", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.Tag(tagged(map[string]string{"json": "id"}), "db"); ok {
			t.Fatalf("an absent tag reported present")
		}
	})

	t.Run("collects every entry the field carries", func(t *testing.T) {
		t.Parallel()
		// Built by walking the bag rather than probing a known set,
		// because the set is whatever the source declared.
		got := golang.Tags(tagged(map[string]string{"json": "id", "db": "id_col"}))
		if len(got) != 2 || got["json"] != "id" || got["db"] != "id_col" {
			t.Fatalf("Tags = %v, want both entries", got)
		}
	})

	t.Run("a field carrying no tags collects nothing", func(t *testing.T) {
		t.Parallel()
		if got := golang.Tags(&node.Field{Name: "ID"}); len(got) != 0 {
			t.Fatalf("Tags = %v, want empty", got)
		}
	})

	t.Run("non-tag metadata is not collected", func(t *testing.T) {
		t.Parallel()
		// The namespace prefix is what separates a tag from every
		// other fact stamped on the same field.
		f := tagged(map[string]string{"json": "id"})
		golang.MetaGoName.Set(f.EnsureMeta(), "ID", "test")
		if got := golang.Tags(f); len(got) != 1 {
			t.Fatalf("Tags = %v, want only the json entry", got)
		}
	})

	t.Run("a nil field and an empty name read as absent", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.Tag(nil, "json"); ok {
			t.Fatalf("a nil field reported a tag")
		}
		if _, ok := golang.Tag(tagged(nil), ""); ok {
			t.Fatalf("an empty tag name reported a value")
		}
		if got := golang.Tags(nil); got != nil {
			t.Fatalf("Tags(nil) = %v, want nil", got)
		}
	})
}

func TestBridgeAccessors(t *testing.T) {
	t.Parallel()

	t.Run("GoType distinguishes an unstamped ref from an empty stamp", func(t *testing.T) {
		t.Parallel()
		// An unstamped ref falls back to the node's own name at the
		// render site, which is not the same as one a bridge
		// deliberately rendered as empty.
		r := namedTypeRef("", "Item")
		if _, ok := golang.GoType(r); ok {
			t.Fatalf("an unstamped ref reported a rendered type")
		}
		golang.MetaGoType.Set(r.EnsureMeta(), "", "bridge")
		if _, ok := golang.GoType(r); !ok {
			t.Fatalf("an empty stamp must still report present")
		}
	})

	t.Run("GoName and GoImport read their stamps", func(t *testing.T) {
		t.Parallel()
		p := &node.Package{Name: "pb", Path: "example.com/pb"}
		golang.MetaGoName.Set(p.EnsureMeta(), "pbv1", "bridge")
		golang.MetaGoImport.Set(p.EnsureMeta(), "example.com/gen/pb", "bridge")
		if got, _ := golang.GoName(p); got != "pbv1" {
			t.Fatalf("GoName = %q, want pbv1", got)
		}
		if got, _ := golang.GoImport(p); got != "example.com/gen/pb" {
			t.Fatalf("GoImport = %q, want example.com/gen/pb", got)
		}
	})

	t.Run("nil inputs read as unstamped", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.GoType(nil); ok {
			t.Fatalf("GoType(nil) reported present")
		}
		if _, ok := golang.GoName(nil); ok {
			t.Fatalf("GoName(nil) reported present")
		}
		if _, ok := golang.GoImport(nil); ok {
			t.Fatalf("GoImport(nil) reported present")
		}
	})
}

func TestConstraintTermsAccessor(t *testing.T) {
	t.Parallel()

	t.Run("reads the terms a constraint declares", func(t *testing.T) {
		t.Parallel()
		p := &node.TypeParam{Name: "T"}
		golang.MetaConstraintTerms.Set(p.EnsureMeta(), []golang.ConstraintTerm{
			{Type: namedTypeRef("", "int"), Approximate: true},
		}, "test")
		got := golang.ConstraintTerms(p)
		if len(got) != 1 || !got[0].Approximate {
			t.Fatalf("ConstraintTerms = %+v, want one approximate term", got)
		}
	})

	t.Run("a parameter declaring none reads as nil", func(t *testing.T) {
		t.Parallel()
		if got := golang.ConstraintTerms(&node.TypeParam{Name: "T"}); got != nil {
			t.Fatalf("ConstraintTerms = %+v, want nil", got)
		}
		if got := golang.ConstraintTerms(nil); got != nil {
			t.Fatalf("ConstraintTerms(nil) = %+v, want nil", got)
		}
	})
}

func TestIsConstraintInterface(t *testing.T) {
	t.Parallel()

	t.Run("reads the frontend's stamp", func(t *testing.T) {
		t.Parallel()
		// Preferred over the model-level node.IsConstraint where the
		// Go frontend ran: the frontend resolved the declaration
		// through the type checker, while the structural answer infers.
		i := &node.Interface{Name: "Numeric"}
		golang.MetaIsConstraintInterface.Set(i.EnsureMeta(), true, "test")
		if !golang.IsConstraintInterface(i) {
			t.Fatalf("IsConstraintInterface does not read go.isConstraintInterface")
		}
	})

	t.Run("an unstamped interface is a method-set contract", func(t *testing.T) {
		t.Parallel()
		if golang.IsConstraintInterface(&node.Interface{Name: "Store"}) {
			t.Fatalf("an unstamped interface must not read as a constraint")
		}
	})
}

func TestTags_NonStringValue(t *testing.T) {
	t.Parallel()

	t.Run("skips a tag-namespaced key holding a non-string", func(t *testing.T) {
		t.Parallel()
		// The namespace is registered dynamically, so nothing stops a
		// plugin claiming a name under it with another type. Skipping
		// beats a panic or a coerced value nobody wrote.
		f := &node.Field{Name: "ID"}
		meta.EnsureKey(golang.MetaTagPrefix+"json", meta.StringParser).
			Set(f.EnsureMeta(), "id", "test")
		meta.EnsureKey(golang.MetaTagPrefix+"count", meta.BoolParser).
			Set(f.EnsureMeta(), true, "test")
		got := golang.Tags(f)
		if len(got) != 1 || got["json"] != "id" {
			t.Fatalf("Tags = %v, want only the string entry", got)
		}
	})
}

func TestTags_Tombstoned(t *testing.T) {
	t.Parallel()

	t.Run("skips a tag a later authority removed", func(t *testing.T) {
		t.Parallel()
		// A tombstoned key still appears in the bag's name list — the
		// removal is recorded rather than erased, so provenance
		// survives — but reads as unset. Collecting it would resurrect
		// a tag the source no longer carries.
		f := &node.Field{Name: "ID"}
		k := meta.EnsureKey(golang.MetaTagPrefix+"json", meta.StringParser)
		k.Set(f.EnsureMeta(), "id", "test")
		k.Tombstone(f.EnsureMeta(), "test")
		if got := golang.Tags(f); len(got) != 0 {
			t.Fatalf("Tags = %v, want none after the tombstone", got)
		}
	})
}
