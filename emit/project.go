// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// The vocabulary a generator carries between a language and its
// templates.
//
// A generator projecting a declaration into something renderable asks
// two questions no neutral core can answer for itself — what shape a
// type has, and what parts of a declaration a constructor can set —
// and renders the answers through templates that must not care which
// language gave them. These types are that middle: named in terms
// every language shares, built by whichever language read the source.
//
// Here rather than in a language package because a language package
// is exactly what must not appear in a neutral core, and here rather
// than in the plugin façade because a language package cannot import
// the façade. Both name these.

// TypeShape classifies a declared type by the structure it has, in
// terms every language shares.
//
// The vocabulary a generator branches on when deciding what a
// declaration owes — a collection owes a way to append, a keyed one a
// way to set an entry, an optional one a way to say "present". Which
// spellings map to which shape is the language's business: Go reaches
// [ShapeOptional] through a pointer, Rust through Option, TypeScript
// through a `?`, and a generator branching on the shape never learns
// which.
type TypeShape string

// The shapes. [ShapeScalar] is the zero value, so a projection that
// never classified reads as "a single value" rather than as an
// unhandled case — the conservative answer, since a scalar owes the
// least.
const (
	// ShapeScalar is a single value with no inner type worth naming.
	// Defined types land here and keep their own spelling.
	ShapeScalar TypeShape = ""

	// ShapeSequence is an ordered run of one element type.
	ShapeSequence TypeShape = "sequence"

	// ShapeBytes is a sequence of bytes, which is a sequence the
	// language can also spell as text.
	//
	// Distinct from [ShapeSequence] because that second spelling is
	// the whole point: a caller holding text should not convert at
	// every call site, and the conversion is available for this
	// element type and no other.
	ShapeBytes TypeShape = "bytes"

	// ShapeMapping is a keyed collection carrying a value per key.
	ShapeMapping TypeShape = "mapping"

	// ShapeSet is a keyed collection carrying membership only.
	//
	// Distinct from [ShapeMapping] because its whole meaning is in its
	// keys: a setter asking for the value asks the caller for the one
	// thing they cannot vary.
	ShapeSet TypeShape = "set"

	// ShapeOptional is a value that may be absent, over one inner
	// type.
	ShapeOptional TypeShape = "optional"
)

// TypeInfo is what a language answers about one declared type.
type TypeInfo struct {
	// Shape is the structure the type has.
	Shape TypeShape

	// Elem is the inner type: a sequence's element, a mapping's value,
	// an optional's contents. Nil for the shapes with none.
	Elem Ref

	// Key is a mapping's or set's key type. Nil otherwise.
	Key Ref
}

// Member is one settable part of a declaration.
//
// Named for what a constructor does with it rather than for how a
// language spells it: a Go struct field and a Go embedded type are
// both members, and a generator setting one does not care which it
// had.
type Member struct {
	// Name is the identifier a setter is named after and a literal
	// keys on.
	Name string

	// Type is the member's declared type, lifted into the renderable
	// form.
	Type Ref

	// Meta is the member's metadata, so a generator can read what an
	// annotator stamped on it — a declared default, a shape, a role.
	// Nil for a member the language synthesised rather than read.
	Meta *meta.Bag

	// Pos is where the member was declared, so a diagnostic about it
	// points at source rather than at the generator.
	Pos position.Pos

	// Source is the declaration the member was read from, nil for a
	// synthesised one.
	//
	// For the caller that needs more than [Member.Meta] and
	// [Member.Pos] — a tag, a doc comment, the declared type before
	// lifting. A generator that needs none of those ignores it, which
	// is why the common facts are promoted rather than reached
	// through here.
	Source *node.Field

	// Promoted reports that the member came from something the
	// language folded into the declaration rather than from a field
	// written on it — a Go embed. A generator that treats the two
	// alike ignores it; one that must set the member as a unit reads
	// it.
	Promoted bool
}
