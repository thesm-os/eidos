// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store

import "go.thesmos.sh/eidos/node"

// MethodSet returns i's full method set, resolving embedded
// interfaces against the graph this reader holds.
//
// The walk itself is [node.MethodSet]; this supplies the resolver.
// A generator reading [node.Interface.Methods] directly reads what
// the source typed rather than what the interface has, and the
// difference is invisible until the generated code fails to
// compile — a double missing an embedded method does not satisfy
// the interface it doubles.
//
// Resolution is against the packages this run loaded, so the answer
// depends on the invocation and not only on the source: a run over
// one package cannot see an interface embedded from another it
// never read. Those embeds report [node.ReasonUnresolved] rather
// than being dropped, so a caller can say so instead of emitting a
// short method set as though it were complete.
func (r *Reader) MethodSet(i *node.Interface) node.MethodSetResult {
	return node.MethodSet(i, r.resolveInterface)
}

// resolveInterface answers [node.InterfaceResolver] from the node
// buckets.
//
// The two-result contract matters: found=false means this run never
// loaded the reference, which is legitimate for a narrow run;
// found=true with a nil interface means the name resolved to a
// declaration that is not an interface, which is a source defect.
// Collapsing them would report a user's broken embed as a
// scope problem and send them looking in the wrong place.
func (r *Reader) resolveInterface(t *node.TypeRef) (*node.Interface, bool) {
	// No nil guard on t: [node.MethodSet] filters an embed carrying
	// no type before it reaches a resolver, so the branch would be
	// unreachable from the only caller.
	qname := t.Name
	if t.Package != "" {
		qname = t.Package + "." + t.Name
	}
	// Looked up by qualified name, which bypasses the scope
	// predicate deliberately: a narrowed run still has to resolve
	// cross-references, and silently reporting an in-graph embed as
	// unloaded would make the method set depend on the user's
	// -target filter.
	nodes := r.store.Nodes()
	// Recorded because the resolved interface is an input to
	// whatever the caller emits: a plugin whose output changes when
	// an embedded interface changes must carry that in its
	// fingerprint, or a warm cache serves output that predates the
	// edit.
	r.reads.Record("node:interface:" + qname)
	if iface, ok := nodes.Interfaces().ByQName(qname); ok {
		return iface, true
	}
	// Present under another kind means the embed names a real
	// declaration that cannot be embedded in an interface.
	for _, present := range []func(string) bool{
		func(q string) bool { _, ok := nodes.Structs().ByQName(q); return ok },
		func(q string) bool { _, ok := nodes.Enums().ByQName(q); return ok },
		func(q string) bool { _, ok := nodes.Aliases().ByQName(q); return ok },
	} {
		if present(qname) {
			return nil, true
		}
	}
	return nil, false
}

// Implementers returns every struct in the graph whose method set
// satisfies i.
//
// What a double or a suite is checked against: a generator emitting
// assertions about an interface needs the concrete types a consumer
// actually implements it with, and deriving that from the graph
// beats asking the author to list them in a directive that goes
// stale.
//
// Satisfaction is by name and receiver-independent — a method is
// counted whether declared on the value or the pointer. The model
// carries no parameter-level type identity strong enough to compare
// signatures across packages, so two same-named methods of
// different shapes count as a match here. That is the permissive
// answer on purpose: a false positive surfaces as a compile error
// naming the type, while a false negative silently omits a type the
// author expected to see.
//
// An interface whose own method set could not be resolved returns
// nil rather than matching everything: every struct trivially
// satisfies an empty set, and answering "all of them" for an
// interface this run failed to read is worse than answering
// nothing.
func (r *Reader) Implementers(i *node.Interface) []*node.Struct {
	set := r.MethodSet(i)
	if !set.OK() || len(set.Methods) == 0 {
		return nil
	}
	var out []*node.Struct
	// Scoped, unlike the embed resolution above: this is a range
	// query, and a run narrowed to one target should report the
	// implementers within it rather than the whole graph's.
	for _, s := range r.Structs().Slice() {
		if satisfies(s, set.Methods) {
			out = append(out, s)
		}
	}
	return out
}

// satisfies reports whether s declares a method for every entry in
// the required set.
func satisfies(s *node.Struct, required []*node.Method) bool {
	// required comes from a resolved MethodSet, which skips nil and
	// unnamed entries, so no guard is needed here.
	for _, m := range required {
		if !node.Declares(s.Methods, m.Name) {
			return false
		}
	}
	return true
}
