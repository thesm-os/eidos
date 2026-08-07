// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strings"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// Instantiating a generic declaration: choosing concrete types for
// its parameters, and substituting them through a projection.
//
// A Go test function cannot take type parameters, so anything
// generated to exercise a generic type has to name concrete types
// somewhere. Deriving them is only possible where the constraint's
// type set is knowable without loading the package that declares
// it, which is why this answers all-or-nothing and says so.

// WitnessPalette supplies the derived concrete type for each
// position of a type-parameter list.
//
// Positional and pairwise distinct, so a generated helper that
// crossed two parameters produces code that does not compile rather
// than code that type-checks and asserts the wrong thing. A
// declaration with more parameters than there are entries gets no
// derived witness at all — wrapping the list would hand two
// parameters the same type and lose exactly that property.
//
//nolint:gochecknoglobals // immutable positional table
var WitnessPalette = []string{"string", "int", "bool", "float64", "int64", "uint", "uint8", "int32"}

// Witnesses returns one concrete type per parameter, or nil when
// any parameter carries a constraint that cannot be reasoned about.
//
// All-or-nothing because an entry point instantiates the whole list
// at once: a witness for one parameter is worth nothing without one
// for the rest. A nil result is the caller's signal to emit a note
// in place of the checks — there is no way to name the types they
// would run at.
func Witnesses(params []*node.TypeParam) []emit.Ref {
	if len(params) == 0 || len(params) > len(WitnessPalette) {
		return nil
	}
	out := make([]emit.Ref, 0, len(params))
	for i, p := range params {
		if p == nil || !AdmitsAnyBasicType(p.Constraint) {
			return nil
		}
		out = append(out, emit.Builtin(WitnessPalette[i]))
	}
	return out
}

// WitnessNames returns the derived witnesses as bare type names.
func WitnessNames(params []*node.TypeParam) []string {
	refs := Witnesses(params)
	if len(refs) == 0 {
		return nil
	}
	return WitnessPalette[:len(refs)]
}

