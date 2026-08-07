// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import "strings"

// Declares reports whether the method set contains one of the given
// name.
//
// Takes the method list rather than the declaration holding it,
// because a struct, an interface and an enum all carry one and the
// question is the same for each. Keying on the container is what
// leaves a consumer with a private copy per kind, and the copies
// then disagree about an edge case nobody re-derives on the second
// attempt.
func Declares(methods []*Method, name string) bool {
	return MethodByName(methods, name) != nil
}

// MethodByName returns the named method from the set, or nil.
func MethodByName(methods []*Method, name string) *Method {
	if name == "" {
		return nil
	}
	for _, m := range methods {
		if m != nil && m.Name == name {
			return m
		}
	}
	return nil
}

// PointerReceiver reports whether the named method is declared on a
// pointer receiver, which decides whether a caller writes `&T{}` or
// `T{}`.
//
// False when the method is absent, so a caller that has not checked
// [Declares] gets the value form rather than a claim about a method
// that is not there. The value form is the safer default: it fails
// to compile against a pointer-receiver method set, which is a
// loud, locatable error, whereas the pointer form silently compiles
// against either.
func PointerReceiver(methods []*Method, name string) bool {
	m := MethodByName(methods, name)
	return m != nil && m.Receiver != nil && m.Receiver.IsPointer()
}

// FieldOfType returns the first exported field of s whose type is
// the named builtin, or nil when it has none.
//
// First rather than only: a type carrying two fields of one type
// has no answer this can read as "which one did you mean", and a
// caller wanting a particular one has to say so itself.
func FieldOfType(s *Struct, builtin string) *Field {
	if s == nil || builtin == "" {
		return nil
	}
	for _, f := range s.Fields {
		if f == nil || f.Type == nil || !f.Type.IsBuiltin() {
			continue
		}
		if f.Type.Name == builtin && IsExportedName(f.Name) {
			return f
		}
	}
	return nil
}

// IsExportedName reports whether an identifier is exported under
// the leading-upper-case convention Go and its peers share.
//
// Byte-wise rather than rune-wise on purpose: the rule is about the
// ASCII case of the first character, and a leading multi-byte rune
// is not exported under it.
func IsExportedName(name string) bool {
	return name != "" && name[0] >= 'A' && name[0] <= 'Z'
}

// EmbedName returns the identifier an embedded type contributes as
// a field name, and whether it was embedded by pointer.
//
// An embed by pointer carries its name on the pointee rather than
// on the reference itself, so reading the reference's own name
// yields the empty string and the whole field is silently dropped
// from anything derived from it. Unwrapping the pointer is the
// difference between a generated type that mirrors its source and
// one that quietly omits a field.
func EmbedName(e *Embed) (name string, byPointer bool) {
	if e == nil || e.Type == nil {
		return "", false
	}
	t := e.Type
	if t.IsPointer() {
		if t.Elem == nil {
			return "", true
		}
		return t.Elem.Name, true
	}
	return t.Name, false
}

// LocalName returns the trailing identifier of a possibly-qualified
// name, which is what a caller composing a call expression needs.
//
// Resolution rewrites a sibling reference a directive names — a
// contract's partner, a mixin's `fn` — from the identifier the
// author wrote into the qualified `<pkg-path>.<Type>.<Method>` form
// the store keys on. That form is what makes the reference
// unambiguous across packages, and it is not what a call expression
// can use.
//
// A name that could not be resolved is left as the author wrote it
// and reported as a diagnostic, so an unqualified argument is
// returned unchanged rather than treated as an error here: the run
// that produced it has already said so, and failing twice for one
// cause helps nobody.
func LocalName(qualified string) string {
	if i := strings.LastIndexByte(qualified, '.'); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}
