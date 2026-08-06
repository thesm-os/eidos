// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"errors"
	"strings"

	"go.thesmos.sh/eidos/emit"
)

// ErrEmptyTarget is returned by [renderFile] when a Target carries
// no declarations and its [emit.File] (if present) has no slot
// contributions either. Empty Targets are filtered before render
// — no sink write, no manifest entry. The error lets the caller
// (the per-Target render loop in [Backend.Render]) distinguish
// the empty case from a render failure and skip silently rather
// than surfacing a diagnostic.
var ErrEmptyTarget = errors.New("golang: target is empty after pre-render")

// fileFor returns the [emit.File] within entities for the supplied
// Target — at most one per Target by the [store.EmitView.FileFor]
// invariant. Returns nil when no File is routed to the Target
// (typical for tests that emit decls directly without an enclosing
// File entity).
func fileFor(entities []emit.Node, target emit.Target) *emit.File {
	for _, n := range entities {
		f, ok := n.(*emit.File)
		if !ok {
			continue
		}
		if f.Target() == target {
			return f
		}
	}
	return nil
}

// declEntities returns entities with any [emit.File] filtered out —
// the rendering-loop input for the layout-item-6 free-floating
// decls. Caller orders the returned slice; the order it arrives in
// is the store's insertion order, which is already plugin-topo
// order at the AddPackage call granularity.
func declEntities(entities []emit.Node) []emit.Node {
	out := make([]emit.Node, 0, len(entities))
	for _, n := range entities {
		if _, ok := n.(*emit.File); ok {
			continue
		}
		out = append(out, n)
	}
	return out
}

// preRenderImports drains every import contribution attached to the
// File before the body templates fire. Explicit aliases on
// [emit.File.Imports] register first (so the alias request reaches
// [writer.ImportSet] before the path is first imported); the
// [emit.File.ImportsSlot] cross-cutting contributions follow,
// re-grouped by plugin-topo order so concurrent contributions
// resolve aliases deterministically.
//
// Alias collisions caught at registration surface as Warn
// diagnostics — the alias retries with the suffix discipline. Imp
// errors (e.g. empty path) surface to the caller; the per-Target
// render loop converts them into Error diagnostics.
//
// Registration is not emission. Every entry drained here is
// unconditional and completes before a single body byte is written,
// so nothing ties an entry to a reference — which makes this the
// main path capable of stranding an import. [pruneImports] drops
// the ones the rendered body never names, so an entry reaching the
// set is necessary for it to reach the import block and not
// sufficient.
func (s *renderState) preRenderImports(file *emit.File) error {
	if file == nil {
		return nil
	}
	for _, imp := range file.Imports {
		if imp.Alias != "" {
			// First-write-wins on alias; later collisions resolve
			// through Imp's suffix loop, so we tolerate the error.
			_ = s.imports.Alias(imp.Path, imp.Alias)
		}
		if _, err := s.imports.Imp(imp.Path); err != nil {
			return err
		}
	}
	slot := file.ImportsSlot()
	items, _ := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	for _, item := range items {
		imp, ok := item.(*emit.Import)
		if !ok {
			continue
		}
		if imp.Alias != "" {
			_ = s.imports.Alias(imp.Path, imp.Alias)
		}
		if _, err := s.imports.Imp(imp.Path); err != nil {
			return err
		}
	}
	return nil
}

// renderFileSlots returns the rendered (top, init, bottom) text for
// the supplied File. Each segment is empty when the corresponding
// slot has no contributions — empty segments are dropped before
// concatenation so they introduce no whitespace.
//
// Top items render via the kind-template dispatcher
// ([renderState.render]); they may be any Node kind (struct, alias,
// constant, etc.). Bottom items follow the same path. Init
// statements compose into a single `func init() { … }` body in
// topo + append order; the entire init block is omitted when the
// slot is empty.
func (s *renderState) renderFileSlots(file *emit.File) (top, initBlock, bottom string, err error) {
	if file == nil {
		return "", "", "", nil
	}
	top, err = s.renderFileNodeSlot(file.Top())
	if err != nil {
		return "", "", "", err
	}
	initBlock, err = s.renderInitBlock(file.Init())
	if err != nil {
		return "", "", "", err
	}
	bottom, err = s.renderFileNodeSlot(file.Bottom())
	if err != nil {
		return "", "", "", err
	}
	return top, initBlock, bottom, nil
}

// renderFileNodeSlot dispatches each item in slot through the
// kind-template renderer and concatenates the result with the same
// "\n\n" separator the free-floating-decl loop uses. Empty input
// returns the empty string.
func (s *renderState) renderFileNodeSlot(slot *emit.Slot) (string, error) {
	if slot.Len() == 0 {
		return "", nil
	}
	items, _ := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	var b strings.Builder
	for _, item := range items {
		// Written through rather than via render's string: the
		// result is appended here and dropped.
		if err := s.renderInto(&b, item); err != nil {
			return "", err
		}
		b.WriteString("\n\n")
	}
	return b.String(), nil
}

// renderInitBlock composes the file's init() body from the supplied
// init slot. Each item renders either through the statement
// dispatcher (typed [emit.Stmt] entries) or through the kind-template
// dispatcher (plugin-defined emit kinds shipping a single-statement
// template). Empty input returns the empty string; the caller
// concatenates an empty segment unchanged so no `func init() {}`
// artifact slips into the output.
func (s *renderState) renderInitBlock(slot *emit.Slot) (string, error) {
	if slot.Len() == 0 {
		return "", nil
	}
	items, _ := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	var body strings.Builder
	for _, item := range items {
		rendered, err := s.renderInitItem(item)
		if err != nil {
			return "", err
		}
		body.WriteString(rendered)
		body.WriteByte('\n')
	}
	return "func init() {\n" + body.String() + "}\n", nil
}

// renderInitItem renders one entry of [emit.File.Init]: a typed
// [emit.Stmt] dispatches through [renderState.renderStmt]; a
// plugin-defined emit kind dispatches through the kind-template
// renderer so its `registrygen.registration` template (or similar)
// produces the one-line form the init-block layout expects.
func (s *renderState) renderInitItem(item emit.Node) (string, error) {
	if stmt, ok := item.(*emit.Stmt); ok {
		return s.renderStmt(stmt)
	}
	return s.render(item)
}
