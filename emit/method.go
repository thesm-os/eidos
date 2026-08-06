// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/core/kind"
)

// Method is a method declaration — a function with a receiver.
// Methods attach to a [contract.Owner]: a [Struct] / [Interface] /
// [Alias] on the emit side (the conventional cases) or a source-
// side [node.Struct] / [node.Interface] / [node.Enum] / [node.Alias]
// when a generator emits a method onto a user-declared type
// (the enum-stringer / sentinel-error pattern). The render path is
// identical in both cases; the Owner field just distinguishes the
// graph the receiver type lives in.
//
// Methods can be reached two ways in the emit graph:
//
//   - As a child of an owner-eligible decl ([Struct.Methods],
//     [Interface.Methods], [Alias.Methods]) — the conventional
//     case, e.g. mockgen's struct methods, buildergen's setters.
//   - As a top-level entry on [Package.Methods] — the case for
//     methods on a source-side type the plugin did not emit. The
//     [Method.Target] / [Method.Package] fields route the
//     rendered output; the framework's layout phase stamps Target
//     the same way it does for [Function].
//
// Method exposes four standard slots:
//
//   - "prebody"  — Stmts run before the method's Body.
//     Common target of cross-cutters (logging, validation, audit).
//   - "postbody" — Stmts run after the method's Body returns. Less
//     common but useful for deferred work.
//   - "params"   — Param injection (rare; usually parameters live
//     in the typed Params slice).
//   - "returns"  — Return-type injection (rare).
type Method struct {
	BaseEmit

	// Name is the method identifier.
	Name string `json:"name"`

	// Package is the import path the rendered method declares
	// when the method is a top-level decl on [Package.Methods].
	// Empty for nested methods (where Package is inherited from
	// the owner's Package field at render time).
	Package string `json:"package,omitempty"`

	// Receiver is the receiver type of a struct method. nil for
	// methods declared inside an interface.
	Receiver Ref `json:"-"`

	// ReceiverName is the receiver variable name for the generated
	// method ("r" in `func (r *Repo) Get()`). Empty for interface
	// methods and for receiver declarations using the blank
	// receiver form.
	ReceiverName string `json:"receiver_name,omitempty"`

	// Params are the method's positional parameters in source order.
	Params []*Param `json:"params,omitempty"`

	// Returns are the method's return slots in source order. Each
	// slot carries an optional Name plus a required Type; mixing
	// named and unnamed slots in one slice is rejected at render
	// with [ErrMixedNamedReturns].
	Returns []*Return `json:"returns,omitempty"`

	// TypeParams are the method's generic type parameters.
	TypeParams []*TypeParam `json:"type_params,omitempty"`

	// Body holds the method's statement body in source order.
	// Cross-cutting injection goes through the prebody / postbody
	// slots rather than mutating Body directly.
	Body []*Stmt `json:"body,omitempty"`

	// Target identifies where the backend writes this method's
	// rendered output. Populated by the framework's layout phase
	// for top-level methods on [Package.Methods]; left at the
	// zero value for nested methods (which inherit routing from
	// their owner).
	Target Target `json:"target,omitzero"`

	// Owner is the [contract.Owner] this method conceptually
	// belongs to — an emit-side [Struct] / [Interface] / [Alias]
	// for methods on emitted types, or a source-side
	// [node.Struct] / [node.Interface] / [node.Enum] / [node.Alias]
	// for methods on user-declared types. Excluded from JSON
	// encoding to break the host → child cycle; deserialised
	// graphs re-wire Owner via [RewireOwners] using [OwnerRef].
	Owner contract.Owner `json:"-"`

	// OwnerRef is the JSON-survivable form of Owner — the
	// {Kind, QName} tuple the rewire pass resolves against the
	// live store. Populated alongside Owner at construction time
	// by the framework's package builder.
	OwnerRef contract.OwnerRef `json:"owner_ref,omitzero"`

	slotMap
}

// Kind returns [KindMethod].
func (*Method) Kind() kind.Kind { return KindMethod }

// OwnerName returns the [contract.Owner.OwnerName] of m's Owner,
// or the empty string when Owner is nil. Plugins query this
// instead of type-switching on the concrete Owner type to derive
// the conceptual owner-type identifier.
func (m *Method) OwnerName() string {
	if m.Owner == nil {
		return ""
	}
	return m.Owner.OwnerName()
}

