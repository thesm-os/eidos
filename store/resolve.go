// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store

import "go.thesmos.sh/eidos/node"

// Resolve returns the declaration a type reference names, and whether
// this run loaded one.
//
// The store-backed answer to the question `lang/golang` asks through
// its `Resolver` port. That package declares the port because it must
// stay free of the store, and hangs a substantial part of its surface
// off it — `UnderlyingOf`, `ComparableDeep`, `SampleFor`,
// `ZeroLiteralFor`, `FieldSet`, `PromotedFields`, `PromotedMethods`,
// `ExportedFieldSet`, `EmbedsType`. Until this method existed nothing
// in the tree supplied one, so every consumer holding a real graph had
// to write the adapter before it could call any of them, and the
// functions went unexercised against real data.
//
// # Which declarations answer
//
// Aliases, structs, interfaces and enums — the four kinds a named type
// reference can denote. Aliases carry the defined-type case, which is
// what makes `Weekday` resolvable to the integer behind it.
//
// # Scope
//
// Looked up by qualified name, bypassing the scope predicate, for the
// same reason [Reader.MethodSet] does: a run narrowed by `-target`
// still has to resolve cross-references, and reporting an in-graph
// type as unloaded would make a generator's output depend on the
// user's filter rather than on the source.
//
// # Reads
//
// Each bucket consulted is recorded, so a plugin whose output depends
// on a resolved type carries that in its fingerprint and a warm cache
// cannot serve output predating the edit. Recorded before the lookup
// rather than after, so a miss is recorded too — a type added later
// changes the answer and must invalidate.
//
// A reference with no name resolves to nothing rather than to whatever
// sits under the empty qualified name.
func (r *Reader) Resolve(t *node.TypeRef) (node.Node, bool) {
	if t == nil || t.Name == "" {
		return nil, false
	}
	qname := t.Name
	if t.Package != "" {
		qname = t.Package + "." + t.Name
	}
	nodes := r.store.Nodes()

	// Ordered by how often each answers, so the common case records
	// one read rather than four. A qualified name denotes at most one
	// declaration in valid Go, so the order cannot change the answer.
	if a, ok := lookup(r, "node:alias:", qname, nodes.Aliases()); ok {
		return a, true
	}
	if s, ok := lookup(r, "node:struct:", qname, nodes.Structs()); ok {
		return s, true
	}
	if i, ok := lookup(r, "node:interface:", qname, nodes.Interfaces()); ok {
		return i, true
	}
	if e, ok := lookup(r, "node:enum:", qname, nodes.Enums()); ok {
		return e, true
	}
	return nil, false
}

// PackageAt returns the package with the given import path.
//
// The packages bucket is already keyed by path, so this is a map hit
// where the alternative — filtering [Reader.Packages] — walks every
// package in the run. A generator asking once per declaration turns
// that into a quadratic scan over a graph that already has the index.
func (r *Reader) PackageAt(path string) (*node.Package, bool) {
	r.reads.Record("node:package:" + path)
	return r.store.Nodes().Packages().ByQName(path)
}

// FileAt returns the file with the given path.
//
// Keyed the same way as [Reader.PackageAt], and wanted for the same
// reason: resolving a qualifier to an import means reading the imports
// of the file a declaration was written in, and finding that file by
// scanning is the shape a plugin reaches for when no accessor exists.
//
// Paths are as the frontend recorded them. A caller holding a
// [position.Pos] takes its file path rather than reconstructing one.
func (r *Reader) FileAt(path string) (*node.File, bool) {
	r.reads.Record("node:file:" + path)
	return r.store.Nodes().Files().ByQName(path)
}

// lookup records the read for one bucket and returns its hit as a
// [node.Node].
//
// Generic over the bucket's element so the four call sites in
// [Reader.Resolve] stay one line each; the conversion to the interface
// is the whole reason it cannot be inlined as a method value.
func lookup[T node.Node](r *Reader, prefix, qname string, b *Bucket[T]) (node.Node, bool) {
	r.reads.Record(prefix + qname)
	got, ok := b.ByQName(qname)
	if !ok {
		return nil, false
	}
	return got, true
}
