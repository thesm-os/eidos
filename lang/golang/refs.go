// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// Spelling a type or a symbol: as a display string for a human, and
// as an [emit.Ref] for the backend.
//
// The two are not interchangeable and conflating them is the
// mistake this file exists to prevent. A display string names a
// type in a diagnostic; a reference names one in generated source
// and carries the import the rendered file has to register. Text
// cannot ask for an import, so a generator that builds a qualified
// name by concatenation emits a file naming a package it never
// imports.

// ErrBadSymbol is returned by [RefForQualified] for a value that is
// neither a bare identifier nor `<import/path>.<Symbol>`.
var ErrBadSymbol = errors.New("golang: malformed symbol reference")

// QName returns the fully-qualified spelling of a type reference —
// `example.com/store.User`, or the bare name for a builtin or an
// in-package type.
//
// The form the shape vocabulary records and consumers read back, so
// a stamp written by one plugin and read by another agrees. Empty
// for nil, so a caller need not guard before interpolating.
func QName(t *node.TypeRef) string {
	if t == nil {
		return ""
	}
	if t.Package == "" {
		return t.Name
	}
	return t.Package + "." + t.Name
}

// Display returns the spelling a source author would recognise —
// `store.User` rather than `example.com/store.User`.
//
// For diagnostics, never for generated source. The last path
// segment is what appears in the author's own file, so a message
// using it names something they can search for; the full path names
// something they wrote once in an import block.
func Display(t *node.TypeRef) string {
	if t == nil {
		return ""
	}
	if t.Package == "" {
		return t.Name
	}
	return path.Base(t.Package) + "." + t.Name
}

// MethodQName composes the store's canonical method key,
// `<ownerQName>.<method>`.
//
// The form the shape resolver rewrites sibling references into, and
// the key a consumer looks a method up by. Composed here so the
// spelling lives once rather than in each package that has to match
// it.
func MethodQName(ownerQName, method string) string {
	if ownerQName == "" {
		return method
	}
	return ownerQName + "." + method
}

// LocalName returns the trailing identifier of a possibly-qualified
// name.
//
// What a generator needs to compose a call on a subject it already
// holds: the resolver rewrites a directive's sibling reference into
// the qualified form that makes it unambiguous across packages, and
// that form is exactly what a call expression cannot use.
//
// A name carrying no qualifier is returned unchanged rather than
// treated as an error — an unresolved reference has already been
// diagnosed by the run that produced it, and failing twice for one
// cause helps nobody.
func LocalName(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// RefFor lifts a type name written by a source author into the
// reference a rendered file can use.
//
// A predeclared name renders bare; anything else is taken to be
// declared in srcPkg and qualified against it. That rule is what
// makes a directive argument usable from a generated file routed
// somewhere else — the backend elides the qualifier where the two
// share a package and registers the import where they do not.
//
// A name that carries its own qualifier is not resolvable here: the
// generator would have to invent the import path. Use
// [RefForQualified] for the notation that supplies one.
func RefFor(name, srcPkg string) emit.Ref {
	if IsPredeclared(name) || srcPkg == "" {
		return emit.Builtin(name)
	}
	return emit.External(srcPkg, name)
}

// RefForQualified lifts a directive value naming a symbol into a
// reference, accepting both notations authors write.
//
//	Validate                     -> resolved against srcPkg
//	example.com/x.Validate       -> a full import path, needing no import
//	gopkg.in/yaml.v3.Marshal     -> also a full path; the dots before
//	                                the last one belong to the path
//
// Split on the last dot, because a path may contain them. The
// second notation exists because an import written only to feed a
// directive is an unused import, which does not compile — without
// it a value can only name a package the file already uses for real
// code.
//
// A leading or trailing dot returns [ErrBadSymbol] rather than a
// reference to the empty string, which would render as a bare `.`
// the consumer's compiler reports against generated code.
func RefForQualified(raw, srcPkg string) (emit.Ref, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: empty", ErrBadSymbol)
	}
	i := strings.LastIndex(raw, ".")
	if i < 0 {
		return RefFor(raw, srcPkg), nil
	}
	if i == 0 || i == len(raw)-1 {
		return nil, fmt.Errorf("%w: %q", ErrBadSymbol, raw)
	}
	return emit.External(raw[:i], raw[i+1:]), nil
}

// RefsOf lifts a list of source types into their emit form.
//
// Returns nil for an empty list so a caller appending the result to
// a signature emits nothing rather than an empty bracket list.
func RefsOf(types []*node.TypeRef) []emit.Ref {
	if len(types) == 0 {
		return nil
	}
	out := make([]emit.Ref, len(types))
	for i, t := range types {
		out[i] = FromNode(t)
	}
	return out
}

// ParamRefs lifts a parameter list's declared types into emit form,
// dropping the names.
//
// What a function type takes: `func(context.Context, string) error`
// names no parameters, so the identifiers a body would bind are not
// part of the type.
func ParamRefs(params []*node.Param) []emit.Ref {
	if len(params) == 0 {
		return nil
	}
	out := make([]emit.Ref, len(params))
	for i, p := range params {
		if p != nil {
			out[i] = FromNode(p.Type)
		}
	}
	return out
}

// ReturnRefs lifts a return list's declared types into emit form.
func ReturnRefs(returns []*node.Return) []emit.Ref {
	if len(returns) == 0 {
		return nil
	}
	out := make([]emit.Ref, len(returns))
	for i, r := range returns {
		if r != nil {
			out[i] = FromNode(r.Type)
		}
	}
	return out
}

// PkgPathOf returns the import path of the package owning n, or
// empty when the node kind carries none.
//
// [node.Node] declares no package accessor — a field and a package
// both satisfy the interface and only one has a path — so a caller
// wanting this has to reach for the concrete kind. The type switch
// is the whole implementation, and it is here rather than in each
// caller because getting it wrong is silent: an assertion against
// an accessor no kind implements compiles, always misses, and
// leaves every reference unqualified.
//
// Empty is the honest answer for a kind that has no package. The
// backend's same-package elision renders an unqualified reference,
// which is correct for a declaration landing beside its source.
func PkgPathOf(n node.Node) string {
	switch v := n.(type) {
	case *node.Package:
		return v.Path
	case *node.Struct:
		return v.Package
	case *node.Interface:
		return v.Package
	case *node.Function:
		return v.Package
	case *node.Alias:
		return v.Package
	case *node.Enum:
		return v.Package
	case *node.Variable:
		return v.Package
	case *node.Constant:
		return v.Package
	case *node.TypeRef:
		return v.Package
	case *node.Method:
		// A method carries no package of its own; its owner does, and
		// walking up is what makes a method usable as an origin.
		return PkgPathOf(v.Owner)
	case *node.Field:
		return PkgPathOf(v.Owner)
	default:
		return ""
	}
}

// SubjectRef names a declaration from wherever generated output
// lands.
//
// Qualified against the origin's own package when it has one, bare
// otherwise: [emit.External] rejects an empty path, so the two
// cases cannot share a construction and every caller that wanted
// this wrote the branch itself.
func SubjectRef(origin node.Node, name string) emit.Ref {
	return RefFor(name, PkgPathOf(origin))
}
