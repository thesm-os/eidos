// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/core/kind"
)

// Compile-time assertion that [*Struct] satisfies the cross-graph
// [contract.Owner] surface — fails to build if either accessor
// drifts from the interface signature.
var _ contract.Owner = (*Struct)(nil)

// Struct is a structured product type emit. Fields and methods live
// in typed slices for direct owner-generator use; the "fields",
// "methods", and "embeds" slots carry cross-cutting contributions
// from other generators (ValidationGen adding tagged fields,
// MockGen adding methods, etc.).
type Struct struct {
	BaseEmit

	// Name is the struct's identifier.
	Name string `json:"name"`

	// Package is the package name the rendered file declares.
	// (Distinct from import paths — this is the local declaration
	// package.) Empty for anonymous struct types.
	Package string `json:"package,omitempty"`

	// Fields are the named fields contributed by the owner generator.
	// Cross-cutting field additions append through [Struct.FieldsSlot].
	Fields []*Field `json:"fields,omitempty"`

	// Embeds are the embedded types contributed by the owner.
	// Cross-cutting embedded-type additions append through
	// [Struct.EmbedsSlot].
	Embeds []*Embed `json:"embeds,omitempty"`

	// Methods are the methods declared on the struct by the owner.
	// Cross-cutting method additions append through
	// [Struct.MethodsSlot].
	Methods []*Method `json:"methods,omitempty"`

	// TypeParams are the struct's generic type parameters.
	TypeParams []*TypeParam `json:"type_params,omitempty"`

	// Target identifies where the backend writes this struct's
	// rendered output. Multiple emit entities sharing a Target
	// compose into the same file.
	Target Target `json:"target,omitzero"`

	slotMap
}

// Kind returns [KindStruct].
func (*Struct) Kind() kind.Kind { return KindStruct }

// QName returns the qualified name "Package.Name", or just "Name"
// when Package is empty.
func (s *Struct) QName() string {
	if s.Package == "" {
		return s.Name
	}
	return s.Package + "." + s.Name
}

// OwnerName satisfies [contract.Owner]; returns the struct's
// bare identifier. The accessor lets [Method.Owner] (typed as
// [contract.Owner]) hand back the owner identifier without the
// caller type-switching on the concrete kind.
func (s *Struct) OwnerName() string { return s.Name }

// OwnerQName satisfies [contract.Owner]; synonym for
// [Struct.QName] under the [contract.Owner] interface.
func (s *Struct) OwnerQName() string { return s.QName() }

// FieldsSlot returns the "fields" slot for cross-cutting field
// injection.
func (s *Struct) FieldsSlot() *Slot { return s.Slot(slotFields) }

// MethodsSlot returns the "methods" slot for cross-cutting method
// injection.
func (s *Struct) MethodsSlot() *Slot { return s.Slot(slotMethods) }

// EmbedsSlot returns the "embeds" slot for cross-cutting embed
// injection.
func (s *Struct) EmbedsSlot() *Slot { return s.Slot(slotEmbeds) }

// Slot returns the named slot, creating it lazily without an
// element-kind constraint. Used for custom slot names plugins
// declare alongside their emit kinds.
func (s *Struct) Slot(name string) *Slot { return s.slot(s, name, structSlotKind(name)) }

// IsGeneric reports whether the struct declares generic type
// parameters.
func (s *Struct) IsGeneric() bool { return len(s.TypeParams) > 0 }

// FieldByName returns the named field, or nil when no field with
// that name exists. Does not search the [Struct.FieldsSlot].
func (s *Struct) FieldByName(name string) *Field {
	for _, f := range s.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// FieldsWith returns fields matching pred in declaration order.
func (s *Struct) FieldsWith(pred func(*Field) bool) []*Field {
	out := make([]*Field, 0, len(s.Fields))
	for _, f := range s.Fields {
		if pred(f) {
			out = append(out, f)
		}
	}
	return out
}

// MethodByName returns the method with the given name, or nil when
// no such method exists.
func (s *Struct) MethodByName(name string) *Method {
	for _, m := range s.Methods {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// MethodsWith returns methods matching pred in declaration order.
func (s *Struct) MethodsWith(pred func(*Method) bool) []*Method {
	out := make([]*Method, 0, len(s.Methods))
	for _, m := range s.Methods {
		if pred(m) {
			out = append(out, m)
		}
	}
	return out
}

// structSlotKind returns the element kind the named slot carries on a
// Struct, or "" for a name this kind does not reserve.
//
// The kind is a property of the NAME, not of which accessor created
// the slot. Without this, Struct.Slot(name) created an unconstrained
// slot while the typed accessor created a constrained one, and since
// creation is lookup-or-create the surviving constraint was decided by
// whichever plugin ran first — so two contributors to one host got
// different validation depending on registration order, and the
// permissive path was reachable by accident.
func structSlotKind(name string) kind.Kind {
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
