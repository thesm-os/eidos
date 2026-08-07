// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strings"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// The type-parameter helpers, keyed on the parameters rather than
// on the declaration that holds them.
//
// Five node kinds carry a type-parameter list — [node.Struct],
// [node.Interface], [node.Function], [node.Method] and
// [node.Alias] — and the rendering is identical for all five. Keyed
// on the container, a consumer generating over interfaces cannot
// reuse the struct-shaped helper and writes its own, which is how
// one downstream generator ended up with three copies of this and
// two packages each exporting an `Args` for different forms.
//
// The container-shaped entry points remain as thin wrappers, since
// they are published API and the struct case is the common one.

// TypeParamsOf returns the type-parameter list a declaration
// carries, or nil when it carries none or is not a generic-capable
// kind.
//
// The type switch is the whole implementation: the model has no
// common accessor for the field, and adding one would put a method
// on five kinds for a question only a generator asks.
func TypeParamsOf(n node.Node) []*node.TypeParam {
	switch v := n.(type) {
	case *node.Struct:
		return v.TypeParams
	case *node.Interface:
		return v.TypeParams
	case *node.Function:
		return v.TypeParams
	case *node.Method:
		return v.TypeParams
	case *node.Alias:
		return v.TypeParams
	default:
		return nil
	}
}

// IsGeneric reports whether a declaration carries type parameters.
//
// Worth asking directly because the answer changes what a generator
// may emit at all: a generic type cannot be referenced bare, and a
// concrete entry point driving one must instantiate it, since a Go
// test function cannot take type parameters.
func IsGeneric(n node.Node) bool { return len(TypeParamsOf(n)) > 0 }

// TypeParamDecls lifts a type-parameter list into the
// [emit.TypeParam] form the backend's `renderTypeParams` entry
// consumes — the declaration spelling, `[K comparable, V any]`.
//
// Constraint conversion runs through [ConstraintFromNode] so an
// external constraint type registers its import on the rendered
// file automatically. Returns nil for an empty list so the
// template's call emits no bracket list at all; an empty-but-non-nil
// slice would render `[]`, which does not compile.
func TypeParamDecls(params []*node.TypeParam) []*emit.TypeParam {
	if len(params) == 0 {
		return nil
	}
	out := make([]*emit.TypeParam, len(params))
	for i, p := range params {
		out[i] = &emit.TypeParam{
			Name:       p.Name,
			Constraint: ConstraintFromNode(p.Constraint),
		}
	}
	return out
}

// TypeParamNames renders a type-parameter list in its use
// spelling — `[K, V]` — or the empty string for a non-generic
// declaration.
//
// The second of Go's three spellings, alongside [TypeParamDecls]
// (declaration) and [Instantiation] (concrete). A generic type
// referenced without it does not compile; a declaration rendered
// with it declares nothing.
func TypeParamNames(params []*node.TypeParam) string {
	if len(params) == 0 {
		return ""
	}
	names := make([]string, len(params))
	for i, p := range params {
		names[i] = p.Name
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// TypeParamRefs lifts a type-parameter list into the [emit.Ref]
// arguments an instantiation of the declaring type takes — the
// values behind `Container[T, K]`.
//
// Each parameter renders as a bare identifier, which is what a
// reference inside the declaration's own scope needs: the parameter
// is in scope there, so it names itself rather than resolving to a
// package-qualified type.
func TypeParamRefs(params []*node.TypeParam) []emit.Ref {
	if len(params) == 0 {
		return nil
	}
	out := make([]emit.Ref, len(params))
	for i, p := range params {
		out[i] = emit.Builtin(p.Name)
	}
	return out
}

// SelfRef returns the reference a declaration uses to name its own
// type — `Container` when non-generic, `Container[T]` when generic.
//
// The generic form is what an emitted helper referring to its host
// needs: a generic type referenced bare does not compile, and a
// helper that named one would fail at the point of use rather than
// where the generator made the choice.
//
// Same-package elision in the Go backend's `renderType` drops the
// qualifier when the rendered file lives in the declaring package,
// so a caller passes the package unconditionally.
func SelfRef(pkg, name string, params []*node.TypeParam) emit.Ref {
	return emit.External(pkg, name, TypeParamRefs(params)...)
}
