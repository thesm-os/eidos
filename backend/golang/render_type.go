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

// renderComposite dispatches on the [emit.CompositeRef.Shape] and
// returns the Go source spelling for the composite. All documented
// shapes are wired: Pointer (`*T`), Slice (`[]T`), Array (`[N]T`),
// Map (`map[K]V`), Func (`func(P) R`), and Union (`A | ~B | C`).
// Unknown shape values surface as [ErrUnsupportedRef] wrapped with
// the offending shape — a future variant added to the discriminator
// would land here.
func (s *renderState) renderComposite(r *emit.CompositeRef) (string, error) {
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
	case emit.ShapeFunc:
		return s.renderFuncShape(r.FuncParams, r.FuncReturns)
	case emit.ShapeUnion:
		return s.renderUnion(r.UnionTerms)
	default:
		return "", fmt.Errorf("%w: composite shape %s", ErrUnsupportedRef, r.Shape)
	}
}

// renderFuncShape returns the Go source spelling of a function
// type: `func(P1, P2) R`, `func()`, `func(P) (R1, R2)`. Parameter
// and return types render through [renderState.renderType]; both
// lists are unnamed (the type-only form Go allows in field /
// variable / parameter declarations of function type).
func (s *renderState) renderFuncShape(params, returns []emit.Ref) (string, error) {
	paramParts := make([]string, 0, len(params))
	for _, p := range params {
		r, err := s.renderType(p)
		if err != nil {
			return "", err
		}
		paramParts = append(paramParts, r)
	}
	retText, err := s.renderReturns(emit.AnonReturns(returns...))
	if err != nil {
		return "", err
	}
	out := "func(" + strings.Join(paramParts, ", ") + ")"
	if retText != "" {
		out += " " + retText
	}
	return out, nil
}

// renderUnion produces the Go union-constraint spelling for a
// `T1 | T2 | ~T3` sequence: terms joined by " | ", with the
// approximation marker `~` prefixing terms whose Approx flag is
// set. Empty term slices yield the empty string — the caller
// (typically [renderState.renderTypeParams]) is responsible for
// treating an empty constraint as a programming error if relevant.
func (s *renderState) renderUnion(terms []emit.UnionTerm) (string, error) {
	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		rendered, err := s.renderType(t.Type)
		if err != nil {
			return "", err
		}
		if t.Approx {
			rendered = "~" + rendered
		}
		parts = append(parts, rendered)
	}
	return strings.Join(parts, " | "), nil
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
