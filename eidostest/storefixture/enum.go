// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// EnumBuilder configures a [node.Enum] within a [Builder]'s
// accumulating package. Variants are appended in declaration order;
// each variant's [node.EnumVariant.Owner] back-pointer is wired
// automatically.
type EnumBuilder struct {
	e       *node.Enum
	pkgPath string
	// file is the synthetic source file [Builder.Enum] stamped on the
	// enum. Variants and methods inherit it verbatim, for the same
	// reason [StructBuilder]'s members do: a frontend records every
	// member of a declaration at the file the declaration was parsed
	// from, and Layout routes from whichever of them a plugin picks
	// as its origin.
	//
	// Read at call time from the value the parent computed, never
	// from b.e.SourcePos — a Pos call inside the callback would
	// otherwise make a member's file depend on whether it was
	// declared before or after it.
	file string
}

// Node returns the underlying [node.Enum].
func (b *EnumBuilder) Node() *node.Enum { return b.e }

// Pos overrides the enum's source position. Layout derives the
// basename of any file generated from this node from Pos.File, so
// the value decides the output filename; the fixture's synthetic
// `<pkg>/<lowercased-name>.go` keeps that basename non-empty. See
// [StructBuilder.Pos] for what an empty one costs.
func (b *EnumBuilder) Pos(p position.Pos) *EnumBuilder {
	b.e.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *EnumBuilder) Docs(lines ...string) *EnumBuilder {
	b.e.DocLines = append(b.e.DocLines, lines...)
	return b
}

// Directive attaches d to the enum's directive list.
func (b *EnumBuilder) Directive(d *directive.Directive) *EnumBuilder {
	b.e.DirectiveList = append(b.e.DirectiveList, d)
	return b
}

// Underlying records the enum's underlying type. Leave unset for
// typeless enums; downstream consumers can detect the absence and
// fall back to a default type.
func (b *EnumBuilder) Underlying(t *node.TypeRef) *EnumBuilder {
	b.e.Underlying = t
	return b
}

// Variant appends a variant, running each supplied callback against
// it. The variant's Owner back-pointer is wired automatically. Pass
// an empty value when the variant has no declared value (e.g.
// languages where variants are unit-only).
//
// The callback is variadic rather than a required parameter because
// most variants need none, and because that keeps every existing
// two-argument call compiling. Its reason for existing is the
// per-variant directive: a text override is authored on the variant,
// and until now no fixture could spell one — so the highest-precedence
// layer of the rule that decides a variant's textual form was
// reachable only through a real frontend.
func (b *EnumBuilder) Variant(name, value string, fn ...func(*VariantBuilder)) *EnumBuilder {
	v := &node.EnumVariant{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: b.file}},
		Name:     name,
		Value:    value,
		Owner:    b.e,
	}
	vb := &VariantBuilder{v: v}
	for _, configure := range fn {
		if configure != nil {
			configure(vb)
		}
	}
	b.e.Variants = append(b.e.Variants, v)
	return b
}

// Method declares a method on the enum's type — the hook
// [StructBuilder] and [InterfaceBuilder] already carried and this one
// did not.
//
// Worth having because a generator's first question about an enum is
// often which of the conventional methods the author already wrote:
// an existing `String` is what stops a generator shadowing it, and a
// fixture that could not declare one could not reach that branch.
func (b *EnumBuilder) Method(name string, fn func(*MethodBuilder)) *EnumBuilder {
	m := &node.Method{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: b.file}},
		Name:     name,
		Receiver: PkgNamed(b.pkgPath, b.e.Name),
		Owner:    b.e,
	}
	mb := &MethodBuilder{m: m}
	if fn != nil {
		fn(mb)
	}
	b.e.Methods = append(b.e.Methods, m)
	return b
}

// VariantBuilder configures one [node.EnumVariant] within an
// enclosing enum.
type VariantBuilder struct {
	v *node.EnumVariant
}

// Node returns the underlying [node.EnumVariant].
func (b *VariantBuilder) Node() *node.EnumVariant { return b.v }

// Pos overrides the variant's source position, which it otherwise
// inherits from the enum.
func (b *VariantBuilder) Pos(p position.Pos) *VariantBuilder {
	b.v.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *VariantBuilder) Docs(lines ...string) *VariantBuilder {
	b.v.DocLines = append(b.v.DocLines, lines...)
	return b
}

// Directive attaches d to the variant's directive list.
//
// The reason [VariantBuilder] exists. A per-variant text override is
// the highest-precedence layer of the rule deciding what a variant
// renders as, and it is authored on the variant — so a fixture unable
// to attach one left that layer testable only through a frontend
// run, which is where every other layer is already covered.
func (b *VariantBuilder) Directive(d *directive.Directive) *VariantBuilder {
	b.v.DirectiveList = append(b.v.DirectiveList, d)
	return b
}
