// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// ErrUnsupportedRef is returned by [renderState.renderType] when
// called with an [emit.Ref] kind the current funcmap can't render,
// or by [internalTargetName] when a [emit.TypeRef] points at a
// target kind whose name can't yet be extracted. The wrapped
// message names the concrete Go type so diagnostics attribute the
// gap precisely.
var ErrUnsupportedRef = errors.New("golang: unsupported Ref")

// renderType produces the Go source spelling for r. Supported kinds:
//
//   - [emit.BuiltinRef] — rendered as the builtin's Name.
//   - [emit.ExternalRef] — rendered as "<alias>.<Name>", with
//     <alias> obtained by registering the package path via the
//     state's [writer.ImportSet].
//   - [emit.TypeRef] — rendered as the unqualified name of the
//     target node (TypeRef is same-package by contract).
//   - [emit.CompositeRef] — dispatched to [renderState.renderComposite]
//     for the per-shape rendering.
//
// Other ref kinds return [ErrUnsupportedRef] wrapped with the
// concrete Go type.
//
// `renderType` is one of the reserved canonical-render funcmap
// entries — plugin overrides are rejected at Build time.
func (s *renderState) renderType(r emit.Ref) (string, error) {
	if got, ok := bridgeTypeOverride(r); ok {
		if err := s.registerBridgeImport(r); err != nil {
			return "", err
		}
		return got, nil
	}
	if got, ok, err := s.renderChan(r); ok || err != nil {
		return got, err
	}
	switch typed := r.(type) {
	case *emit.BuiltinRef:
		return typed.Name, nil
	case *emit.ExternalRef:
		// Cross-language frontends thread the source-language path
		// through emit.ExternalRef.Package (proto's
		// `eidos.test.buildfixture`); the bridge-imports map
		// resolves that to the Go-canonical import path so the
		// rendered import block carries a path go/build accepts.
		// Go-source pipelines see no bridge meta and the path passes
		// through verbatim.
		alias, err := s.imports.Imp(s.resolveImportPath(typed.Package))
		if err != nil {
			return "", fmt.Errorf("backend/golang: renderType: %w", err)
		}
		args, err := s.renderTypeArgs(typed.TypeArgs)
		if err != nil {
			return "", err
		}
		name := goExternalRefName(typed.Name)
		if alias == "" {
			// Same-package elision: Imp returned the empty alias
			// because typed.Package equals the rendered file's own
			// import path. Drop the qualifier and emit the bare name.
			return name + args, nil
		}
		return alias + "." + name + args, nil
	case *emit.TypeRef:
		base, err := internalTargetName(typed.Target)
		if err != nil {
			return "", err
		}
		args, err := s.renderTypeArgs(typed.TypeArgs)
		if err != nil {
			return "", err
		}
		// Cross-package qualification: when the target has a
		// resolved import path that differs from the rendering
		// file's own import path, register the target's package on
		// the file's import set and qualify the rendered name with
		// the resulting alias. Targets without a resolved import
		// path (synthetic, unrouted) fall through to bare —
		// preserving the historical "same-package by contract"
		// behaviour for refs that the Layout phase never saw.
		targetPath := s.resolveImportPath(targetImportPath(typed.Target))
		if targetPath == "" {
			return base + args, nil
		}
		alias, err := s.imports.Imp(targetPath)
		if err != nil {
			return "", fmt.Errorf("backend/golang: renderType: %w", err)
		}
		if alias == "" {
			return base + args, nil
		}
		return alias + "." + base + args, nil
	case *emit.CompositeRef:
		return s.renderComposite(typed)
	default:
		return "", fmt.Errorf("%w: %T", ErrUnsupportedRef, r)
	}
}

