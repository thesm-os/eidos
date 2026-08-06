// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
)

// assertEqualString fails the test if got and want differ.
func assertEqualString(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("string mismatch:\n got:  %q\n want: %q", got, want)
	}
}

// assertNoError fails the test if err is non-nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// directiveAt builds a directive instance with a name and position.
func directiveAt(name directive.Name, pos position.Pos) *directive.Directive {
	return &directive.Directive{Name: name, Pos: pos, KV: map[string]string{}}
}

// builtinRef builds a [emit.BuiltinRef] for the named primitive type.
func builtinRef(name string) *emit.BuiltinRef {
	return emit.Builtin(name)
}

// externalRef builds a [emit.ExternalRef] for the named third-party
// type.
func externalRef(pkg, name string) *emit.ExternalRef {
	return emit.External(pkg, name)
}

// constraintFrom builds a [emit.Constraint] embedding refs as named
// bounds. Used by tests that need a quick generic-constraint instance
// without manual struct-literal noise.
func constraintFrom(refs ...emit.Ref) *emit.Constraint {
	return &emit.Constraint{Embedded: refs}
}

// recordingVisitor collects the directive.Kind of every node Walk
// visits, in visit order. Tests assert on the resulting slice.
type recordingVisitor struct {
	kinds *[]kind.Kind
}

// Visit appends the visited node's kind and continues descent.
func (r recordingVisitor) Visit(n emit.Node) emit.Visitor {
	*r.kinds = append(*r.kinds, n.Kind())
	return r
}

// recordWalk runs [emit.Walk] over n and returns the visit-order
// list of node kinds.
func recordWalk(n emit.Node) []kind.Kind {
	var kinds []kind.Kind
	emit.Walk(n, recordingVisitor{kinds: &kinds})
	return kinds
}

// slotHost is the generic slot-access surface every slot-bearing emit
// decl exposes. Declared here so the reserved-slot table can hold
// hosts of different concrete types in one slice.
type slotHost interface {
	Slot(name string) *emit.Slot
}

// slotKindCase pins one reserved slot name to the element kind it must
// carry, together with the typed accessor that names the same slot.
//
// newHost returns a fresh host per assertion: slots are created on
// first access and cached, so a shared host would let the first
// subtest decide what the rest observe — the very coupling these
// cases exist to rule out.
type slotKindCase struct {
	host     string
	slotName string
	want     kind.Kind
	newHost  func() slotHost
	typed    func(slotHost) *emit.Slot
	foreign  func() emit.Node
}

// label names a case for [testing.T.Run].
func (c slotKindCase) label() string { return c.host + "." + c.slotName }

// field returns a node of [emit.KindField], the stand-in foreign kind
// for every reserved slot that does not itself carry fields.
func field() emit.Node { return &emit.Field{Name: "Foreign"} }

// slotKindCases enumerates every reserved slot in the emit graph: the
// slots a host both names in a typed accessor and exposes by string
// through [slotHost.Slot].
//
// The pairing is the whole point. A reserved name reached through
// Slot("prebody") and through Prebody() has to describe one slot with
// one constraint; anything else makes a contributor's validation
// depend on which plugin touched the host first.
func slotKindCases() []slotKindCase {
	return []slotKindCase{
		{
			host: "Method", slotName: "prebody", want: emit.KindStmt,
			newHost: func() slotHost { return &emit.Method{Name: "Save"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Method).Prebody() },
			foreign: field,
		},
		{
			host: "Method", slotName: "postbody", want: emit.KindStmt,
			newHost: func() slotHost { return &emit.Method{Name: "Save"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Method).Postbody() },
			foreign: field,
		},
		{
			host: "Method", slotName: "params", want: emit.KindParam,
			newHost: func() slotHost { return &emit.Method{Name: "Save"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Method).ParamsSlot() },
			foreign: field,
		},
		{
			host: "Method", slotName: "returns", want: emit.KindReturn,
			newHost: func() slotHost { return &emit.Method{Name: "Save"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Method).ReturnsSlot() },
			foreign: field,
		},
		{
			host: "Function", slotName: "prebody", want: emit.KindStmt,
			newHost: func() slotHost { return &emit.Function{Name: "Run"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Function).Prebody() },
			foreign: field,
		},
		{
			host: "Function", slotName: "postbody", want: emit.KindStmt,
			newHost: func() slotHost { return &emit.Function{Name: "Run"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Function).Postbody() },
			foreign: field,
		},
		{
			host: "Function", slotName: "params", want: emit.KindParam,
			newHost: func() slotHost { return &emit.Function{Name: "Run"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Function).ParamsSlot() },
			foreign: field,
		},
		{
			host: "Function", slotName: "returns", want: emit.KindReturn,
			newHost: func() slotHost { return &emit.Function{Name: "Run"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Function).ReturnsSlot() },
			foreign: field,
		},
		{
			host: "Struct", slotName: "fields", want: emit.KindField,
			newHost: func() slotHost { return &emit.Struct{Name: "User"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Struct).FieldsSlot() },
			foreign: func() emit.Node { return &emit.Method{Name: "Foreign"} },
		},
		{
			host: "Struct", slotName: "methods", want: emit.KindMethod,
			newHost: func() slotHost { return &emit.Struct{Name: "User"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Struct).MethodsSlot() },
			foreign: field,
		},
		{
			host: "Struct", slotName: "embeds", want: emit.KindEmbed,
			newHost: func() slotHost { return &emit.Struct{Name: "User"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Struct).EmbedsSlot() },
			foreign: field,
		},
		{
			host: "Interface", slotName: "methods", want: emit.KindMethod,
			newHost: func() slotHost { return &emit.Interface{Name: "Store"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Interface).MethodsSlot() },
			foreign: field,
		},
		{
			host: "Interface", slotName: "embeds", want: emit.KindEmbed,
			newHost: func() slotHost { return &emit.Interface{Name: "Store"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Interface).EmbedsSlot() },
			foreign: field,
		},
		{
			host: "Alias", slotName: "methods", want: emit.KindMethod,
			newHost: func() slotHost { return &emit.Alias{Name: "ID"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Alias).MethodsSlot() },
			foreign: field,
		},
		{
			host: "Enum", slotName: "variants", want: emit.KindEnumVariant,
			newHost: func() slotHost { return &emit.Enum{Name: "Status"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.Enum).VariantsSlot() },
			foreign: field,
		},
		{
			host: "File", slotName: "imports", want: emit.KindImport,
			newHost: func() slotHost { return &emit.File{Name: "user.go"} },
			typed:   func(h slotHost) *emit.Slot { return h.(*emit.File).ImportsSlot() },
			foreign: field,
		},
	}
}
