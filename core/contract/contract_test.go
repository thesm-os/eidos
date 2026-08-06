// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package contract_test

import (
	"encoding/json"
	"testing"

	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
)

// TestOwnerRef_RoundTrip pins the JSON-survivable shape of
// [contract.OwnerRef]: the resolved owner pointer is lost on
// marshal, but the Graph + Kind + QName triple survives so a
// later rewire pass can repopulate the live [contract.Owner].
func TestOwnerRef_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("marshal-then-unmarshal preserves Kind + QName", func(t *testing.T) {
		t.Parallel()
		ref := contract.OwnerRef{
			Kind:  kind.Kind("node.enum"),
			QName: "example.com/store.Status",
		}
		buf, err := json.Marshal(ref)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got contract.OwnerRef
		if err := json.Unmarshal(buf, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got != ref {
			t.Fatalf("round-trip = %+v, want %+v", got, ref)
		}
	})

	t.Run("IsZero reports the zero-value sentinel", func(t *testing.T) {
		t.Parallel()
		if !(contract.OwnerRef{}).IsZero() {
			t.Fatalf("zero value should report IsZero == true")
		}
		populated := contract.OwnerRef{Kind: kind.Kind("emit.struct"), QName: "x.Y"}
		if populated.IsZero() {
			t.Fatalf("populated ref should report IsZero == false")
		}
	})
}

// TestRefOf pins the construction helper: passing a live
// [contract.Owner] produces a ref whose Kind + QName come from
// the owner itself. Kind alone discriminates the source/emit
// graph because the framework's [kind.Kind] values are already
// namespaced — no separate caller-supplied graph hint needed.
func TestRefOf(t *testing.T) {
	t.Parallel()

	t.Run("populated owner yields the matching ref", func(t *testing.T) {
		t.Parallel()
		owner := &fakeOwner{
			kind:  kind.Kind("node.enum"),
			name:  "Status",
			qname: "example.com/store.Status",
		}
		got := contract.RefOf(owner)
		want := contract.OwnerRef{
			Kind:  kind.Kind("node.enum"),
			QName: "example.com/store.Status",
		}
		if got != want {
			t.Fatalf("RefOf = %+v, want %+v", got, want)
		}
	})

	t.Run("nil owner yields the zero ref", func(t *testing.T) {
		t.Parallel()
		got := contract.RefOf(nil)
		if !got.IsZero() {
			t.Fatalf("RefOf(nil) should yield zero ref; got %+v", got)
		}
	})
}

// TestOwner_InterfaceContract pins the [contract.Owner] surface:
// the test fixture below satisfies the interface, which exercises
// every method signature plugins call against an owner without
// type-switching the concrete implementation.
func TestOwner_InterfaceContract(t *testing.T) {
	t.Parallel()

	var _ contract.Owner = (*fakeOwner)(nil)

	t.Run("OwnerName + OwnerQName flow through", func(t *testing.T) {
		t.Parallel()
		o := &fakeOwner{kind: kind.Kind("node.enum"), name: "Status", qname: "example.com/store.Status"}
		if got := o.OwnerName(); got != "Status" {
			t.Fatalf("OwnerName = %q, want %q", got, "Status")
		}
		if got := o.OwnerQName(); got != "example.com/store.Status" {
			t.Fatalf("OwnerQName = %q, want %q", got, "example.com/store.Status")
		}
	})
}

// fakeOwner is the minimal stand-in for an owner-eligible node
// — exercises the [contract.Owner] interface without dragging
// the emit or node packages in here. Real implementors
// (emit.Struct, node.Enum, …) gain the same one-line accessors
// behind the same interface.
type fakeOwner struct {
	kind  kind.Kind
	name  string
	qname string
}

func (o *fakeOwner) Kind() kind.Kind                               { return o.kind }
func (*fakeOwner) Pos() position.Pos                               { return position.Pos{} }
func (*fakeOwner) Docs() []string                                  { return nil }
func (*fakeOwner) Directives() []*directive.Directive              { return nil }
func (*fakeOwner) Directive(_ directive.Name) *directive.Directive { return nil }
func (*fakeOwner) HasDirective(_ directive.Name) bool              { return false }
func (*fakeOwner) Meta() *meta.Bag                                 { return nil }
func (*fakeOwner) EnsureMeta() *meta.Bag                           { return meta.NewBag() }
func (o *fakeOwner) OwnerName() string                             { return o.name }
func (o *fakeOwner) OwnerQName() string                            { return o.qname }
