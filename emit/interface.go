// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/core/kind"
)

// Compile-time assertion that [*Interface] satisfies
// [contract.Owner] — fails to build if either accessor drifts.
var _ contract.Owner = (*Interface)(nil)

// Interface is a contract a type satisfies without being
// instantiable. Methods declared inside an interface have a nil
// [Method.Receiver] — the receiver is implicit in the interface
// contract.
//
// Mirrors [node.Interface], including its Fields: what separates an
// Interface from a [Struct] is instantiability, not which members it
// may declare. A generator emitting a TypeScript interface populates
// Fields and usually no Methods at all; one emitting a Go interface
// does the reverse.
type Interface struct {
	BaseEmit

	// Name is the interface's identifier.
	Name string `json:"name"`

	// Package is the package name the rendered file declares.
	// Empty for anonymous interface types.
	Package string `json:"package,omitempty"`

	// Fields are the interface's declared data members in source
	// order. Empty for a language whose interfaces are method sets.
	Fields []*Field `json:"fields,omitempty"`

	// Methods are the interface's declared method signatures in
	// source order. Each Method has a nil Receiver.
	Methods []*Method `json:"methods,omitempty"`

	// Embeds are the embedded interfaces (and union constraint
	// terms in Go's generic-type-set position) in source order.
	Embeds []*Embed `json:"embeds,omitempty"`

	// TypeParams are the interface's generic type parameters.
	TypeParams []*TypeParam `json:"type_params,omitempty"`

	// Target identifies where the backend writes this interface's
	// rendered output.
	Target Target `json:"target,omitzero"`

	slotMap
}

// Kind returns [KindInterface].
func (*Interface) Kind() kind.Kind { return KindInterface }

// QName returns the qualified name "Package.Name", or just "Name"
// when Package is empty.
func (i *Interface) QName() string {
	if i.Package == "" {
		return i.Name
	}
	return i.Package + "." + i.Name
}

// OwnerName satisfies [contract.Owner]; returns the interface's
// bare identifier.
func (i *Interface) OwnerName() string { return i.Name }

// OwnerQName satisfies [contract.Owner]; synonym for
// [Interface.QName].
func (i *Interface) OwnerQName() string { return i.QName() }

// FieldsSlot returns the "fields" slot for cross-cutting field
// injection.
func (i *Interface) FieldsSlot() *Slot { return i.Slot(slotFields) }

// MethodsSlot returns the "methods" slot for cross-cutting method
// injection.
func (i *Interface) MethodsSlot() *Slot { return i.Slot(slotMethods) }

// EmbedsSlot returns the "embeds" slot for cross-cutting embed
// injection.
func (i *Interface) EmbedsSlot() *Slot { return i.Slot(slotEmbeds) }

// Slot returns the named slot, creating it lazily without an
// element-kind constraint.
func (i *Interface) Slot(name string) *Slot { return i.slot(i, name, interfaceSlotKind(name)) }

// IsGeneric reports whether the interface declares generic type
// parameters.
func (i *Interface) IsGeneric() bool { return len(i.TypeParams) > 0 }

// FieldByName returns the field with the given name, or nil when no
// such field exists.
func (i *Interface) FieldByName(name string) *Field {
	for _, f := range i.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// FieldsWith returns fields matching pred in declaration order.
func (i *Interface) FieldsWith(pred func(*Field) bool) []*Field {
	out := make([]*Field, 0, len(i.Fields))
	for _, f := range i.Fields {
		if pred(f) {
			out = append(out, f)
		}
	}
	return out
}

// MethodByName returns the method with the given name, or nil when
// no such method exists.
func (i *Interface) MethodByName(name string) *Method {
	for _, m := range i.Methods {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// MethodsWith returns methods matching pred in declaration order.
func (i *Interface) MethodsWith(pred func(*Method) bool) []*Method {
	out := make([]*Method, 0, len(i.Methods))
	for _, m := range i.Methods {
		if pred(m) {
			out = append(out, m)
		}
	}
	return out
}

// interfaceSlotKind returns the element kind the named slot carries on a
// Interface, or "" for a name this kind does not reserve.
//
// The kind is a property of the NAME, not of which accessor created
// the slot. Without this, Interface.Slot(name) created an unconstrained
// slot while the typed accessor created a constrained one, and since
// creation is lookup-or-create the surviving constraint was decided by
// whichever plugin ran first — so two contributors to one host got
// different validation depending on registration order, and the
// permissive path was reachable by accident.
func interfaceSlotKind(name string) kind.Kind {
	switch name {
	case slotFields:
		return KindField
	case slotMethods:
		return KindMethod
	case slotEmbeds:
		return KindEmbed
	default:
		return ""
	}
}
