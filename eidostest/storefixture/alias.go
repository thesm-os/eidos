// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// AliasBuilder configures a [node.Alias] within a [Builder]'s
// accumulating package.
type AliasBuilder struct {
	a       *node.Alias
	pkgPath string
	file    string
}

// Node returns the underlying [node.Alias].
func (b *AliasBuilder) Node() *node.Alias { return b.a }

// Pos overrides the alias's source position. Layout derives the
// basename of any file generated from this node from Pos.File, so
// the value decides the output filename; the fixture's synthetic
// `<pkg>/<lowercased-name>.go` keeps that basename non-empty. See
// [StructBuilder.Pos] for what an empty one costs.
func (b *AliasBuilder) Pos(p position.Pos) *AliasBuilder {
	b.a.SourcePos = p
	return b
}

// Docs appends doc-comment lines.
func (b *AliasBuilder) Docs(lines ...string) *AliasBuilder {
	b.a.DocLines = append(b.a.DocLines, lines...)
	return b
}

// Directive attaches d to the alias's directive list.
func (b *AliasBuilder) Directive(d *directive.Directive) *AliasBuilder {
	b.a.DirectiveList = append(b.a.DirectiveList, d)
	return b
}

// Target records the type the alias refers to.
func (b *AliasBuilder) Target(t *node.TypeRef) *AliasBuilder {
	b.a.Target = t
	return b
}

// True marks the declaration as a true type alias (`type X = Y`)
// rather than a new named type (`type X Y`). The default — without
// calling True — is the new-named-type form.
func (b *AliasBuilder) True() *AliasBuilder {
	b.a.IsAlias = true
	return b
}

// TypeParam declares a generic type parameter on the alias. Pass
// nil for an implicit "any" bound, or use [Constraint] for an
// explicit named-bound constraint.
func (b *AliasBuilder) TypeParam(name string, constraint *node.Constraint) *AliasBuilder {
	b.a.TypeParams = append(b.a.TypeParams, &node.TypeParam{
		Name:       name,
		Constraint: constraint,
		Owner:      b.a,
	})
	return b
}

// Method declares a method on the named type.
//
// Go attaches methods to any defined type, so `type Weekday int` is
// as legitimate a method carrier as a struct — and the sibling
// [EnumBuilder.Method] exists because coalescing a typed-const group
// into an enum moves those same methods onto the enum. A fixture that
// could build one and not the other left the alias arm of every
// method walk untestable, which is how a walk comes to omit it.
//
// True aliases carry no methods of their own — Go forbids it — so a
// builder marked through [AliasBuilder.True] should not declare one.
// The rule is the language's rather than this package's, and it is
// not enforced here: a fixture deliberately building an impossible
// graph is how a consumer's handling of one gets tested.
func (b *AliasBuilder) Method(name string, fn func(*MethodBuilder)) *AliasBuilder {
	m := &node.Method{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: b.file}},
		Name:     name,
		Receiver: PkgNamed(b.pkgPath, b.a.Name),
		Owner:    b.a,
	}
	mb := &MethodBuilder{m: m}
	if fn != nil {
		fn(mb)
	}
	b.a.Methods = append(b.a.Methods, m)
	return b
}
