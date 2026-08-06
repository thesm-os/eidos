// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store

import (
	"fmt"
	"sync/atomic"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/node"
)

// NodeView is the source-side view onto the [Store]. Frontends
// populate it during the frontend phase via [NodeView.AddPackage];
// annotators and generators query it during their phases through
// the per-kind buckets and cross-cutting indices.
//
// Each per-kind bucket preserves insertion order for deterministic
// iteration and provides O(1) qualified-name lookup. Cross-cutting
// indices ([NodeView.ByPackage], [NodeView.ByDirective]) collect
// every kind under their respective key for queries that span
// declaration kinds.
//
// Methods are safe for concurrent use.
type NodeView struct {
	packages     *Bucket[*node.Package]
	files        *Bucket[*node.File]
	imports      *Bucket[*node.Import]
	structs      *Bucket[*node.Struct]
	interfaces   *Bucket[*node.Interface]
	methods      *Bucket[*node.Method]
	fields       *Bucket[*node.Field]
	functions    *Bucket[*node.Function]
	variables    *Bucket[*node.Variable]
	constants    *Bucket[*node.Constant]
	enums        *Bucket[*node.Enum]
	enumVariants *Bucket[*node.EnumVariant]
	aliases      *Bucket[*node.Alias]

	byPackage   *MultiIndex[string, node.Node]
	byDirective *MultiIndex[directive.Name, node.Node]
	byMetaKey   *MultiIndex[string, node.Node]

	frozen atomic.Bool
}

// newNodeView constructs an empty NodeView with all buckets and
// indices ready for use.
func newNodeView() *NodeView {
	return &NodeView{
		packages:     NewBucket[*node.Package](),
		files:        NewBucket[*node.File](),
		imports:      NewBucket[*node.Import](),
		structs:      NewBucket[*node.Struct](),
		interfaces:   NewBucket[*node.Interface](),
		methods:      NewBucket[*node.Method](),
		fields:       NewBucket[*node.Field](),
		functions:    NewBucket[*node.Function](),
		variables:    NewBucket[*node.Variable](),
		constants:    NewBucket[*node.Constant](),
		enums:        NewBucket[*node.Enum](),
		enumVariants: NewBucket[*node.EnumVariant](),
		aliases:      NewBucket[*node.Alias](),
		byPackage:    NewMultiIndex[string, node.Node](),
		byDirective:  NewMultiIndex[directive.Name, node.Node](),
		byMetaKey:    NewMultiIndex[string, node.Node](),
	}
}

// AddPackage records p and every declaration it contains in the
// view's per-kind buckets and cross-cutting indices. Returns
// [ErrNilEntry] when p is nil; returns [ErrDuplicateQName] (wrapped
// with the offending qualified name) when any entry collides with a
// previously-recorded entry.
//
// Entries are added in the package's source order: files first, then
// imports, then declarations in the order the [node.Package] holds
// them. Within each kind, the bucket preserves insertion order so
// later iteration is deterministic.
func (v *NodeView) AddPackage(p *node.Package) error {
	if p == nil {
		return ErrNilEntry
	}
	if v.frozen.Load() {
		return fmt.Errorf("%w: NodeView (post-frontend phase)", ErrFrozen)
	}

	if err := v.packages.Add(p.Path, p); err != nil {
		return err
	}
	v.indexCommon(p, p.Path)

	for _, f := range p.Files {
		if err := v.addFile(f, p.Path); err != nil {
			return err
		}
	}
	for _, imp := range p.Imports {
		// Imports may repeat across files declaring the same path;
		// the import bucket dedups and we tolerate the duplicate.
		_ = v.imports.Add(imp.Path, imp) //nolint:errcheck,gosec // intentional dedup
		v.indexCommon(imp, p.Path)
	}
	for _, s := range p.Structs {
		if err := v.addStruct(s, p.Path); err != nil {
			return err
		}
	}
	for _, i := range p.Interfaces {
		if err := v.addInterface(i, p.Path); err != nil {
			return err
		}
	}
	for _, fn := range p.Functions {
		if err := v.addFunction(fn, p.Path); err != nil {
			return err
		}
	}
	for _, vd := range p.Variables {
		if err := v.addVariable(vd, p.Path); err != nil {
			return err
		}
	}
	for _, c := range p.Constants {
		if err := v.addConstant(c, p.Path); err != nil {
			return err
		}
	}
	for _, e := range p.Enums {
		if err := v.addEnum(e, p.Path); err != nil {
			return err
		}
	}
	for _, a := range p.Aliases {
		if err := v.addAlias(a, p.Path); err != nil {
			return err
		}
	}
	return nil
}