// OwnerQName returns the [contract.Owner.OwnerQName] of m's
// Owner, or the empty string when Owner is nil.
func (m *Method) OwnerQName() string {
	if m.Owner == nil {
		return ""
	}
	return m.Owner.OwnerQName()
}

// QName returns the qualified name "<package>.<owner>.<name>"
// for a top-level method, "<owner-qname>.<name>" for a nested
// method (Owner is set, Package is empty), or "<name>" when both
// are absent. Used for diagnostics and store-key composition.
func (m *Method) QName() string {
	switch {
	case m.Package != "" && m.Owner != nil:
		return m.Package + "." + m.Owner.OwnerName() + "." + m.Name
	case m.Owner != nil:
		return m.Owner.OwnerQName() + "." + m.Name
	case m.Package != "":
		return m.Package + "." + m.Name
	default:
		return m.Name
	}
}

// Prebody returns the "prebody" slot for cross-cutting contributions
// that run before [Method.Body].
func (m *Method) Prebody() *Slot { return m.Slot(slotPrebody) }

// Postbody returns the "postbody" slot for cross-cutting
// contributions that run after [Method.Body] returns.
func (m *Method) Postbody() *Slot { return m.Slot(slotPostbody) }

// ParamsSlot returns the "params" slot for cross-cutting parameter
// injection. Distinct from [Method.Params] (the typed field for the
// owning generator's direct content).
func (m *Method) ParamsSlot() *Slot { return m.Slot(slotParams) }

// ReturnsSlot returns the "returns" slot for cross-cutting
// return-type injection. The slot's element kind is [KindReturn],
// so contributions are typed [*Return] values; the backend merges
// the typed [Method.Returns] slice with slot contributions in
// capability-topo + append order.
func (m *Method) ReturnsSlot() *Slot { return m.Slot(slotReturns) }

// Slot returns the named slot, creating it lazily without an
// element-kind constraint. Used for custom slot names a plugin
// declares alongside its emit kinds.
func (m *Method) Slot(name string) *Slot { return m.slot(m, name, methodSlotKind(name)) }

// HasReceiver reports whether the method has an explicit receiver.
func (m *Method) HasReceiver() bool { return m.Receiver != nil }

// IsVariadic reports whether the method's last positional parameter
// is variadic.
func (m *Method) IsVariadic() bool {
	n := len(m.Params)
	return n > 0 && m.Params[n-1].Variadic
}

// IsGeneric reports whether the method declares generic type
// parameters of its own.
func (m *Method) IsGeneric() bool { return len(m.TypeParams) > 0 }

// ParamCount returns the number of declared positional parameters.
func (m *Method) ParamCount() int { return len(m.Params) }

// ReturnCount returns the number of declared return values.
func (m *Method) ReturnCount() int { return len(m.Returns) }

// ParamByName returns the parameter with the given name, or nil
// when no such parameter exists. Empty name returns nil.
func (m *Method) ParamByName(name string) *Param {
	if name == "" {
		return nil
	}
	for _, p := range m.Params {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// ParamAt returns the parameter at the given index, or nil when
// out of range. Bounds-checked positional access.
func (m *Method) ParamAt(i int) *Param {
	if i < 0 || i >= len(m.Params) {
		return nil
	}
	return m.Params[i]
}

// ReturnAt returns the return slot at the given index, or nil when
// out of range.
func (m *Method) ReturnAt(i int) *Return {
	if i < 0 || i >= len(m.Returns) {
		return nil
	}
	return m.Returns[i]
}

// methodSlotKind returns the element kind the named slot carries on a
// Method, or "" for a name this kind does not reserve.
//
// The kind is a property of the NAME, not of which accessor created
// the slot. Without this, Method.Slot(name) created an unconstrained
// slot while the typed accessor created a constrained one, and since
// creation is lookup-or-create the surviving constraint was decided by
// whichever plugin ran first — so two contributors to one host got
// different validation depending on registration order, and the
// permissive path was reachable by accident.
func methodSlotKind(name string) kind.Kind {
	switch name {
	case slotPrebody, slotPostbody:
		return KindStmt
	case slotParams:
		return KindParam
	case slotReturns:
		return KindReturn
	default:
		return ""
	}
}