// goExternalRefName normalises an [emit.ExternalRef.Name] to a
// Go-valid identifier. Cross-language frontends surface nested
// types under the source language's separator (proto's
// dot-joined `Outer.Inner`); Go identifiers cannot contain dots,
// so the dot-joined form maps to the underscore-joined
// `Outer_Inner` that matches the protoc-gen-go convention. Names
// without dots pass through verbatim; Go-source-derived refs
// never carry dots since Go identifiers can't contain them, so
// the normalisation is a no-op for Go-only pipelines.
func goExternalRefName(name string) string {
	if !strings.ContainsRune(name, '.') {
		return name
	}
	return strings.ReplaceAll(name, ".", "_")
}

// bridgeTypeOverride consults the bridge-stamped `go.type` meta
// on r's source-side origin and returns the override when
// present. Cross-language bridge annotators (the protogo bridge
// for proto→Go, future protorust / prototypescript variants)
// stamp the rendered Go-side form on the source node.TypeRef so
// the render-site lands a Go-compilable identifier without
// learning anything proto-specific. Empty return falls through to
// the standard kind-based rendering.
//
// The lookup goes through the source-side meta bag reached via
// the emit ref's OriginNode (refconv threads this) — no cross-
// package import of the bridge plugin's key constants is needed
// because [meta.EnsureKey] returns the same registry singleton
// regardless of declaration site.
func bridgeTypeOverride(r emit.Ref) (string, bool) {
	if r == nil {
		return "", false
	}
	origin, ok := r.Origin().(*node.TypeRef)
	if !ok {
		return "", false
	}
	got, ok := goTypeKey.Get(origin.Meta())
	if !ok || got == "" {
		return "", false
	}
	return got, true
}

// renderChan renders a Go channel type when r's source-side origin
// is marked as one, reporting ok=false for everything else.
//
// The Go frontend models a channel as a *named* reference in a
// synthetic package — `go`.`chan` with the element as its single
// type argument — and stamps the real facts as meta: `go.isChannel`
// and `go.chanDir`. Without this arm the named reference falls
// through to the ExternalRef path, which emits `import "go"` (a path
// that resolves against the standard library and fails) and
// qualifies as `go.chan[T]`, which does not parse.
//
// Rendering here rather than modelling a channel shape in [emit]
// keeps a Go concurrency primitive out of the language-agnostic
// layer. A channel has no counterpart in most targets — Rust's is a
// library type, not a type form — so a backend that does not
// understand `go.isChannel` simply never asks, instead of being
// handed a shape it must implement or explicitly refuse.
//
// The element renders through [renderState.renderType], so its own
// import registers exactly as it would anywhere else. That is the
// property a string-shaped workaround could not have: a builtin ref
// carrying "<-chan pkg.T" is a leaf and registers nothing.
func (s *renderState) renderChan(r emit.Ref) (string, bool, error) {
	if r == nil {
		return "", false, nil
	}
	origin, ok := r.Origin().(*node.TypeRef)
	if !ok {
		return "", false, nil
	}
	if isChan, ok := goIsChannelKey.Get(origin.Meta()); !ok || !isChan {
		return "", false, nil
	}
	elem, err := s.chanElem(r, origin)
	if err != nil {
		return "", true, err
	}
	dir, _ := goChanDirKey.Get(origin.Meta())
	switch dir {
	case "send":
		return "chan<- " + elem, true, nil
	case "recv":
		return "<-chan " + elem, true, nil
	default:
		// Unset or "both". Bidirectional is the permissive form: it
		// satisfies neither directed one, so a missing stamp fails at
		// compile time rather than silently narrowing a signature.
		return "chan " + elem, true, nil
	}
}

// chanElem renders a channel's element type, which the frontend
// carries as the reference's single type argument.
//
// An element is always present for a channel the frontend produced.
// A missing one means the ref was hand-built by a plugin claiming
// `go.isChannel` without the structure to back it, which is a plugin
// bug worth naming rather than rendering `chan ` and letting the
// formatter report a syntax error with no attribution.
func (s *renderState) chanElem(r emit.Ref, origin *node.TypeRef) (string, error) {
	if ext, ok := r.(*emit.ExternalRef); ok && len(ext.TypeArgs) == 1 {
		return s.renderType(ext.TypeArgs[0])
	}
	return "", fmt.Errorf("%w: channel ref %q carries no element type",
		ErrUnsupportedRef, origin.Name)
}