// indexCommon updates the cross-cutting [NodeView.byPackage],
// [NodeView.byDirective], and [NodeView.byMetaKey] indices for n.
// The meta-key index is seeded from any keys already set on the
// node's [meta.Bag] at the time of indexing, then kept current via
// an [meta.Observer] that fires on every future Set against the
// same bag.
func (v *NodeView) indexCommon(n node.Node, pkgPath string) {
	v.byPackage.Add(pkgPath, n)
	for _, d := range n.Directives() {
		v.byDirective.Add(d.Name, n)
	}
	bag := n.EnsureMeta()
	for _, name := range bag.Names() {
		v.byMetaKey.Add(name, n)
	}
	bag.AddObserver(func(name string) {
		v.byMetaKey.Add(name, n)
	})
}

func (v *NodeView) addFile(f *node.File, pkgPath string) error {
	if err := v.files.Add(f.Path, f); err != nil {
		return err
	}
	v.indexCommon(f, pkgPath)
	for _, imp := range f.Imports {
		// Per-file imports may repeat across files; the package-level
		// import bucket dedups by path, so skip the file-level
		// re-adds when the bucket already contains the path.
		_ = v.imports.Add(imp.Path, imp) //nolint:errcheck // intentional dedup
		v.indexCommon(imp, pkgPath)
	}
	return nil
}

func (v *NodeView) addStruct(s *node.Struct, pkgPath string) error {
	qname := s.QName()
	if err := v.structs.Add(qname, s); err != nil {
		return err
	}
	v.indexCommon(s, pkgPath)
	for _, f := range s.Fields {
		if err := v.fields.Add(qname+"."+f.Name, f); err != nil {
			return err
		}
		v.indexCommon(f, pkgPath)
	}
	for _, m := range s.Methods {
		if err := v.addMethod(m, qname, pkgPath); err != nil {
			return err
		}
	}
	return nil
}

func (v *NodeView) addInterface(i *node.Interface, pkgPath string) error {
	qname := i.QName()
	if err := v.interfaces.Add(qname, i); err != nil {
		return err
	}
	v.indexCommon(i, pkgPath)
	for _, m := range i.Methods {
		if err := v.addMethod(m, qname, pkgPath); err != nil {
			return err
		}
	}
	return nil
}

func (v *NodeView) addMethod(m *node.Method, ownerQName, pkgPath string) error {
	qname := fmt.Sprintf("%s.%s", ownerQName, m.Name)
	if err := v.methods.Add(qname, m); err != nil {
		return err
	}
	v.indexCommon(m, pkgPath)
	return nil
}

func (v *NodeView) addFunction(f *node.Function, pkgPath string) error {
	if err := v.functions.Add(f.QName(), f); err != nil {
		return err
	}
	v.indexCommon(f, pkgPath)
	return nil
}

func (v *NodeView) addVariable(vd *node.Variable, pkgPath string) error {
	if err := v.variables.Add(vd.QName(), vd); err != nil {
		return err
	}
	v.indexCommon(vd, pkgPath)
	return nil
}

func (v *NodeView) addConstant(c *node.Constant, pkgPath string) error {
	if err := v.constants.Add(c.QName(), c); err != nil {
		return err
	}
	v.indexCommon(c, pkgPath)
	return nil
}

func (v *NodeView) addEnum(e *node.Enum, pkgPath string) error {
	qname := e.QName()
	if err := v.enums.Add(qname, e); err != nil {
		return err
	}
	v.indexCommon(e, pkgPath)
	for _, vt := range e.Variants {
		if err := v.enumVariants.Add(qname+"."+vt.Name, vt); err != nil {
			return err
		}
		v.indexCommon(vt, pkgPath)
	}
	return nil
}