// WitnessUse renders the derived witnesses in use position —
// `[string, int]` — or the empty string when there are none, so a
// template appends it unconditionally.
//
// Safe to build as text because every witness is a builtin: its
// rendered form is its name, so nothing here names a package the
// rendered file would have to import.
func WitnessUse(params []*node.TypeParam) string {
	names := WitnessNames(params)
	if len(names) == 0 {
		return ""
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// AdmitsAnyBasicType reports whether every entry of
// [WitnessPalette] satisfies the constraint.
//
// The set is closed to the two bounds whose type set is known
// without loading anything. A named constraint —
// `constraints.Ordered`, a project's own interface — is a reference
// into a package the generator never read, so a declaration bounded
// by one takes its witnesses from the source or not at all.
//
// # Why the printed form decides
//
// [node.Constraint.Raw] is authoritative wherever a frontend
// populated it, and the structured predicates are consulted only
// where it is empty. That order is load-bearing rather than
// stylistic: [node.Constraint.IsAny] reports true for any
// constraint carrying no embedded bound, and a Go frontend records
// `constraints.Ordered` with its printed form and no embed — so
// asking the predicate first admits every named constraint in
// existence and derives witnesses that do not satisfy it.
//
// A constraint with no printed form is synthetic — a fixture, a
// programmatically built value — and the predicates are all there
// is to read.
func AdmitsAnyBasicType(c *node.Constraint) bool {
	if c == nil {
		return true
	}
	if raw := strings.TrimSpace(c.Raw); raw != "" {
		switch raw {
		case "any", "interface{}", "comparable":
			return true
		default:
			return false
		}
	}
	return c.IsAny() || c.IsComparable()
}

// SubstituteTypeParams rewrites a reference naming a type parameter
// into the concrete type bound to it.
//
// Recursive, so `[]T` and `map[K]V` are rewritten as well as a bare
// `T`. A parameter with no binding is left as it stands, which
// keeps a partial substitution visible in the output rather than
// silently erased.
//
// Returns a copy of any composite it rewrites; the input is not
// mutated, because a projection is commonly substituted once per
// instantiation and a mutating rewrite would corrupt the second.
func SubstituteTypeParams(r emit.Ref, by map[string]emit.Ref) emit.Ref {
	if r == nil || len(by) == 0 {
		return r
	}
	switch typed := r.(type) {
	case *emit.BuiltinRef:
		// A type parameter reaches the emit layer as a bare name,
		// which is the same shape a builtin takes — the binding map is
		// what tells them apart.
		if bound, ok := by[typed.Name]; ok {
			return bound
		}
		return r
	case *emit.CompositeRef:
		clone := *typed
		clone.Elem = SubstituteTypeParams(typed.Elem, by)
		clone.MapKey = SubstituteTypeParams(typed.MapKey, by)
		clone.MapValue = SubstituteTypeParams(typed.MapValue, by)
		return &clone
	case *emit.ExternalRef:
		if len(typed.TypeArgs) == 0 {
			return r
		}
		clone := *typed
		clone.TypeArgs = substituteAll(typed.TypeArgs, by)
		return &clone
	default:
		return r
	}
}

// substituteAll rewrites every reference in a list.
func substituteAll(refs []emit.Ref, by map[string]emit.Ref) []emit.Ref {
	out := make([]emit.Ref, len(refs))
	for i, r := range refs {
		out[i] = SubstituteTypeParams(r, by)
	}
	return out
}

// WitnessBindings pairs each type parameter with the concrete type
// at its position.
//
// The map [SubstituteTypeParams] consumes. Returns nil when the
// lists disagree in length, because a partial binding would
// substitute some positions and leave others naming a parameter no
// longer in scope.
func WitnessBindings(params []*node.TypeParam, witnesses []emit.Ref) map[string]emit.Ref {
	if len(params) == 0 || len(params) != len(witnesses) {
		return nil
	}
	out := make(map[string]emit.Ref, len(params))
	for i, p := range params {
		if p == nil {
			return nil
		}
		out[p.Name] = witnesses[i]
	}
	return out
}

// SubstituteSig returns a copy of s with every type parameter
// rewritten to its witness.
//
// What makes a generic subject reachable from a non-generic entry
// point: a check naming `T` in a parameter position does not
// compile, and rewriting the projection is enough where the
// generated code names types and nothing else.
//
// Returns s unchanged when there is nothing to substitute, so the
// non-generic path allocates nothing.
func SubstituteSig(s *Sig, witnesses []emit.Ref) *Sig {
	if s == nil {
		return nil
	}
	by := WitnessBindings(s.TypeParams, witnesses)
	if by == nil {
		return s
	}
	out := *s
	out.Params = make([]Param, len(s.Params))
	for i := range s.Params {
		out.Params[i] = s.Params[i]
		out.Params[i].Type = SubstituteTypeParams(s.Params[i].Type, by)
	}
	out.Returns = make([]Return, len(s.Returns))
	for i := range s.Returns {
		out.Returns[i] = s.Returns[i]
		out.Returns[i].Type = SubstituteTypeParams(s.Returns[i].Type, by)
	}
	// The parameters are gone from the rewritten signature: every use
	// of them now names a concrete type, and a declaration still
	// carrying the list would declare parameters nothing mentions.
	out.TypeParams = nil
	return &out
}

// TypeParamDeclsFromEmit lifts an emit-side type-parameter list
// into declaration form.
//
// The emit-side twin of [TypeParamDecls], for a generator consuming
// another's output: the constraint already lives on the emit layer
// and passes through, where the source-side lifter has to convert
// it.
func TypeParamDeclsFromEmit(params []*emit.TypeParam) []*emit.TypeParam {
	if len(params) == 0 {
		return nil
	}
	out := make([]*emit.TypeParam, len(params))
	for i, p := range params {
		if p == nil {
			continue
		}
		out[i] = &emit.TypeParam{Name: p.Name, Constraint: p.Constraint}
	}
	return out
}

// TypeParamRefsFromEmit lifts an emit-side type-parameter list into
// the bare-name arguments an instantiation takes.
func TypeParamRefsFromEmit(params []*emit.TypeParam) []emit.Ref {
	if len(params) == 0 {
		return nil
	}
	out := make([]emit.Ref, len(params))
	for i, p := range params {
		if p != nil {
			out[i] = emit.Builtin(p.Name)
		}
	}
	return out
}

// TypeParamNamesFromEmit renders an emit-side type-parameter list
// in use position — `[K, V]`.
func TypeParamNamesFromEmit(params []*emit.TypeParam) string {
	if len(params) == 0 {
		return ""
	}
	names := make([]string, len(params))
	for i, p := range params {
		if p != nil {
			names[i] = p.Name
		}
	}
	return "[" + strings.Join(names, ", ") + "]"
}