// goIsChannelKey is the Go frontend's `go.isChannel` marker.
// [meta.EnsureKey] resolves to the same registry singleton as the
// frontend's declaration, so the backend reads the fact without
// importing the frontend — which depguard forbids outright.
//
//nolint:gochecknoglobals // cross-package registry-singleton key
var goIsChannelKey = meta.EnsureKey("go.isChannel", meta.BoolParser)

// goChanDirKey is the Go frontend's `go.chanDir` stamp, carrying
// "both", "send" or "recv".
//
//nolint:gochecknoglobals // cross-package registry-singleton key
var goChanDirKey = meta.EnsureKey("go.chanDir", meta.StringParser)

// goTypeKey is the bridge-stamped `go.type` meta key shared
// across every cross-language Go-targeting bridge. [meta.EnsureKey]
// resolves to the same registry singleton regardless of the
// declaring package.
//
//nolint:gochecknoglobals // cross-package registry-singleton key
var goTypeKey = meta.EnsureKey("go.type", meta.StringParser)

// goNameKey is the bridge-stamped `go.name` meta key shared
// across every cross-language Go-targeting bridge. Lives at this
// site so render-site lookups don't need to reach into a
// bridge plugin's exported constants.
//
//nolint:gochecknoglobals // cross-package registry-singleton key
var goNameKey = meta.EnsureKey("go.name", meta.StringParser)

// goImportKey is the bridge-stamped `go.import` meta key.
//
//nolint:gochecknoglobals // cross-package registry-singleton key
var goImportKey = meta.EnsureKey("go.import", meta.StringParser)

// registerBridgeImport pulls the bridge-stamped go.import meta
// off r's source-side origin and registers it on the host
// file's ImportSet. The override-path uses verbatim Go-type
// strings ("*timestamppb.Timestamp") rather than emit.External
// pairs, so the import block won't pick up the path through
// the normal ExternalRef→Imp flow — the bridge has to register
// the path explicitly.
func (s *renderState) registerBridgeImport(r emit.Ref) error {
	if r == nil {
		return nil
	}
	origin, ok := r.Origin().(*node.TypeRef)
	if !ok {
		return nil
	}
	path, ok := goImportKey.Get(origin.Meta())
	if !ok || path == "" {
		return nil
	}
	if _, err := s.imports.Imp(path); err != nil {
		return fmt.Errorf("backend/golang: renderType: bridge import %q: %w", path, err)
	}
	return nil
}

// fieldNameFor returns the rendered identifier for f. The
// bridge-stamped go.name meta on f's source-side origin wins
// over the emit-side Name when present; the lookup walks
// f.Origin first, then falls back to f.Name verbatim. The
// origin can be a node.Field (when the generator threads it) or
// nil (when the emit field is synthesized without source
// provenance) — both cases route to the fallback.
func fieldNameFor(f *emit.Field) string {
	if f == nil {
		return ""
	}
	if origin, ok := f.Origin().(*node.Field); ok {
		if got, ok := goNameKey.Get(origin.Meta()); ok && got != "" {
			return got
		}
	}
	return f.Name
}

