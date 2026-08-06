// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import (
	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/core/kind"
)

// Compile-time assertion that [*Enum] satisfies
// [contract.Owner] — fails to build if either accessor drifts.
var _ contract.Owner = (*Enum)(nil)

// Enum is the language-agnostic enum model — a named type plus a set
// of typed variants of that underlying type.
//
// Frontends populate Enum via different routes per language:
//
//   - Go: shape detection over a group of typed constants
//     (`type Status int` + `const (StatusActive Status = iota; …)`).
//   - Rust / TypeScript / others: direct translation from
//     first-class enum / discriminated-union syntax.
//
// The model is intentionally simple — typed variants over an
// underlying type — and does not attempt to capture Rust-style
// payload variants. Frontends that need to model payload-carrying
// enums use a combination of [Enum] + plugin-specific metadata or
// represent them as interface + struct variants instead.
type Enum struct {
	BaseNode

	// Name is the enum's type identifier.
	Name string `json:"name"`

	// Package is the source package path.
	Package string `json:"package,omitempty"`

	// Underlying is the enum's underlying type (Go's `type Status
	// int` would set this to a Named TypeRef for "int").
	Underlying *TypeRef `json:"underlying,omitempty"`

	// Variants are the declared variants in source order.
	Variants []*EnumVariant `json:"variants,omitempty"`

	// Methods declared on the enum's type.
	//
	// Coalescing a typed-iota group into an Enum subsumes the
	// [Alias] that would otherwise have carried them, so they live
	// here instead: one rule — a defined type carries its methods —
	// holds regardless of which node the frontend chose for the
	// declaration. Without this the same `type Status int` keeps its
	// methods right up until a const block makes it an enum, and
	// then loses them silently.
	//
	// What a consumer does with them: the method set is what decides
	// which assertions a generator can honestly make about an enum.
	// String plus Parse admits a round-trip law; String alone admits
	// only distinctness and the out-of-range fallback; neither
	// leaves just the constant set. A methodless graph forces the
	// weakest form on every enum, or emits round-trips against
	// methods that may not exist.
	Methods []*Method `json:"methods,omitempty"`
}

// Kind returns [KindEnum].
func (*Enum) Kind() kind.Kind { return KindEnum }

// QName returns the qualified name "Package.Name", or just "Name"
// when Package is empty.
func (e *Enum) QName() string {
	if e.Package == "" {
		return e.Name
	}
	return e.Package + "." + e.Name
}

// OwnerName satisfies [contract.Owner]; returns the enum's bare
// identifier.
func (e *Enum) OwnerName() string { return e.Name }

// OwnerQName satisfies [contract.Owner]; synonym for [Enum.QName].
func (e *Enum) OwnerQName() string { return e.QName() }

// VariantByName returns the variant with the given name, or nil
// when no such variant exists.
func (e *Enum) VariantByName(name string) *EnumVariant {
	for _, v := range e.Variants {
		if v.Name == name {
			return v
		}
	}
	return nil
}

// VariantsWith returns variants matching pred in declaration order.
func (e *Enum) VariantsWith(pred func(*EnumVariant) bool) []*EnumVariant {
	out := make([]*EnumVariant, 0, len(e.Variants))
	for _, v := range e.Variants {
		if pred(v) {
			out = append(out, v)
		}
	}
	return out
}

// MethodByName returns the method named name, or nil when no such
// method exists.
func (e *Enum) MethodByName(name string) *Method {
	for _, m := range e.Methods {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// MethodsWith returns methods matching pred in declaration order.
func (e *Enum) MethodsWith(pred func(*Method) bool) []*Method {
	out := make([]*Method, 0, len(e.Methods))
	for _, m := range e.Methods {
		if pred(m) {
			out = append(out, m)
		}
	}
	return out
}

// HasUnderlying reports whether the enum declares an underlying type.
// Frontends that produce typeless enums leave this nil; downstream
// consumers can detect that and fall back to a default type.
func (e *Enum) HasUnderlying() bool { return e.Underlying != nil }
