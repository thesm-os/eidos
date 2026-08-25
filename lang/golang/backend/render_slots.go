// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"fmt"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// ErrDuplicateEntity is the umbrella sentinel for duplicate emit
// values surfaced through the backend. Two slot contributions
// sharing a discriminator (slot-level dedup), or two free-floating
// decls sharing a `QName()` in the same Target (decl-level dedup
// caught at AddPackage time via [store.ErrDuplicateQName]), both
// chain back to this sentinel — consumers match against it via
// [errors.Is] regardless of which layer detected the collision.
//
// Aliased to [store.ErrDuplicateEntity] so the umbrella sentinel
// lives in one place; the alias keeps the spec-named identifier
// available from the backend package surface for plugin authors.
var ErrDuplicateEntity = store.ErrDuplicateEntity

// ErrNilHost is returned by funcmap-exposed render helpers when
// invoked with a nil host pointer. Core templates always pass a
// non-nil dot context, so the sentinel surfaces only when a
// plugin-supplied template calls one of the canonical helpers
// (`renderStructFields`, `renderMethodBody`, …) with a wrong
// receiver — typically a nil dot from an outer-scope misuse. The
// wrapped message names the offending helper and the expected
// host type so the diagnostic points the plugin author at the
// concrete fix.
var ErrNilHost = errors.New("backend: nil host passed to funcmap helper")

// nilHostErrf returns a wrapped [ErrNilHost] naming the offending
// helper and the host type it expected. Centralised so every
// funcmap-exposed render helper emits the same diagnostic shape.
func nilHostErrf(helper, hostType string) error {
	return fmt.Errorf("%w: %s requires a non-nil %s", ErrNilHost, helper, hostType)
}

// typedContentSource is the provenance marker for names recorded
// directly on the host's typed slice (Fields / Methods / Variants),
// distinguishing typed-content collisions from slot-vs-slot
// collisions in [ErrDuplicateEntity] diagnostics.
const typedContentSource = "typed-content"

// pluginOrderFrom returns the plugin-name slice the per-Target
// [renderState] uses to topo-sort slot contributions. Drawn from
// [plugin.BackendContext.Ordered] (the pipeline's resolved
// capability topo); falls back to [plugin.BackendContext.Plugins]
// in registration order when the pipeline did not supply an
// ordering. Returns nil when neither slot is populated — slot
// contributions then render in raw append order.
func pluginOrderFrom(ctx *plugin.BackendContext) []string {
	src := ctx.Ordered
	if len(src) == 0 {
		src = ctx.Plugins
	}
	if len(src) == 0 {
		return nil
	}
	out := make([]string, 0, len(src))
	for _, p := range src {
		out = append(out, p.Name())
	}
	return out
}

// orderByPlugin returns the slot items (along with their parallel
// provenance entries) re-sorted by the supplied plugin name order.
// Within each plugin group the original append order is preserved;
// items whose [emit.Provenance.SetBy] does not match any name in
// order render after the ordered groups, in append order, so an
// untracked contribution still surfaces in the output rather than
// silently dropping. Empty order returns items unchanged.
//
// The two parallel slices share index alignment with [emit.Slot] —
// items[i] was contributed under prov[i].
func orderByPlugin(items []emit.Node, prov []emit.Provenance, order []string) ([]emit.Node, []emit.Provenance) {
	if len(order) == 0 || len(items) == 0 {
		return items, prov
	}
	rank := make(map[string]int, len(order))
	for i, name := range order {
		rank[name] = i
	}
	type entry struct {
		bucket int
		seq    int
		item   emit.Node
		prov   emit.Provenance
	}
	entries := make([]entry, len(items))
	for i := range items {
		bucket := len(order)
		if r, ok := rank[prov[i].SetBy]; ok {
			bucket = r
		}
		entries[i] = entry{bucket: bucket, seq: i, item: items[i], prov: prov[i]}
	}
	// Stable sort by (bucket, seq). Implemented inline (no
	// sort.SliceStable closure churn) to keep the hot path
	// allocation-light.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			if entries[j-1].bucket < entries[j].bucket {
				break
			}
			if entries[j-1].bucket == entries[j].bucket && entries[j-1].seq <= entries[j].seq {
				break
			}
			entries[j-1], entries[j] = entries[j], entries[j-1]
		}
	}
	outItems := make([]emit.Node, len(entries))
	outProv := make([]emit.Provenance, len(entries))
	for i, e := range entries {
		outItems[i] = e.item
		outProv[i] = e.prov
	}
	return outItems, outProv
}

