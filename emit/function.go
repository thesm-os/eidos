// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"go.thesmos.sh/eidos/core/kind"
)

// Function is a standalone (non-method) function emit. Cross-cutting
// generators inject pre- and post-body statements through the
// "prebody" and "postbody" slots — same convention as [Method].
type Function struct {
	BaseEmit

	// Name is the function identifier.
	Name string `json:"name"`

	// Package is the package name the rendered file declares.
	Package string `json:"package,omitempty"`

	// Params are the function's positional parameters in source
	// order.
	Params []*Param `json:"params,omitempty"`

	// Returns are the function's return slots in source order.
	// Each slot carries an optional Name plus a required Type;
	// mixing named and unnamed slots in one slice is rejected at
	// render with [ErrMixedNamedReturns].
	Returns []*Return `json:"returns,omitempty"`

	// TypeParams are the function's generic type parameters.
	TypeParams []*TypeParam `json:"type_params,omitempty"`

	// Body holds the function's statement body in source order.
	Body []*Stmt `json:"body,omitempty"`

	// Target identifies where the backend writes this function's
	// rendered output.
	Target Target `json:"target,omitzero"`

	slotMap
}

// Kind returns [KindFunction].
func (*Function) Kind() kind.Kind { return KindFunction }

// QName returns the qualified name "Package.Name", or just "Name"
// when Package is empty.
func (f *Function) QName() string {
	if f.Package == "" {
		return f.Name
	}
	return f.Package + "." + f.Name
}

// Prebody returns the "prebody" slot for cross-cutting contributions
// that run before [Function.Body].
func (f *Function) Prebody() *Slot { return f.Slot(slotPrebody) }

// Postbody returns the "postbody" slot for cross-cutting
// contributions that run after [Function.Body] returns.
func (f *Function) Postbody() *Slot { return f.Slot(slotPostbody) }

// ParamsSlot returns the "params" slot for cross-cutting parameter
// injection.
func (f *Function) ParamsSlot() *Slot { return f.Slot(slotParams) }

// ReturnsSlot returns the "returns" slot for cross-cutting
// return-type injection. The slot's element kind is [KindReturn],
// so contributions are typed [*Return] values; the backend merges
// the typed [Function.Returns] slice with slot contributions in
// capability-topo + append order.
func (f *Function) ReturnsSlot() *Slot { return f.Slot(slotReturns) }

// Slot returns the named slot, creating it lazily.
func (f *Function) Slot(name string) *Slot { return f.slot(f, name, functionSlotKind(name)) }

// IsVariadic reports whether the function's last positional
// parameter is variadic.
func (f *Function) IsVariadic() bool {
	n := len(f.Params)
	return n > 0 && f.Params[n-1].Variadic
}

// IsGeneric reports whether the function declares generic type
// parameters.
func (f *Function) IsGeneric() bool { return len(f.TypeParams) > 0 }

// ParamCount returns the number of declared positional parameters.
func (f *Function) ParamCount() int { return len(f.Params) }

// ReturnCount returns the number of declared return values.
func (f *Function) ReturnCount() int { return len(f.Returns) }

// ParamByName returns the parameter with the given name, or nil
// when no such parameter exists. Empty name returns nil.
func (f *Function) ParamByName(name string) *Param {
	if name == "" {
		return nil
	}
	for _, p := range f.Params {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// ParamAt returns the parameter at the given index, or nil when out
// of range.
func (f *Function) ParamAt(i int) *Param {
	if i < 0 || i >= len(f.Params) {
		return nil
	}
	return f.Params[i]
}

// ReturnAt returns the return slot at the given index, or nil when
// out of range.
func (f *Function) ReturnAt(i int) *Return {
	if i < 0 || i >= len(f.Returns) {
		return nil
	}
	return f.Returns[i]
}

// functionSlotKind returns the element kind the named slot carries on a
// Function, or "" for a name this kind does not reserve.
//
// The kind is a property of the NAME, not of which accessor created
// the slot. Without this, Function.Slot(name) created an unconstrained
// slot while the typed accessor created a constrained one, and since
// creation is lookup-or-create the surviving constraint was decided by
// whichever plugin ran first — so two contributors to one host got
// different validation depending on registration order, and the
// permissive path was reachable by accident.
func functionSlotKind(name string) kind.Kind {
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
