// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package detectors_test

import (
	"sort"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors"
	"go.thesmos.sh/eidos/sdk"
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
func refNamed(pkg, name string) *sdk.TypeRef {
	return &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Package: pkg, Name: name}
}

// refBuiltin returns a predeclared type reference. The node IR
// models these as named refs carrying no package.
func refBuiltin(name string) *sdk.TypeRef {
	return &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: name}
}

func refPointer(elem *sdk.TypeRef) *sdk.TypeRef {
	return &sdk.TypeRef{TypeKind: sdk.TypeRefPointer, Elem: elem}
}

func refSlice(elem *sdk.TypeRef) *sdk.TypeRef {
	return &sdk.TypeRef{TypeKind: sdk.TypeRefSlice, Elem: elem}
}

func refIterSeq(elem *sdk.TypeRef) *sdk.TypeRef {
	return &sdk.TypeRef{
		TypeKind: sdk.TypeRefNamed,
		Package:  "iter",
		Name:     "Seq",
		TypeArgs: []*sdk.TypeRef{elem},
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
func paramSpace() [][]*sdk.Param {
	doc := refNamed("example.com/x", "Doc")
	pool := []*sdk.TypeRef{
		refBuiltin("string"), refPointer(doc), refNamed("io", "Reader"),
		// Parameters with no equality, which `reader` refuses as
		// keys. Present so that rule is measured rather than assumed.
		refSlice(refBuiltin("string")),
		{TypeKind: sdk.TypeRefAnonInterface},
	}

	var positional [][]*sdk.Param
	var grow func(cur []*sdk.Param, depth int)
	grow = func(cur []*sdk.Param, depth int) {
		positional = append(positional, append([]*sdk.Param(nil), cur...))
		if depth == 3 {
			return
		}
		for _, p := range pool {
			grow(append(cur, &sdk.Param{Name: "a", Type: p}), depth+1)
		}
	}
	grow(nil, 0)

	ctxParam := &sdk.Param{Name: "ctx", Type: refNamed("context", "Context")}
	var out [][]*sdk.Param
	for _, withCtx := range []bool{false, true} {
		for _, variadic := range []bool{false, true} {
			for _, base := range positional {
				if variadic && len(base) == 0 {
					continue
				}
				ps := append([]*sdk.Param(nil), base...)
				if variadic {
					last := *ps[len(ps)-1]
					last.Variadic = true
					ps[len(ps)-1] = &last
				}
				if withCtx {
					ps = append([]*sdk.Param{ctxParam}, ps...)
				}
				out = append(out, ps)
			}
		}
	}
	return out
}

// returnSpace returns every return list the sweep probes: 0-3
// returns drawn from a pool reaching each predicate the catalog
// discriminates on — pointer, slice, bool, error, iter.Seq, and a
// package-qualified named type.
//
// The named entry matches one in the parameter pool on purpose:
// answeringwriter discriminates on the parameter's type equalling the
// first result's, so a pool whose two halves share no named type
// cannot reach it.
func returnSpace() [][]*sdk.TypeRef {
	doc := refNamed("example.com/x", "Doc")
	pool := []*sdk.TypeRef{
		refBuiltin("string"), refPointer(doc), refBuiltin("bool"),
		refBuiltin("error"), refIterSeq(refBuiltin("string")), refSlice(refBuiltin("string")),
		refNamed("io", "Reader"),
	}

	var out [][]*sdk.TypeRef
	var grow func(cur []*sdk.TypeRef, depth int)
	grow = func(cur []*sdk.TypeRef, depth int) {
		out = append(out, append([]*sdk.TypeRef(nil), cur...))
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

	for _, name := range nameSpace() {
		for _, ps := range paramSpace() {
			for _, rs := range returnSpace() {
				callable := &sdk.Method{Name: name, Params: ps, Returns: sdk.AnonReturns(rs...)}
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
	}
	return matches, wins
}

// nameSpace returns the callable names the sweep probes.
//
// Every other axis varies the signature, because most detectors read
// only the signature. The name-gated pair are the exception —
// [closer] separates a teardown from a poison probe, and [deleter] a
// removal from a write, each pair being identical as shapes — so a
// sweep holding one name constant could never reach them, and would
// report their predicates unsatisfiable rather than its pool too
// narrow.
//
// One entry per gate plus one no gate claims, which is the whole
// discrimination. The unclaimed name also keeps [poisonaccessor] and
// [writer] reachable, since a claimed name would otherwise take every
// signature it shares with them.
func nameSpace() []string {
	return []string{"M", "Close", "Delete"}
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