// mergedFields returns the rendering-order [emit.Field] slice for
// host: typed [emit.Struct.Fields] first (the owner generator's
// direct content), then every field appended to the "fields" slot
// by cross-cutters, re-grouped by the renderState's plugin order.
// Returns [ErrDuplicateEntity] when a slot contribution duplicates
// a typed-field or earlier-slot-field name from a different
// provenance.
func (s *renderState) mergedFields(host *emit.Struct) ([]*emit.Field, error) {
	slot := host.FieldsSlot()
	items, prov := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	out := make([]*emit.Field, 0, len(host.Fields)+len(items))
	out = append(out, host.Fields...)
	names := fieldNameSet(host.Fields)
	for i, item := range items {
		f := item.(*emit.Field)
		if prior, exists := names[f.Name]; exists {
			return nil, fmt.Errorf(
				"%w: field %q in struct %s [contributors: %s and %s]",
				ErrDuplicateEntity, f.Name, host.QName(), prior, prov[i],
			)
		}
		names[f.Name] = prov[i].String()
		out = append(out, f)
	}
	return out, nil
}

// mergedMethods returns the rendering-order method slice for the
// host: typed [emit.Struct.Methods] first, then the "methods" slot
// items re-grouped by plugin order. Returns [ErrDuplicateEntity]
// when a slot contribution duplicates a typed-method or earlier-
// slot-method name.
func (s *renderState) mergedMethods(host *emit.Struct) ([]*emit.Method, error) {
	slot := host.MethodsSlot()
	items, prov := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	out := make([]*emit.Method, 0, len(host.Methods)+len(items))
	out = append(out, host.Methods...)
	names := methodNameSet(host.Methods)
	for i, item := range items {
		m := item.(*emit.Method)
		if prior, exists := names[m.Name]; exists {
			return nil, fmt.Errorf(
				"%w: method %q on struct %s [contributors: %s and %s]",
				ErrDuplicateEntity, m.Name, host.QName(), prior, prov[i],
			)
		}
		names[m.Name] = prov[i].String()
		out = append(out, m)
	}
	return out, nil
}

// mergedStructEmbeds returns the typed-embeds + slot-embeds slice
// for a struct, in plugin-topo order. Embed names do not carry a
// QName check (struct-level duplicates are caught by the Go
// compiler with a clear error already).
func (s *renderState) mergedStructEmbeds(host *emit.Struct) []*emit.Embed {
	slot := host.EmbedsSlot()
	items, _ := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	out := make([]*emit.Embed, 0, len(host.Embeds)+len(items))
	out = append(out, host.Embeds...)
	for _, item := range items {
		out = append(out, item.(*emit.Embed))
	}
	return out
}

// mergedInterfaceMethods returns the typed-methods + slot-methods
// slice for an interface, in plugin-topo order with the slot's
// duplicate-name check applied.
func (s *renderState) mergedInterfaceMethods(host *emit.Interface) ([]*emit.Method, error) {
	slot := host.MethodsSlot()
	items, prov := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	out := make([]*emit.Method, 0, len(host.Methods)+len(items))
	out = append(out, host.Methods...)
	names := methodNameSet(host.Methods)
	for i, item := range items {
		m := item.(*emit.Method)
		if prior, exists := names[m.Name]; exists {
			return nil, fmt.Errorf(
				"%w: method %q on interface %s [contributors: %s and %s]",
				ErrDuplicateEntity, m.Name, host.QName(), prior, prov[i],
			)
		}
		names[m.Name] = prov[i].String()
		out = append(out, m)
	}
	return out, nil
}

// mergedInterfaceEmbeds returns the typed + slot embeds slice for
// an interface, in plugin-topo order. No duplicate check —
// duplicate embedded interfaces are a Go compile error caught
// downstream.
func (s *renderState) mergedInterfaceEmbeds(host *emit.Interface) []*emit.Embed {
	slot := host.EmbedsSlot()
	items, _ := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	out := make([]*emit.Embed, 0, len(host.Embeds)+len(items))
	out = append(out, host.Embeds...)
	for _, item := range items {
		out = append(out, item.(*emit.Embed))
	}
	return out
}