func (v *NodeView) addAlias(a *node.Alias, pkgPath string) error {
	if err := v.aliases.Add(a.QName(), a); err != nil {
		return err
	}
	v.indexCommon(a, pkgPath)
	return nil
}

// Packages returns the per-kind bucket for [node.Package].
func (v *NodeView) Packages() *Bucket[*node.Package] { return v.packages }

// Files returns the per-kind bucket for [node.File].
func (v *NodeView) Files() *Bucket[*node.File] { return v.files }

// Imports returns the per-kind bucket for [node.Import]. Imports
// dedup by package path; the first file declaring a path wins the
// bucket entry, later files share the same bucket value.
func (v *NodeView) Imports() *Bucket[*node.Import] { return v.imports }

// Structs returns the per-kind bucket for [node.Struct].
func (v *NodeView) Structs() *Bucket[*node.Struct] { return v.structs }

// Interfaces returns the per-kind bucket for [node.Interface].
func (v *NodeView) Interfaces() *Bucket[*node.Interface] { return v.interfaces }

// Methods returns the per-kind bucket for [node.Method]. Methods are
// keyed by "<owner-qname>.<method-name>".
func (v *NodeView) Methods() *Bucket[*node.Method] { return v.methods }

// Fields returns the per-kind bucket for [node.Field]. Fields are
// keyed by "<struct-qname>.<field-name>".
func (v *NodeView) Fields() *Bucket[*node.Field] { return v.fields }

// Functions returns the per-kind bucket for [node.Function].
func (v *NodeView) Functions() *Bucket[*node.Function] { return v.functions }

// Variables returns the per-kind bucket for [node.Variable].
func (v *NodeView) Variables() *Bucket[*node.Variable] { return v.variables }

// Constants returns the per-kind bucket for [node.Constant].
func (v *NodeView) Constants() *Bucket[*node.Constant] { return v.constants }

// Enums returns the per-kind bucket for [node.Enum].
func (v *NodeView) Enums() *Bucket[*node.Enum] { return v.enums }

// EnumVariants returns the per-kind bucket for [node.EnumVariant].
// Variants are keyed by "<enum-qname>.<variant-name>".
func (v *NodeView) EnumVariants() *Bucket[*node.EnumVariant] { return v.enumVariants }

// Aliases returns the per-kind bucket for [node.Alias].
func (v *NodeView) Aliases() *Bucket[*node.Alias] { return v.aliases }

// ByPackage returns the cross-cutting "by declaring package" index.
// Lookups by package path return every node — across kinds — that
// the frontend recorded as belonging to that package.
func (v *NodeView) ByPackage() *MultiIndex[string, node.Node] { return v.byPackage }

// ByDirective returns the cross-cutting "by directive presence"
// index. Lookups by directive name return every node carrying that
// directive in the order the frontend recorded.
func (v *NodeView) ByDirective() *MultiIndex[directive.Name, node.Node] { return v.byDirective }

// ByMetaKey returns the cross-cutting "by metadata key presence"
// index. The index is additive — it records every (key, node) pair
// observed via [meta.Bag.AddObserver]. Tombstones do not remove
// entries; queries that need exact "currently set" semantics
// combine the index with [meta.Bag.Has] checks per node. The same
// node can appear under a key multiple times if the key has been
// re-Set; deduplication is the caller's concern when relevant.
func (v *NodeView) ByMetaKey() *MultiIndex[string, node.Node] { return v.byMetaKey }

// Freeze marks the view as immutable: subsequent calls to
// [NodeView.AddPackage] return [ErrFrozen]. The pipeline calls
// Freeze after the frontend phase to enforce the spec's
// "node structure mutable only during frontend" contract.
//
// Freeze is idempotent — repeated calls are no-ops. Concurrent
// callers see a consistent post-freeze state once any one of them
// returns.
func (v *NodeView) Freeze() { v.frozen.Store(true) }

// IsFrozen reports whether [NodeView.Freeze] has been called.
func (v *NodeView) IsFrozen() bool { return v.frozen.Load() }
