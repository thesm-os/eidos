// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package typescript holds the TypeScript conventions every plugin
// generating or reading TypeScript shares. It is the TypeScript side
// of the per-language adapter pattern.
//
// Conventions only: nothing here parses and nothing here renders.
// Parsing lives in `lang/typescript/frontend`, rendering in
// `lang/typescript/backend`. That division is what lets both sides
// depend on this package, which they must — depguard forbids a
// backend importing a frontend and forbids plugins importing either.
//
// # Surface
//
//   - The `ts.*` metadata vocabulary (meta.go): every key the
//     frontend stamps, the backend reads, and a bridge writes.
//     Declared here rather than in the frontend, because a key is
//     interned by name and a consumer that cannot import the
//     declaring package re-declares it by string instead.
//   - Structural type markers (refs.go): [RefUnion],
//     [RefIntersection], [RefTuple], [RefOperator], with [IsUnion],
//     [IsTuple] and [Members] to read them.
//   - Decorator reading (accessors.go): [Decorators],
//     [DecoratorNamed], [DecoratorsNamed], [HasDecorator]. A
//     decorator is how a TypeScript framework attaches
//     machine-readable metadata to a declaration, and both its order
//     and its repetition carry meaning — hence one ordered list
//     rather than a key per name.
//   - Vocabulary constants (typescript.go): the four visibility
//     levels, the two heritage kinds, the two accessor kinds.
//
// # Why so much rides on metadata
//
// TypeScript's declaration syntax carries modifiers the
// language-agnostic model has no field for — optional, readonly,
// visibility, async — and type shapes it has no variant for: unions,
// intersections, tuples. Promoting any of them to a [node] field
// would put a TypeScript fact in the package every language shares.
// The `go.*` vocabulary makes the same trade for channels and
// type-set constraints.
//
// Three shapes are the exception and stay structural: union,
// intersection and tuple members ride on the marker ref's TypeArgs,
// because they are [node.TypeRef] values and TypeArgs is the field a
// generic walker already descends. Hiding them on a metadata key
// would hide a union's members from every traversal that does not
// know the key.
//
// # Reading
//
//	import "go.thesmos.sh/eidos/lang/typescript"
//
//	if typescript.IsUnion(field.Type) {
//	    for _, m := range typescript.Members(field.Type) { … }
//	}
//	if opt, _ := typescript.MetaOptional.Get(field.Meta()); opt {
//	    // rendered `name?: T`
//	}
//
// Use Meta() for reads — it returns nil for a node nothing has
// stamped and the typed Get handles that. A write needs EnsureMeta(),
// which allocates the bag.
package typescript