// renderComposite produces the source spelling of a composite type,
// choosing between two strategies.
//
// A composite whose children are not themselves composites — *T,
// []T, map[K]V for concrete K and V, the overwhelming majority of
// what real code declares — takes one concatenation, which is one
// allocation and is already the floor.
//
// Anything deeper goes through an append buffer. Concatenating each
// level onto the fully rendered string below it allocates once per
// level and memmoves the whole subtree each time: depth n costs n
// allocations and O(n²) bytes, and a thousand-deep chain produced a
// 1003-byte string at a cost of 533 kB in 1000 allocations. Through
// the buffer that is 4.3 kB in 7.
//
// The buffer is not free — it cannot avoid a second allocation,
// because it escapes through the recursive calls and the result
// string copies out of it — so applying it to shallow shapes would
// trade one allocation for two on the common path. Splitting here
// keeps every existing shape at or below what it cost before.
//
// Func and union shapes always buffer: both join several children,
// so both were paying strings.Join over already-materialised parts
// regardless of depth.
func (s *renderState) renderComposite(r *emit.CompositeRef) (string, error) {
	switch r.Shape {
	case emit.ShapePointer, emit.ShapeSlice, emit.ShapeArray, emit.ShapeMap:
		if !hasCompositeChild(r) {
			return s.renderShallowComposite(r)
		}
	case emit.ShapeFunc, emit.ShapeUnion, emit.ShapeAnonStruct:
		// Always buffered: each joins several children, so each was
		// materialising every child and joining regardless of depth.
	}
	var scratch [64]byte
	buf, err := s.appendComposite(scratch[:0], r)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// hasCompositeChild reports whether r nests another composite, which
// is exactly where the quadratic concatenation begins.
func hasCompositeChild(r *emit.CompositeRef) bool {
	isComposite := func(x emit.Ref) bool {
		_, ok := x.(*emit.CompositeRef)
		return ok
	}
	switch r.Shape {
	case emit.ShapeMap:
		return isComposite(r.MapKey) || isComposite(r.MapValue)
	default:
		return isComposite(r.Elem)
	}
}

// renderShallowComposite is the single-concatenation path for a
// composite with no composite child.
func (s *renderState) renderShallowComposite(r *emit.CompositeRef) (string, error) {
	switch r.Shape {
	case emit.ShapePointer:
		elem, err := s.renderType(r.Elem)
		if err != nil {
			return "", err
		}
		return "*" + elem, nil
	case emit.ShapeSlice:
		elem, err := s.renderType(r.Elem)
		if err != nil {
			return "", err
		}
		return "[]" + elem, nil
	case emit.ShapeArray:
		elem, err := s.renderType(r.Elem)
		if err != nil {
			return "", err
		}
		return "[" + strconv.Itoa(r.ArrayLen) + "]" + elem, nil
	case emit.ShapeMap:
		key, err := s.renderType(r.MapKey)
		if err != nil {
			return "", err
		}
		val, err := s.renderType(r.MapValue)
		if err != nil {
			return "", err
		}
		return "map[" + key + "]" + val, nil
	default:
		return "", fmt.Errorf("%w: composite shape %s", ErrUnsupportedRef, r.Shape)
	}
}

// appendType appends r's Go source spelling to dst.
//
// It dispatches on exactly one thing: a composite recurses into
// [renderState.appendComposite] and writes its prefixes straight into
// dst; everything else routes back through [renderState.renderType]
// and appends the string it returns.
//
// Routing leaves back through renderType is what keeps this small.
// The bridge-override and channel arms that run before renderType's
// type switch are reached without being duplicated, the zero-copy
// fast paths for builtins and same-package refs survive untouched,
// and — the part that matters most — the arms that call
// [writer.ImportSet.Imp] stay in one place. Collision suffixes are
// assigned in first-import order, so a traversal that visited a map
// value before its key would silently change generated aliases in
// any file importing two packages that share a last segment.
func (s *renderState) appendType(dst []byte, r emit.Ref) ([]byte, error) {
	if c, ok := r.(*emit.CompositeRef); ok {
		return s.appendComposite(dst, c)
	}
	text, err := s.renderType(r)
	if err != nil {
		return nil, err
	}
	return append(dst, text...), nil
}

// appendComposite appends r's spelling to dst.
//
// Each arm used to concatenate its prefix onto the fully rendered
// string below it, allocating a fresh string per level and
// memmoving the whole subtree into it. Depth n cost n allocations
// and O(n²) bytes — a thousand-deep pointer chain produced a
// 1003-byte string at a cost of 533 kB.
//
// The visit order reproduces the concatenation order exactly,
// including strconv for the array length, the union separator and
// the approximation prefix, so both the bytes and the Imp call
// sequence are unchanged.
func (s *renderState) appendComposite(dst []byte, r *emit.CompositeRef) ([]byte, error) {
	switch r.Shape {
	case emit.ShapePointer:
		return s.appendType(append(dst, '*'), r.Elem)
	case emit.ShapeSlice:
		return s.appendType(append(dst, "[]"...), r.Elem)
	case emit.ShapeArray:
		dst = append(dst, '[')
		dst = strconv.AppendInt(dst, int64(r.ArrayLen), 10)
		return s.appendType(append(dst, ']'), r.Elem)
	case emit.ShapeMap:
		keyed, err := s.appendType(append(dst, "map["...), r.MapKey)
		if err != nil {
			return nil, err
		}
		return s.appendType(append(keyed, ']'), r.MapValue)
	case emit.ShapeFunc:
		return s.appendFuncShape(dst, r.FuncParams, r.FuncReturns)
	case emit.ShapeUnion:
		return s.appendUnion(dst, r.UnionTerms)
	case emit.ShapeAnonStruct:
		return s.appendAnonStruct(dst, r.StructFields, r.StructEmbeds)
	default:
		return nil, fmt.Errorf("%w: composite shape %s", ErrUnsupportedRef, r.Shape)
	}
}

// appendFuncShape appends the source spelling of a function type.
//
// The return list goes through [renderState.appendAnonReturns]
// rather than through renderReturns: a func type has no return names
// by construction, and reaching renderReturns meant wrapping every
// return type in an emit.Return purely to match its signature — 176
// bytes of provenance scaffolding written, read once for .Type, and
// discarded, which was 77% of what rendering a func type cost.
func (s *renderState) appendFuncShape(dst []byte, params, returns []emit.Ref) ([]byte, error) {
	dst = append(dst, "func("...)
	for i, p := range params {
		if i > 0 {
			dst = append(dst, ", "...)
		}
		var err error
		if dst, err = s.appendType(dst, p); err != nil {
			return nil, err
		}
	}
	dst = append(dst, ')')
	if len(returns) == 0 {
		return dst, nil
	}
	return s.appendAnonReturns(append(dst, ' '), returns)
}

// appendAnonReturns appends an anonymous return list, implementing
// the same truth table as [renderState.renderReturns] without
// wrapping anything: nothing for zero returns, a bare spelling for
// one, a parenthesised comma-joined list for more.
//
// The single-return case is bare — `error`, not `(error)` — which is
// the branch a careless port loses. Anonymous types cannot reach the
// mixed-named branch, so ErrMixedNamedReturns is unreachable here;
// it stays live in renderReturns, where named returns still reach it.
func (s *renderState) appendAnonReturns(dst []byte, types []emit.Ref) ([]byte, error) {
	switch len(types) {
	case 0:
		return dst, nil
	case 1:
		return s.appendType(dst, types[0])
	}
	dst = append(dst, '(')
	for i, t := range types {
		if i > 0 {
			dst = append(dst, ", "...)
		}
		var err error
		if dst, err = s.appendType(dst, t); err != nil {
			return nil, err
		}
	}
	return append(dst, ')'), nil
}

// appendAnonStruct appends an inline anonymous struct type.
//
// Everything goes on one line, separated by semicolons, because
// nothing here needs to decide otherwise: [finalise] runs
// format.Source over the assembled file, and gofmt is what picks the
// layout — it keeps a single-field inline struct on one line and
// explodes a multi-field one onto its own lines. Emitting the
// canonical form directly would mean reproducing that rule and
// tracking the enclosing indentation, for output the formatter
// rewrites anyway.
//
// Embeds follow fields rather than interleaving with them. Go accepts
// either, source order between the two groups is not recoverable from
// the emit shape, and the split matches how [emit.Struct] already
// separates Fields from Embeds.
//
// Field types route back through [renderState.appendType], so an
// inline `struct{ T time.Time }` registers `time` on the file's
// import set exactly as a named field would. A string-shaped
// workaround — rendering the whole struct into a builtin ref — is a
// leaf and would register nothing, which is the same trap
// [renderState.renderChan] documents.
func (s *renderState) appendAnonStruct(dst []byte, fields []emit.AnonField, embeds []emit.Ref) ([]byte, error) {
	if len(fields) == 0 && len(embeds) == 0 {
		// The set idiom: `map[K]struct{}`. No space inside the braces
		// — `struct{}` is what gofmt produces and what every Go
		// programmer reads as empty.
		return append(dst, "struct{}"...), nil
	}
	dst = append(dst, "struct{ "...)
	first := true
	for _, f := range fields {
		if !first {
			dst = append(dst, "; "...)
		}
		first = false
		dst = append(dst, f.Name...)
		dst = append(dst, ' ')
		var err error
		if dst, err = s.appendType(dst, f.Type); err != nil {
			return nil, err
		}
		if f.Tag != "" {
			// Backtick-quoted, matching the raw form Tag is documented
			// to carry. A tag containing a backtick cannot be spelled
			// this way in Go source at all, so there is no escaping to
			// do — such a tag is unrepresentable, not mis-rendered.
			dst = append(dst, " `"...)
			dst = append(dst, f.Tag...)
			dst = append(dst, '`')
		}
	}
	for _, e := range embeds {
		if !first {
			dst = append(dst, "; "...)
		}
		first = false
		var err error
		if dst, err = s.appendType(dst, e); err != nil {
			return nil, err
		}
	}
	return append(dst, " }"...), nil
}

// appendUnion appends a union constraint's terms, separated by
// " | " and each optionally prefixed with the approximation tilde.
func (s *renderState) appendUnion(dst []byte, terms []emit.UnionTerm) ([]byte, error) {
	for i, t := range terms {
		if i > 0 {
			dst = append(dst, " | "...)
		}
		if t.Approx {
			dst = append(dst, '~')
		}
		var err error
		if dst, err = s.appendType(dst, t.Type); err != nil {
			return nil, err
		}
	}
	return dst, nil
}

// targetImportPath returns the resolved import path on the routing
// target of n — the value the Layout phase composed into the
// target's [emit.Target.ImportPath] / [emit.Alias.File.ImportPath].
// Returns the empty string for kinds whose name we can't qualify
// (then [renderState.renderType] falls through to bare-name
// rendering, the historical same-package contract).
func targetImportPath(n emit.Node) string {
	switch t := n.(type) {
	case *emit.Struct:
		return t.Target.ImportPath
	case *emit.Interface:
		return t.Target.ImportPath
	case *emit.Alias:
		return t.File.ImportPath
	case *emit.Enum:
		return t.Target.ImportPath
	case *emit.Function:
		return t.Target.ImportPath
	default:
		return ""
	}
}

// internalTargetName returns the unqualified declaration name of a
// [emit.TypeRef] target node — the spelling used when the target
// lives in the same Go package as the referring entity. Returns
// [ErrUnsupportedRef] wrapped with the concrete Go type when the
// target is a kind whose name can't be extracted.
func internalTargetName(n emit.Node) (string, error) {
	switch t := n.(type) {
	case *emit.Struct:
		return t.Name, nil
	case *emit.Interface:
		return t.Name, nil
	case *emit.Alias:
		return t.Name, nil
	case *emit.Enum:
		return t.Name, nil
	case *emit.Function:
		return t.Name, nil
	default:
		return "", fmt.Errorf("%w: TypeRef target %T", ErrUnsupportedRef, n)
	}
}
