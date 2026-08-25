// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import (
	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/core/kind"
)

// Compile-time assertion that [*Interface] satisfies
// [contract.Owner] — fails to build if either accessor drifts.
var _ contract.Owner = (*Interface)(nil)

// Interface is a contract a type satisfies without being
// instantiable — Go's interface, Rust's trait, TypeScript's
// interface, Swift's protocol. Embedded interfaces surface as [Embed]
// nodes; explicitly-declared methods surface as Methods (with nil
// Receiver).
//
// What separates Interface from [Struct] is instantiability, not
// which members it may declare: a Struct is a type values are made
// of, an Interface is a shape values are checked against. Both may
// carry fields and methods, and which of the two a language populates
// is the language's own business — a Go interface declares only
// methods, a TypeScript interface usually declares only fields.
type Interface struct {
	BaseNode

	// Name is the interface's identifier.
	Name string `json:"name"`

	// Package is the source package path. Empty for anonymous
	// interface types declared inline.
	Package string `json:"package,omitempty"`

	// Fields are the interface's declared data members in source
	// order.
	//
	// Empty for a language whose interfaces are method sets, which is
	// Go's and Rust's case. TypeScript's are structural types and
	// most of them declare no methods at all, so this is where the
	// bulk of such a declaration lives. [Alias.Methods] is the
	// mirror-image field: present because one language allows
	// something the others do not, and empty everywhere else.
	Fields []*Field `json:"fields,omitempty"`

	// Methods are the interface's declared method signatures in
	// source order. Each Method has a nil Receiver — the receiver
	// is implicit in the interface.
	Methods []*Method `json:"methods,omitempty"`

	// Embeds are the embedded interfaces (and union constraint
	// terms in Go's generic-type-set position) in source order.
	Embeds []*Embed `json:"embeds,omitempty"`

	// TypeParams are the interface's generic type parameters.
	TypeParams []*TypeParam `json:"type_params,omitempty"`
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

// FieldByName returns the named field, or nil when no field with
// that name is declared.
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

// OwnerName satisfies [contract.Owner]; returns the interface's
// bare identifier.
func (i *Interface) OwnerName() string { return i.Name }

// OwnerQName satisfies [contract.Owner]; synonym for
// [Interface.QName].
func (i *Interface) OwnerQName() string { return i.QName() }

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

// IsGeneric reports whether the interface declares generic type
// parameters.
func (i *Interface) IsGeneric() bool { return len(i.TypeParams) > 0 }