// mergedVariants returns the typed + slot variant slice for an
// enum, in plugin-topo order with a duplicate-name check.
func (s *renderState) mergedVariants(host *emit.Enum) ([]*emit.EnumVariant, error) {
	slot := host.VariantsSlot()
	items, prov := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	out := make([]*emit.EnumVariant, 0, len(host.Variants)+len(items))
	out = append(out, host.Variants...)
	names := variantNameSet(host.Variants)
	for i, item := range items {
		v := item.(*emit.EnumVariant)
		if prior, exists := names[v.Name]; exists {
			return nil, fmt.Errorf(
				"%w: variant %q on enum %s [contributors: %s and %s]",
				ErrDuplicateEntity, v.Name, host.QName(), prior, prov[i],
			)
		}
		names[v.Name] = prov[i].String()
		out = append(out, v)
	}
	return out, nil
}

// mergedStmts returns the typed body extended with the supplied
// slot's items, re-grouped by plugin order. Use for Function /
// Method "prebody" + "postbody" composition: prebody contributes
// before the typed body, postbody after.
//
// Returns nil + nil for an empty slot (no allocation).
func (s *renderState) mergedSlotStmts(slot *emit.Slot) []*emit.Stmt {
	if slot.Len() == 0 {
		return nil
	}
	items, _ := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	out := make([]*emit.Stmt, 0, len(items))
	for _, item := range items {
		out = append(out, item.(*emit.Stmt))
	}
	return out
}

// mergedSlotParams returns the additional params contributed via
// the supplied slot, re-grouped by plugin order. Caller appends
// these to the host's typed Params slice.
func (s *renderState) mergedSlotParams(slot *emit.Slot) []*emit.Param {
	if slot.Len() == 0 {
		return nil
	}
	items, _ := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	out := make([]*emit.Param, 0, len(items))
	for _, item := range items {
		out = append(out, item.(*emit.Param))
	}
	return out
}

// renderStructFields is the funcmap-facing wrapper that merges the
// host struct's typed [emit.Struct.Fields] with its
// [emit.Struct.FieldsSlot] contributions (the cross-cutting fields
// other generators inject) and produces the inline field-block
// content for `emit.struct` templates.
//
// `renderStructFields` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderStructFields(host *emit.Struct) (string, error) {
	if host == nil {
		return "", nilHostErrf("renderStructFields", "*emit.Struct")
	}
	merged, err := s.mergedFields(host)
	if err != nil {
		return "", err
	}
	return s.renderFields(merged)
}

// renderStructEmbeds is the funcmap-facing wrapper that merges the
// host's typed [emit.Struct.Embeds] with its
// [emit.Struct.EmbedsSlot] contributions and produces the inline
// embed-block content for `emit.struct` templates.
//
// `renderStructEmbeds` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderStructEmbeds(host *emit.Struct) (string, error) {
	if host == nil {
		return "", nilHostErrf("renderStructEmbeds", "*emit.Struct")
	}
	return s.renderEmbeds(s.mergedStructEmbeds(host))
}

// renderStructMethods returns the merged method slice the struct
// template ranges over to render typed + slot-contributed methods
// as standalone declarations below the struct's body. Struct
// methods are routed inline through this funcmap entry (rather
// than via [byTarget] standalone dispatch) so cross-cutting slot
// contributions and typed declarations render side-by-side in
// deterministic topo order.
//
// `renderStructMethods` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderStructMethods(host *emit.Struct) ([]*emit.Method, error) {
	if host == nil {
		return nil, nilHostErrf("renderStructMethods", "*emit.Struct")
	}
	return s.mergedMethods(host)
}

// renderInterfaceEmbeds is the funcmap-facing wrapper that merges
// the interface's typed embeds with its [emit.Interface.EmbedsSlot]
// contributions and produces the inline embed-block content for
// `emit.interface` templates.
//
// `renderInterfaceEmbeds` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderInterfaceEmbeds(host *emit.Interface) (string, error) {
	if host == nil {
		return "", nilHostErrf("renderInterfaceEmbeds", "*emit.Interface")
	}
	return s.renderEmbeds(s.mergedInterfaceEmbeds(host))
}

