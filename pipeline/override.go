// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"fmt"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

// MetaDirectiveName is the directive name the pipeline interprets
// as a meta-override. A directive whose [directive.Directive.Name]
// equals MetaDirectiveName on any node is applied to that node's
// [meta.Bag] at [meta.AuthorityDirective]:
//
//   - `+<prefix>:meta KEY=VALUE` sets the key to the parsed value
//   - `+<prefix>:meta KEY` sets the key to its parser's boolean
//     "true" form (works for bool-keyed metadata)
//   - `-<prefix>:meta KEY` tombstones the key (or prefix when the
//     name is not a registered key)
//
// The directive name is fixed at "meta" so the override mechanism
// is uniformly recognisable across prefixes and configurations.
const MetaDirectiveName directive.Name = "meta"

// runDirectiveOverride iterates every node recorded in the store
// and applies any directive with [MetaDirectiveName] as a meta
// mutation at [meta.AuthorityDirective]. Unknown meta key names
// surface as Warn diagnostics so typos are visible without aborting
// the run.
func (p *Pipeline) runDirectiveOverride(s *store.Store) {
	p.logPhaseStart("directive-override", "applying +%s/-%s directives", MetaDirectiveName, MetaDirectiveName)
	ps := p.diag.For("pipeline")
	// The walk invokes plugin-registered meta parsers through
	// SetDirectiveFromString, so plugin-authored code runs here and
	// needs the same containment every other plugin boundary has.
	defer diag.RecoverAs(ps, position.Pos{})
	v := s.Nodes()
	visit := func(n node.Node) {
		for _, d := range n.Directives() {
			if d.Name != MetaDirectiveName {
				continue
			}
			applyMetaDirective(n.Meta(), d, ps)
		}
	}
	rangeNodeView(v, visit)
}

// rangeNodeView invokes fn once per recorded source node across
// every kind tracked by [store.NodeView]. Iteration order within a
// kind is insertion order; across kinds the order matches the
// physical iteration here.
func rangeNodeView(v *store.NodeView, fn func(node.Node)) {
	v.Packages().Range(func(n *node.Package) bool { fn(n); return true })
	v.Files().Range(func(n *node.File) bool { fn(n); return true })
	v.Imports().Range(func(n *node.Import) bool { fn(n); return true })
	v.Structs().Range(func(n *node.Struct) bool { fn(n); return true })
	v.Interfaces().Range(func(n *node.Interface) bool { fn(n); return true })
	v.Methods().Range(func(n *node.Method) bool { fn(n); return true })
	v.Fields().Range(func(n *node.Field) bool { fn(n); return true })
	v.Functions().Range(func(n *node.Function) bool { fn(n); return true })
	v.Variables().Range(func(n *node.Variable) bool { fn(n); return true })
	v.Constants().Range(func(n *node.Constant) bool { fn(n); return true })
	v.Enums().Range(func(n *node.Enum) bool { fn(n); return true })
	v.EnumVariants().Range(func(n *node.EnumVariant) bool { fn(n); return true })
	v.Aliases().Range(func(n *node.Alias) bool { fn(n); return true })
}

// applyMetaDirective interprets one [directive.Directive] as a
// meta-override mutation on bag. Positive directives set values
// (KV form) or set the key to "true" (positional form). Negated
// directives tombstone the named keys; an unregistered name is
// treated as a prefix tombstone so plugin authors can clear groups
// without having to enumerate every leaf.
func applyMetaDirective(bag *meta.Bag, d *directive.Directive, ps *diag.PluginSink) {
	if d.Negated {
		for k := range d.KV {
			tombstoneMeta(bag, k, d.Pos)
		}
		for _, k := range d.Args {
			tombstoneMeta(bag, k, d.Pos)
		}
		return
	}
	for k, v := range d.KV {
		if err := setMeta(bag, k, v, d.Pos); err != nil {
			ps.Warnf(d.Pos, "directive override: %v", err)
		}
	}
	for _, k := range d.Args {
		if err := setMeta(bag, k, "true", d.Pos); err != nil {
			ps.Warnf(d.Pos, "directive override: %v", err)
		}
	}
}

// setMeta resolves name through the global [meta] key registry and
// stamps value at [meta.AuthorityDirective]. Returns a wrapped
// error when the name has no registered key or when the parser
// rejects value.
func setMeta(bag *meta.Bag, name, value string, pos position.Pos) error {
	k, err := meta.Lookup(name)
	if err != nil {
		return fmt.Errorf("unknown meta key %q: %w", name, err)
	}
	if err := k.SetDirectiveFromString(bag, value, pos); err != nil {
		return fmt.Errorf("parse %q=%q: %w", name, value, err)
	}
	return nil
}

// tombstoneMeta tombstones name on bag at AuthorityDirective. When
// name is a registered key the tombstone is exact; when name is not
// registered the tombstone is recorded as a prefix so descendants
// of name resolve to "not set" via the [meta.Bag] prefix-walk rule.
func tombstoneMeta(bag *meta.Bag, name string, pos position.Pos) {
	if k, err := meta.Lookup(name); err == nil {
		k.TombstoneDirective(bag, pos)
		return
	}
	bag.TombstonePrefix(name, meta.AuthorityDirective, "directive", pos)
}
