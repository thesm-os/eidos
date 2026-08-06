// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store

import (
	"fmt"
	"sync"
	"sync/atomic"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// EmitView is the output-side view onto the [Store]. Generators
// populate it during the generator phase via [EmitView.AddPackage];
// later generators and the backend query it through the per-kind
// buckets and cross-cutting indices.
//
// The index layout mirrors [NodeView]: per-kind buckets for typed
// access plus cross-cutting indices for queries that span kinds. An
// additional "by target" index supports the backend's group-by-file
// rendering convention (multiple emit entities sharing a [emit.Target]
// compose into the same output file).
//
// Methods are safe for concurrent use.
type EmitView struct {
	packages     *Bucket[*emit.Package]
	files        *Bucket[*emit.File]
	imports      *Bucket[*emit.Import]
	structs      *Bucket[*emit.Struct]
	interfaces   *Bucket[*emit.Interface]
	methods      *Bucket[*emit.Method]
	fields       *Bucket[*emit.Field]
	functions    *Bucket[*emit.Function]
	variables    *Bucket[*emit.Variable]
	constants    *Bucket[*emit.Constant]
	enums        *Bucket[*emit.Enum]
	enumVariants *Bucket[*emit.EnumVariant]
	aliases      *Bucket[*emit.Alias]

	byPackage   *MultiIndex[string, emit.Node]
	byDirective *MultiIndex[directive.Name, emit.Node]
	byTarget    *MultiIndex[emit.Target, emit.Node]
	byMetaKey   *MultiIndex[string, emit.Node]

	pendingOriginSlots   []PendingOriginSlot
	pendingOriginSlotsMu sync.Mutex

	frozen    atomic.Bool
	fileForMu sync.Mutex
	addPkgMu  sync.Mutex
}

// newEmitView constructs an empty EmitView with all buckets and
// indices ready for use.
func newEmitView() *EmitView {
	return &EmitView{
		packages:     NewBucket[*emit.Package](),
		files:        NewBucket[*emit.File](),
		imports:      NewBucket[*emit.Import](),
		structs:      NewBucket[*emit.Struct](),
		interfaces:   NewBucket[*emit.Interface](),
		methods:      NewBucket[*emit.Method](),
		fields:       NewBucket[*emit.Field](),
		functions:    NewBucket[*emit.Function](),
		variables:    NewBucket[*emit.Variable](),
		constants:    NewBucket[*emit.Constant](),
		enums:        NewBucket[*emit.Enum](),
		enumVariants: NewBucket[*emit.EnumVariant](),
		aliases:      NewBucket[*emit.Alias](),
		byPackage:    NewMultiIndex[string, emit.Node](),
		byDirective:  NewMultiIndex[directive.Name, emit.Node](),
		byTarget:     NewMultiIndex[emit.Target, emit.Node](),
		byMetaKey:    NewMultiIndex[string, emit.Node](),
	}
}

// AddPackage records p and every declaration it contains in the
// view's per-kind buckets and cross-cutting indices. Returns
// [ErrNilEntry] when p is nil; returns [ErrDuplicateQName] (wrapped
// with the offending qualified name) when any decl entry collides
// with a previously-recorded entry.
//
// When p.Path is already present (e.g. a second generator emitting
// into the same logical Go package), the view merges p's
// declarations into the existing [emit.Package] rather than
// rejecting the duplicate. The decls are appended to the existing
// package's per-kind slices in arrival order; the existing
// package's identity (DocLines, owning fields) is preserved as the
// authoritative record. Indexing of each decl proceeds normally,
// so per-kind QName uniqueness still applies — two plugins trying
// to emit the same `<pkg>.<Name>` decl still surface
// [ErrDuplicateQName] from the bucket layer.
//
// Entries are added in the package's declaration order: files first,
// then imports, then declarations as held by the [emit.Package].
// Each routable declaration is also recorded under its [emit.Target]
// in [EmitView.ByTarget] for backend file grouping.
func (v *EmitView) AddPackage(p *emit.Package) error {
	if p == nil {
		return ErrNilEntry
	}
	if v.frozen.Load() {
		return fmt.Errorf("%w: EmitView (post-generator phase)", ErrFrozen)
	}

	// Serialise the check-then-create on the packages bucket so two
	// concurrent AddPackage calls targeting the same Path resolve
	// deterministically — one creates the host, the other merges
	// into it. The bucket's own locks are too narrow to bridge the
	// ByQName / Add window.
	v.addPkgMu.Lock()
	host, merge := v.packages.ByQName(p.Path)
	if !merge {
		// Add cannot fail here: addPkgMu serialises us against any
		// concurrent AddPackage, and ByQName above confirmed the
		// key is absent.
		_ = v.packages.Add(p.Path, p) //nolint:errcheck // serialised; duplicate impossible
		v.indexCommon(p, p.Path, emit.Target{})
		host = p
	}
	v.addPkgMu.Unlock()

	for _, f := range p.Files {
		if err := v.addFile(f, p.Path); err != nil {
			return err
		}
		if merge {
			host.Files = append(host.Files, f)
		}
	}
	for _, imp := range p.Imports {
		_ = v.imports.Add(imp.Path, imp) //nolint:errcheck // intentional dedup
		v.indexCommon(imp, p.Path, emit.Target{})
		if merge {
			host.Imports = append(host.Imports, imp)
		}
	}
	for _, s := range p.Structs {
		if err := v.addStruct(s, p.Path); err != nil {
			return err
		}
		if merge {
			host.Structs = append(host.Structs, s)
		}
	}
	for _, i := range p.Interfaces {
		if err := v.addInterface(i, p.Path); err != nil {
			return err
		}
		if merge {
			host.Interfaces = append(host.Interfaces, i)
		}
	}
	for _, fn := range p.Functions {
		if err := v.addFunction(fn, p.Path); err != nil {
			return err
		}
		if merge {
			host.Functions = append(host.Functions, fn)
		}
	}
	for _, m := range p.Methods {
		// Top-level methods compose their bucket qname from the
		// owner's QName + the method's Name — same shape as
		// addMethod uses for nested methods. The empty Target is
		// the routing layer's responsibility; RebuildByTarget
		// re-indexes top-level methods after composeTarget stamps
		// the resolved Target field.
		ownerQName := m.OwnerQName()
		if ownerQName == "" {
			ownerQName = p.Path
		}
		if err := v.addMethod(m, ownerQName, p.Path, emit.Target{}); err != nil {
			return err
		}
		if merge {
			host.Methods = append(host.Methods, m)
		}
	}
	for _, vd := range p.Variables {
		if err := v.addVariable(vd, p.Path); err != nil {
			return err
		}
		if merge {
			host.Variables = append(host.Variables, vd)
		}
	}
	for _, c := range p.Constants {
		if err := v.addConstant(c, p.Path); err != nil {
			return err
		}
		if merge {
			host.Constants = append(host.Constants, c)
		}
	}
	for _, e := range p.Enums {
		if err := v.addEnum(e, p.Path); err != nil {
			return err
		}
		if merge {
			host.Enums = append(host.Enums, e)
		}
	}
	for _, a := range p.Aliases {
		if err := v.addAlias(a, p.Path); err != nil {
			return err
		}
		if merge {
			host.Aliases = append(host.Aliases, a)
		}
	}
	return nil
}

// indexCommon updates the cross-cutting indices for n.
// A zero-value [emit.Target] skips the by-target indexing — callers
// pass the entity's Target when it is routable, or the zero value
// for entities (Package, Import) that route alongside their owner.
// The meta-key index is seeded from any keys already set on the
// entity's [meta.Bag] and kept current via an [meta.Observer]
// registered for future Set calls.
func (v *EmitView) indexCommon(n emit.Node, pkgPath string, target emit.Target) {
	v.byPackage.Add(pkgPath, n)
	for _, d := range n.Directives() {
		v.byDirective.Add(d.Name, n)
	}
	if !target.IsZero() {
		v.byTarget.Add(target, n)
	}
	bag := n.EnsureMeta()
	for _, name := range bag.Names() {
		v.byMetaKey.Add(name, n)
	}
	bag.AddObserver(func(name string) {
		v.byMetaKey.Add(name, n)
	})
}

func (v *EmitView) addFile(f *emit.File, pkgPath string) error {
	if err := v.files.Add(f.Path(), f); err != nil {
		return err
	}
	v.indexCommon(f, pkgPath, f.Target())
	for _, imp := range f.Imports {
		_ = v.imports.Add(imp.Path, imp) //nolint:errcheck // intentional dedup
		v.indexCommon(imp, pkgPath, emit.Target{})
	}
	return nil
}

func (v *EmitView) addStruct(s *emit.Struct, pkgPath string) error {
	qname := s.QName()
	if err := v.structs.Add(qname, s); err != nil {
		return err
	}
	v.indexCommon(s, pkgPath, s.Target)
	for _, f := range s.Fields {
		if err := v.fields.Add(qname+"."+f.Name, f); err != nil {
			return err
		}
		v.indexCommon(f, pkgPath, emit.Target{})
	}
	for _, m := range s.Methods {
		// Struct methods are rendered inline through the struct
		// template alongside any slot contributions — they index
		// into the methods bucket for query access but bypass
		// [byTarget] so the backend doesn't double-render them.
		// Same pattern interface methods follow.
		if err := v.addMethod(m, qname, pkgPath, emit.Target{}); err != nil {
			return err
		}
	}
	return nil
}

func (v *EmitView) addInterface(i *emit.Interface, pkgPath string) error {
	qname := i.QName()
	if err := v.interfaces.Add(qname, i); err != nil {
		return err
	}
	v.indexCommon(i, pkgPath, i.Target)
	for _, m := range i.Methods {
		// Interface methods are nested signatures rendered inside
		// the interface's template, not standalone file-scope
		// decls — they index into the methods bucket for query
		// access but bypass [byTarget] so the backend doesn't
		// double-render them. Struct methods, in contrast, are
		// file-scope decls and route through their owning struct's
		// Target.
		if err := v.addMethod(m, qname, pkgPath, emit.Target{}); err != nil {
			return err
		}
	}
	return nil
}

func (v *EmitView) addMethod(m *emit.Method, ownerQName, pkgPath string, target emit.Target) error {
	qname := ownerQName + "." + m.Name
	if err := v.methods.Add(qname, m); err != nil {
		return err
	}
	v.indexCommon(m, pkgPath, target)
	return nil
}

func (v *EmitView) addFunction(f *emit.Function, pkgPath string) error {
	if err := v.functions.Add(f.QName(), f); err != nil {
		return err
	}
	v.indexCommon(f, pkgPath, f.Target)
	return nil
}

func (v *EmitView) addVariable(vd *emit.Variable, pkgPath string) error {
	if err := v.variables.Add(vd.QName(), vd); err != nil {
		return err
	}
	v.indexCommon(vd, pkgPath, vd.Target)
	return nil
}

func (v *EmitView) addConstant(c *emit.Constant, pkgPath string) error {
	if err := v.constants.Add(c.QName(), c); err != nil {
		return err
	}
	v.indexCommon(c, pkgPath, c.Target)
	return nil
}

func (v *EmitView) addEnum(e *emit.Enum, pkgPath string) error {
	qname := e.QName()
	if err := v.enums.Add(qname, e); err != nil {
		return err
	}
	v.indexCommon(e, pkgPath, e.Target)
	for _, vt := range e.Variants {
		if err := v.enumVariants.Add(qname+"."+vt.Name, vt); err != nil {
			return err
		}
		// Enum variants are rendered inline inside their owning
		// enum's `const ( ... )` block — they index into the
		// variants bucket for query access but bypass [byTarget]
		// so the backend doesn't double-render them. Same pattern
		// interface methods follow.
		v.indexCommon(vt, pkgPath, emit.Target{})
	}
	return nil
}

func (v *EmitView) addAlias(a *emit.Alias, pkgPath string) error {
	if err := v.aliases.Add(a.QName(), a); err != nil {
		return err
	}
	v.indexCommon(a, pkgPath, a.File)
	return nil
}

// Packages returns the per-kind bucket for [emit.Package].
func (v *EmitView) Packages() *Bucket[*emit.Package] { return v.packages }

// Files returns the per-kind bucket for [emit.File]. Files are keyed
// by their full path ("<dir>/<name>").
func (v *EmitView) Files() *Bucket[*emit.File] { return v.files }

// Imports returns the per-kind bucket for [emit.Import]. Imports
// dedup by package path; the first appearance wins the bucket entry.
func (v *EmitView) Imports() *Bucket[*emit.Import] { return v.imports }

// Structs returns the per-kind bucket for [emit.Struct].
func (v *EmitView) Structs() *Bucket[*emit.Struct] { return v.structs }

// Interfaces returns the per-kind bucket for [emit.Interface].
func (v *EmitView) Interfaces() *Bucket[*emit.Interface] { return v.interfaces }

// Methods returns the per-kind bucket for [emit.Method]. Methods are
// keyed by "<owner-qname>.<method-name>".
func (v *EmitView) Methods() *Bucket[*emit.Method] { return v.methods }

// Fields returns the per-kind bucket for [emit.Field]. Fields are
// keyed by "<struct-qname>.<field-name>".
func (v *EmitView) Fields() *Bucket[*emit.Field] { return v.fields }

// Functions returns the per-kind bucket for [emit.Function].
func (v *EmitView) Functions() *Bucket[*emit.Function] { return v.functions }

// Variables returns the per-kind bucket for [emit.Variable].
func (v *EmitView) Variables() *Bucket[*emit.Variable] { return v.variables }

// Constants returns the per-kind bucket for [emit.Constant].
func (v *EmitView) Constants() *Bucket[*emit.Constant] { return v.constants }

// Enums returns the per-kind bucket for [emit.Enum].
func (v *EmitView) Enums() *Bucket[*emit.Enum] { return v.enums }

// EnumVariants returns the per-kind bucket for [emit.EnumVariant].
// Variants are keyed by "<enum-qname>.<variant-name>".
func (v *EmitView) EnumVariants() *Bucket[*emit.EnumVariant] { return v.enumVariants }

// Aliases returns the per-kind bucket for [emit.Alias].
func (v *EmitView) Aliases() *Bucket[*emit.Alias] { return v.aliases }

// ByPackage returns the cross-cutting "by declaring package" index.
func (v *EmitView) ByPackage() *MultiIndex[string, emit.Node] { return v.byPackage }

// ByDirective returns the cross-cutting "by directive presence"
// index.
func (v *EmitView) ByDirective() *MultiIndex[directive.Name, emit.Node] { return v.byDirective }

// ByTarget returns the cross-cutting "by output target" index. The
// backend uses this index to group emit entities into the
// appropriate output files.
func (v *EmitView) ByTarget() *MultiIndex[emit.Target, emit.Node] { return v.byTarget }

// RebuildByTarget discards the byTarget index and reconstructs it
// from the current Target of every routable entity. The pipeline
// router calls this after rewriting Targets so the backend's
// byTarget grouping reflects the resolved routing rather than the
// targets recorded at AddPackage time.
//
// Every [emit.File] is re-indexed by its own Target so backends
// (which look up the File for a Target via the index) continue to
// find File entries the router didn't itself rewrite — File Targets
// are pinned at FileFor-creation time.
func (v *EmitView) RebuildByTarget() {
	// Sized from the file population, which is Target identity: the
	// files bucket is keyed by Dir + "/" + Filename, exactly the
	// fields that distinguish one Target from another.
	v.byTarget = newMultiIndexSize[emit.Target, emit.Node](v.files.Len())
	v.files.Range(func(e *emit.File) bool {
		if t := e.Target(); !t.IsZero() {
			v.byTarget.Add(t, e)
		}
		return true
	})
	v.structs.Range(func(e *emit.Struct) bool {
		if !e.Target.IsZero() {
			v.byTarget.Add(e.Target, e)
		}
		return true
	})
	v.interfaces.Range(func(e *emit.Interface) bool {
		if !e.Target.IsZero() {
			v.byTarget.Add(e.Target, e)
		}
		return true
	})
	v.functions.Range(func(e *emit.Function) bool {
		if !e.Target.IsZero() {
			v.byTarget.Add(e.Target, e)
		}
		return true
	})
	v.methods.Range(func(e *emit.Method) bool {
		// Nested methods carry a zero Target (they ride their
		// owner's routing); only top-level methods on
		// [emit.Package.Methods] receive a non-zero Target from
		// the layout phase, so the filter naturally selects them.
		if !e.Target.IsZero() {
			v.byTarget.Add(e.Target, e)
		}
		return true
	})
	v.variables.Range(func(e *emit.Variable) bool {
		if !e.Target.IsZero() {
			v.byTarget.Add(e.Target, e)
		}
		return true
	})
	v.constants.Range(func(e *emit.Constant) bool {
		if !e.Target.IsZero() {
			v.byTarget.Add(e.Target, e)
		}
		return true
	})
	v.enums.Range(func(e *emit.Enum) bool {
		if !e.Target.IsZero() {
			v.byTarget.Add(e.Target, e)
		}
		return true
	})
	v.aliases.Range(func(e *emit.Alias) bool {
		if !e.File.IsZero() {
			v.byTarget.Add(e.File, e)
		}
		return true
	})
}

// ByMetaKey returns the cross-cutting "by metadata key presence"
// index. Like the node-side equivalent, the index is additive: Set
// operations append (key, entity) pairs and tombstones do not
// remove. Callers needing exact "currently set" semantics combine
// with [meta.Bag.Has].
func (v *EmitView) ByMetaKey() *MultiIndex[string, emit.Node] { return v.byMetaKey }

// Freeze marks the view as immutable: subsequent calls to
// [EmitView.AddPackage] return [ErrFrozen]. The pipeline calls
// Freeze after the generator phase to enforce the spec's
// "emit structure mutable only during generator" contract.
//
// Freeze is idempotent — repeated calls are no-ops.
func (v *EmitView) Freeze() { v.frozen.Store(true) }

// IsFrozen reports whether [EmitView.Freeze] has been called.
func (v *EmitView) IsFrozen() bool { return v.frozen.Load() }

// FileFor returns the [emit.File] routed to target, creating one
// if none exists yet. The "exactly one emit.File per Target"
// invariant is what makes multi-generator file composition safe:
// later generators look up the same File and accumulate into its
// slots rather than each generator creating its own conflicting
// File.
//
// The created File carries the target's Dir / Filename / Package
// fields verbatim; callers append to its slots via the standard
// [emit.File.Top] / [emit.File.Bottom] / [emit.File.Init] /
// [emit.File.ImportsSlot] / [emit.File.Slot] accessors.
//
// FileFor returns [ErrFrozen] when the view is frozen and no File
// exists for the target. Lookups for already-present Files succeed
// regardless of frozen state.
//
// The check-then-create sequence is serialised through a per-view
// mutex so two concurrent goroutines targeting the same Target
// see the same File without a race.
func (v *EmitView) FileFor(target emit.Target) (*emit.File, error) {
	key := target.Dir + "/" + target.Filename
	v.fileForMu.Lock()
	defer v.fileForMu.Unlock()
	if f, ok := v.files.ByQName(key); ok {
		return f, nil
	}
	if v.frozen.Load() {
		return nil, fmt.Errorf("%w: cannot create emit.File for %+v", ErrFrozen, target)
	}
	f := &emit.File{
		Name:       target.Filename,
		Package:    target.Package,
		Dir:        target.Dir,
		ImportPath: target.ImportPath,
	}
	// Add cannot fail: we hold fileForMu and the ByQName above
	// guarantees no concurrent caller inserted under key.
	_ = v.files.Add(key, f) //nolint:errcheck // serialised; duplicate impossible
	v.indexCommon(f, target.Dir, f.Target())
	return f, nil
}

// PendingOriginSlot is one queued contribution to a per-file slot
// anchored to an origin node rather than to a concrete
// [emit.Target]. The Layout phase resolves origin to a Target,
// materialises the matching [emit.File], and appends [Item] to
// the named slot. Until Layout runs, the contribution sits on the
// view's pending list in registration order.
type PendingOriginSlot struct {
	Origin   node.Node
	SlotName string
	Item     emit.Node
	Prov     emit.Provenance
}

// AppendOriginSlot queues item for materialisation into origin's
// resolved [emit.File] under slotName during the Layout phase.
// The call validates synchronously: a nil origin, a nil item, or
// an empty slot name returns a typed error at call time without
// queueing anything. Slot names beyond the documented File slot
// set (top, bottom, init, imports) are accepted — plugin-defined
// emit kinds may declare custom slot names through [emit.File.Slot].
//
// On success the contribution is appended to the pending list in
// registration order; Layout drains the list in plugin-topo order
// across plugins and FIFO of registration within each plugin.
//
// Returns [ErrNilEntry] for nil origin or item, [ErrUnknownSlotName]
// for an empty slot name (wrapped with the offending value), and
// [ErrFrozen] when the view is frozen post-Layout.
func (v *EmitView) AppendOriginSlot(origin node.Node, slotName string, item emit.Node, prov emit.Provenance) error {
	if origin == nil || item == nil {
		return ErrNilEntry
	}
	if slotName == "" {
		return fmt.Errorf("%w: %q", ErrUnknownSlotName, slotName)
	}
	if v.frozen.Load() {
		return fmt.Errorf("%w: EmitView (post-layout phase)", ErrFrozen)
	}
	v.pendingOriginSlotsMu.Lock()
	defer v.pendingOriginSlotsMu.Unlock()
	v.pendingOriginSlots = append(v.pendingOriginSlots, PendingOriginSlot{
		Origin:   origin,
		SlotName: slotName,
		Item:     item,
		Prov:     prov,
	})
	return nil
}

// PendingOriginSlots returns a snapshot of every queued
// origin-anchored slot contribution in registration order. The
// returned slice is a copy; callers may iterate and mutate it
// without affecting the view's pending list. Layout drains the
// list via this accessor at routing time.
func (v *EmitView) PendingOriginSlots() []PendingOriginSlot {
	v.pendingOriginSlotsMu.Lock()
	defer v.pendingOriginSlotsMu.Unlock()
	out := make([]PendingOriginSlot, len(v.pendingOriginSlots))
	copy(out, v.pendingOriginSlots)
	return out
}