// renderInterfaceMethods returns the merged method slice the
// interface template ranges over to render typed +
// slot-contributed methods as inline signatures inside the
// interface body. Returns [ErrDuplicateEntity] when a slot
// contribution duplicates a typed or earlier-slot method name.
//
// `renderInterfaceMethods` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderInterfaceMethods(host *emit.Interface) ([]*emit.Method, error) {
	if host == nil {
		return nil, nilHostErrf("renderInterfaceMethods", "*emit.Interface")
	}
	return s.mergedInterfaceMethods(host)
}

// renderEnumVariants is the funcmap-facing wrapper that merges the
// enum's typed [emit.Enum.Variants] with its
// [emit.Enum.VariantsSlot] contributions and renders the resulting
// sequence through [renderState.renderVariants].
//
// `renderEnumVariants` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderEnumVariants(host *emit.Enum) (string, error) {
	if host == nil {
		return "", nilHostErrf("renderEnumVariants", "*emit.Enum")
	}
	merged, err := s.mergedVariants(host)
	if err != nil {
		return "", err
	}
	// renderVariants reads e.Variants; pass a shallow copy with
	// the merged slice substituted so the cross-cutting items
	// reach the rendering path without mutating the host.
	clone := *host
	clone.Variants = merged
	return s.renderVariants(&clone)
}

// renderFunctionBody returns the merged statement body for a
// function — prebody slot contributions first, then the typed
// [emit.Function.Body], then postbody slot contributions. Each
// slot's items are pre-grouped by plugin topo order via
// [renderState.mergedSlotStmts]. The result feeds the function
// template's body block through [renderState.renderStmtBlock].
//
// `renderFunctionBody` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderFunctionBody(host *emit.Function) (string, error) {
	if host == nil {
		return "", nilHostErrf("renderFunctionBody", "*emit.Function")
	}
	body := composeBody(s.mergedSlotStmts(host.Prebody()), host.Body, s.mergedSlotStmts(host.Postbody()))
	return s.renderStmtBlock(body)
}

// renderMethodBody returns the merged statement body for a method
// — prebody slot contributions, the typed [emit.Method.Body],
// then postbody slot contributions, sequenced just like
// [renderState.renderFunctionBody].
//
// `renderMethodBody` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderMethodBody(host *emit.Method) (string, error) {
	if host == nil {
		return "", nilHostErrf("renderMethodBody", "*emit.Method")
	}
	body := composeBody(s.mergedSlotStmts(host.Prebody()), host.Body, s.mergedSlotStmts(host.Postbody()))
	return s.renderStmtBlock(body)
}

// renderFunctionParams returns the rendered parenthesised parameter
// list for a function — typed [emit.Function.Params] followed by
// every cross-cutting parameter contributed through
// [emit.Function.ParamsSlot], re-grouped by plugin topo order.
//
// `renderFunctionParams` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderFunctionParams(host *emit.Function) (string, error) {
	if host == nil {
		return "", nilHostErrf("renderFunctionParams", "*emit.Function")
	}
	return s.renderParams(mergeParams(host.Params, s.mergedSlotParams(host.ParamsSlot())))
}

// renderMethodParams returns the rendered parenthesised parameter
// list for a method — typed [emit.Method.Params] followed by every
// cross-cutting parameter contributed through
// [emit.Method.ParamsSlot], re-grouped by plugin topo order.
//
// `renderMethodParams` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderMethodParams(host *emit.Method) (string, error) {
	if host == nil {
		return "", nilHostErrf("renderMethodParams", "*emit.Method")
	}
	return s.renderParams(mergeParams(host.Params, s.mergedSlotParams(host.ParamsSlot())))
}

