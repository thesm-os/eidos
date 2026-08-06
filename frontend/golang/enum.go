// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"maps"
	"slices"

	"go.thesmos.sh/eidos/node"
)

// detectEnums coalesces groups of typed constants that share the
// same in-package underlying named type into [node.Enum]
// declarations. The pattern matches the idiomatic Go enum form:
//
//	type Status int
//	const (
//	    StatusActive   Status = iota
//	    StatusInactive
//	)
//
// Constants without a typed name (untyped literals, package-private
// constants of basic types) stay as [node.Constant] entries.
// Constants whose type is not declared in the same package also
// stay as [node.Constant] entries — they don't form an
// in-package enum.
func (c *converter) detectEnums() {
	// Index alias declarations by qualified name so we can route
	// constants to their underlying named type without re-walking
	// the package.
	aliasByName := map[string]*node.Alias{}
	for _, a := range c.out.Aliases {
		aliasByName[a.QName()] = a
	}

	// In-package types of const declarations are always aliases —
	// Go grammar forbids const declarations of struct or interface
	// type — so any in-package ref.Name necessarily appears in
	// aliasByName.
	groups := map[string][]*node.Constant{}
	keepConstants := make([]*node.Constant, 0, len(c.out.Constants))
	for _, cst := range c.out.Constants {
		ref := cst.Type
		if ref == nil || ref.Package != c.out.Path {
			keepConstants = append(keepConstants, cst)
			continue
		}
		groups[c.out.Path+"."+ref.Name] = append(groups[c.out.Path+"."+ref.Name], cst)
	}

	// Promote in qname-sorted order so c.out.Enums is byte-identical
	// across runs even when the underlying group map randomises.
	for _, qname := range slices.Sorted(maps.Keys(groups)) {
		alias := aliasByName[qname]
		c.promoteAliasToEnum(alias, groups[qname])
	}
	c.out.Constants = keepConstants
}

// promoteAliasToEnum converts an [node.Alias] + a set of associated
// [node.Constant] entries into a [node.Enum]. The alias is removed
// from the package's Aliases slice (it has been subsumed); the
// resulting Enum carries the alias's underlying type as
// [node.Enum.Underlying] and its method set as [node.Enum.Methods].
//
// Carrying the methods across is not cosmetic. The alias is the only
// node that held them, and removeAlias deletes it, so a method set
// left behind here is not relocated — it is discarded, with no
// diagnostic. The same `type Status int` kept its methods right up
// until a const block turned it into an enum.
//
// The methods' Owner back-pointers are not re-pointed here. They
// still name the alias at this point, and [node.RewireOwners] — which
// the loader runs over every package after conversion — re-attaches
// them to the Enum via rewireEnum. Doing it in both places would put
// the same invariant in two spots that can disagree.
func (c *converter) promoteAliasToEnum(alias *node.Alias, members []*node.Constant) {
	enum := &node.Enum{
		BaseNode:   alias.BaseNode,
		Name:       alias.Name,
		Package:    alias.Package,
		Underlying: alias.Target,
		Methods:    alias.Methods,
	}
	for _, m := range members {
		variant := &node.EnumVariant{
			BaseNode: m.BaseNode,
			Name:     m.Name,
			Value:    m.Value,
		}
		c.stampVariantMeta(variant, m)
		enum.Variants = append(enum.Variants, variant)
	}
	c.out.Enums = append(c.out.Enums, enum)
	c.removeAlias(alias)
}

// removeAlias drops the given alias from the converter's package.
// The alias has been promoted to an enum and no longer represents
// a stand-alone declaration.
func (c *converter) removeAlias(target *node.Alias) {
	kept := c.out.Aliases[:0]
	for _, a := range c.out.Aliases {
		if a == target {
			continue
		}
		kept = append(kept, a)
	}
	c.out.Aliases = kept
	delete(c.aliasByQName, target.QName())
}
