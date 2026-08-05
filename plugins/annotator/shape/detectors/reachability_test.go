// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package detectors_test

import (
	"sort"
	"testing"

	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors"
)

// The umbrella plugin dispatches a callable to the first detector
// that matches, in descending [shape.Detector.Priority] order. A
// detector whose accept set is a subset of a higher-priority one is
// therefore dead: registered, reporting matches when probed
// directly, and never classifying anything.
//
// Neither existing suite catches that. Per-detector tests drive one
// detector in isolation, where a shadowed predicate still returns
// true, and the [detectors.All] tests assert registration properties
// rather than behaviour.
//
// It shipped once. `writer` accepted at most one non-error return
// where `reader` accepted exactly one, making every signature reader
// recognised a strict subset of writer's; running first at the
// higher priority, writer claimed all of them. The harm was the
// stamp that replaced reader's, not the dead rule — writer records
// its parameter as ValueType, so `Get(ctx, id string) (Doc, error)`
// was labelled a writer of `string` with the Doc unrecorded.

// refNamed returns a package-qualified named type reference.
func refNamed(pkg, name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Package: pkg, Name: name}
}

// refBuiltin returns a predeclared type reference. The node IR
// models these as named refs carrying no package.
func refBuiltin(name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Name: name}
}

func refPointer(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: elem}
}

func refSlice(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: elem}
}

func refIterSeq(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{
		TypeKind: node.TypeRefNamed,
		Package:  "iter",
		Name:     "Seq",
		TypeArgs: []*node.TypeRef{elem},
	}
}

// paramSpace returns every parameter list the sweep probes: 0-3
// positional parameters, optionally preceded by a context and
// optionally ending in a variadic.
//
// The variadic variants matter more than they look: `batchreader`
// is reachable only through a variadic sole parameter, so a sweep
// that omits them reports a false shadowing against a detector
// whose predicate is fine.
func paramSpace() [][]*node.Param {
	doc := refNamed("example.com/x", "Doc")
	pool := []*node.TypeRef{
		refBuiltin("string"), refPointer(doc), refNamed("io", "Reader"),
		// Parameters with no equality, which `reader` refuses as
		// keys. Present so that rule is measured rather than assumed.
		refSlice(refBuiltin("string")),
		{TypeKind: node.TypeRefAnonInterface},
	}

	var positional [][]*node.Param
	var grow func(cur []*node.Param, depth int)
	grow = func(cur []*node.Param, depth int) {
		positional = append(positional, append([]*node.Param(nil), cur...))
		if depth == 3 {
			return
		}
		for _, p := range pool {
			grow(append(cur, &node.Param{Name: "a", Type: p}), depth+1)
		}
	}
	grow(nil, 0)

	ctxParam := &node.Param{Name: "ctx", Type: refNamed("context", "Context")}
	var out [][]*node.Param
	for _, withCtx := range []bool{false, true} {
		for _, variadic := range []bool{false, true} {
			for _, base := range positional {
				if variadic && len(base) == 0 {
					continue
				}
				ps := append([]*node.Param(nil), base...)
				if variadic {
					last := *ps[len(ps)-1]
					last.Variadic = true
					ps[len(ps)-1] = &last
				}
				if withCtx {
					ps = append([]*node.Param{ctxParam}, ps...)
				}
				out = append(out, ps)
			}
		}
	}
	return out
}

// returnSpace returns every return list the sweep probes: 0-3
// returns drawn from a pool reaching each predicate the catalog
// discriminates on — pointer, slice, bool, error, and iter.Seq.
func returnSpace() [][]*node.TypeRef {
	doc := refNamed("example.com/x", "Doc")
	pool := []*node.TypeRef{
		refBuiltin("string"), refPointer(doc), refBuiltin("bool"),
		refBuiltin("error"), refIterSeq(refBuiltin("string")), refSlice(refBuiltin("string")),
	}

	var out [][]*node.TypeRef
	var grow func(cur []*node.TypeRef, depth int)
	grow = func(cur []*node.TypeRef, depth int) {
		out = append(out, append([]*node.TypeRef(nil), cur...))
		if depth == 3 {
			return
		}
		for _, r := range pool {
			grow(append(cur, r), depth+1)
		}
	}
	grow(nil, 0)
	return out
}

// sweepDispatch runs every signature past every detector in
// priority order, counting how often each one matches and how often
// it wins. Detectors must already be sorted descending by Priority.
//
// The two counts are deliberately separate: a detector matching
// nothing is misspecified, one matching without ever winning is
// shadowed, and reporting both as "unreachable" sends the reader
// after the wrong bug.
func sweepDispatch(all []shape.Detector) (matches, wins map[string]int) {
	matches = make(map[string]int, len(all))
	wins = make(map[string]int, len(all))

	for _, ps := range paramSpace() {
		for _, rs := range returnSpace() {
			callable := &node.Method{Name: "M", Params: ps, Returns: rs}
			claimed := false
			for _, d := range all {
				detect, ok := d.Detect["golang"]
				if !ok {
					continue
				}
				if _, hit := detect(callable); !hit {
					continue
				}
				matches[d.Name]++
				if !claimed {
					wins[d.Name]++
					claimed = true
				}
			}
		}
	}
	return matches, wins
}

// TestAll_Reachability pins that every shipped detector both
// recognises something and wins the dispatch for it. One subtest
// per detector so a shadowed entry names itself in the failure.
func TestAll_Reachability(t *testing.T) {
	t.Parallel()

	all := detectors.All()
	sort.SliceStable(all, func(i, j int) bool { return all[i].Priority > all[j].Priority })
	matches, wins := sweepDispatch(all)

	for _, d := range all {
		t.Run(d.Name, func(t *testing.T) {
			t.Parallel()
			if matches[d.Name] == 0 {
				t.Fatalf("detector %q (priority %d) matches no signature in the sweep; "+
					"its predicate is unsatisfiable, or the sweep's type pool no longer reaches it",
					d.Name, d.Priority)
			}
			if wins[d.Name] == 0 {
				t.Fatalf("detector %q (priority %d) matches %d signatures but wins none; "+
					"a higher-priority detector accepts every one of them, so this detector is dead",
					d.Name, d.Priority, matches[d.Name])
			}
		})
	}
}