// renderFunctionReturns returns the rendered return clause for a
// function — typed [emit.Function.Returns] followed by every
// cross-cutting return contributed through
// [emit.Function.ReturnsSlot], re-grouped by plugin topo order.
// Surfaces [emit.ErrMixedNamedReturns] when the merged slice mixes
// named and unnamed entries.
//
// The result includes its own leading space when non-empty so the
// `emit.function` template can drop the surrounding `{{ if }}`
// wrapper: a void function renders an empty string here.
//
// `renderFunctionReturns` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderFunctionReturns(host *emit.Function) (string, error) {
	if host == nil {
		return "", nilHostErrf("renderFunctionReturns", "*emit.Function")
	}
	return s.composedReturns(mergeReturns(host.Returns, s.mergedSlotReturns(host.ReturnsSlot())))
}

// renderMethodReturns returns the rendered return clause for a
// method — typed [emit.Method.Returns] followed by every
// cross-cutting return contributed through
// [emit.Method.ReturnsSlot], re-grouped by plugin topo order. The
// result includes a leading space when non-empty.
//
// `renderMethodReturns` is one of the reserved canonical-render
// funcmap entries — plugin overrides are rejected at Build time.
func (s *renderState) renderMethodReturns(host *emit.Method) (string, error) {
	if host == nil {
		return "", nilHostErrf("renderMethodReturns", "*emit.Method")
	}
	return s.composedReturns(mergeReturns(host.Returns, s.mergedSlotReturns(host.ReturnsSlot())))
}

// composedReturns wraps [renderState.renderReturns], prefixing the
// non-empty result with a single space so the per-host wrappers
// drop the template-side `{{ if }}` conditional.
func (s *renderState) composedReturns(returns []*emit.Return) (string, error) {
	out, err := s.renderReturns(returns)
	if err != nil || out == "" {
		return out, err
	}
	return " " + out, nil
}

// mergedSlotReturns returns the additional [*emit.Return] entries
// contributed via the supplied slot, re-grouped by plugin order.
// Caller appends these to the host's typed Returns slice.
func (s *renderState) mergedSlotReturns(slot *emit.Slot) []*emit.Return {
	if slot.Len() == 0 {
		return nil
	}
	items, _ := orderByPlugin(slot.Items, slot.ProvenanceList, s.pluginOrder)
	out := make([]*emit.Return, 0, len(items))
	for _, item := range items {
		out = append(out, item.(*emit.Return))
	}
	return out
}

// mergeReturns appends slot-contributed returns after the typed
// return list, returning a fresh slice. Empty inputs are handled
// gracefully — typed is returned untouched when slot is empty.
func mergeReturns(typed, slot []*emit.Return) []*emit.Return {
	if len(slot) == 0 {
		return typed
	}
	out := make([]*emit.Return, 0, len(typed)+len(slot))
	out = append(out, typed...)
	out = append(out, slot...)
	return out
}

// composeBody concatenates the three statement segments — prebody,
// typed body, postbody — in source order. Returns a fresh slice;
// nil segments contribute zero entries.
func composeBody(pre, typed, post []*emit.Stmt) []*emit.Stmt {
	out := make([]*emit.Stmt, 0, len(pre)+len(typed)+len(post))
	out = append(out, pre...)
	out = append(out, typed...)
	out = append(out, post...)
	return out
}

// mergeParams appends slot-contributed parameters after the typed
// parameter list, returning a fresh slice. Empty inputs are
// handled gracefully.
func mergeParams(typed, slot []*emit.Param) []*emit.Param {
	if len(slot) == 0 {
		return typed
	}
	out := make([]*emit.Param, 0, len(typed)+len(slot))
	out = append(out, typed...)
	out = append(out, slot...)
	return out
}

// fieldNameSet returns a name → provenance-marker map seeded with
// the typed-fields slice; the marker stays "typed:<owner>" to
// distinguish typed-content collisions from slot-contributor
// collisions in error messages.
func fieldNameSet(fields []*emit.Field) map[string]string {
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		out[f.Name] = typedContentSource
	}
	return out
}

// methodNameSet — same shape as [fieldNameSet] for typed methods.
func methodNameSet(methods []*emit.Method) map[string]string {
	out := make(map[string]string, len(methods))
	for _, m := range methods {
		out[m.Name] = typedContentSource
	}
	return out
}

// variantNameSet — same shape as [fieldNameSet] for typed enum
// variants.
func variantNameSet(variants []*emit.EnumVariant) map[string]string {
	out := make(map[string]string, len(variants))
	for _, v := range variants {
		out[v.Name] = typedContentSource
	}
	return out
}
