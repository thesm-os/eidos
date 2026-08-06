// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shapewriter

import (
	"fmt"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier surfaced through
// [plugin.Plugin.Name] for ordering tie-breaks, diagnostic
// attribution, and cache-key derivation.
const Name = "shape-writer"

// DirectiveName is the bare directive name (without the `+gen:` or
// `-gen:` prefix) the plugin reads from each struct's directive
// list. The positive form forces detection; the negative form
// suppresses it.
const DirectiveName sdk.DirectiveName = "writer"

// Detected is the meta key the plugin stamps with `true` when the
// struct matches the writer shape and `false` otherwise. The key
// is always set — consumers don't need to distinguish "annotator
// didn't run" from "annotator ran, no match".
//
//nolint:gochecknoglobals // package-level meta key, registered once at init.
var Detected = meta.NewKey("shape.writer.detected", meta.BoolParser)

// MethodQName is the meta key the plugin stamps with the matched
// method's fully-qualified name (`<ownerQName>.<methodName>`) on
// detected structs. Empty when the heuristic does not match — the
// directive-driven positive override sets detected=true without a
// method, in which case the key value is left empty and consumers
// must guard accordingly.
//
//nolint:gochecknoglobals // package-level meta key, registered once at init.
var MethodQName = meta.NewKey("shape.writer.method", meta.StringParser)

// writeMethodName is the unqualified method name the heuristic
// targets — the canonical `Write` slot on io.Writer.
const writeMethodName = "Write"

// Plugin is the writer-shape annotator. The zero value is usable.
type Plugin struct{}

// New returns a ready-to-register plugin. Provided for parity with
// other plugins that need constructor-time wiring.
func New() *Plugin { return &Plugin{} }

// Name returns [Name].
func (*Plugin) Name() string { return Name }

// Priority places the plugin in the shape-detector bucket so it
// runs alongside other annotators that stamp `shape.*` metadata.
func (*Plugin) Priority() sdk.Priority { return sdk.AnnotatorShape }

// Provides returns nil — the plugin doesn't expose a capability
// label for cross-plugin topo ordering. Downstream consumers reach
// the shape metadata directly through the meta keys.
func (*Plugin) Provides() []string { return nil }

// Requires returns nil — the plugin has no upstream dependency.
func (*Plugin) Requires() []string { return nil }

// Directives declares the `+gen:writer` / `-gen:writer` schema with
// the pipeline so directive validation rejects malformed uses at
// frontend-parse time.
func (*Plugin) Directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(node.KindStruct).
			Describe("Forces (+) or suppresses (-) writer-shape detection on the host struct.").
			Build(),
	}
}

// Annotate iterates the node store's struct bucket through
// [sdk.Walk] and stamps the writer-shape metadata on each struct.
func (p *Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
	return sdk.Walk(ctx, p)
}

// OnStruct is the [sdk.StructHook] entry point. The heuristic runs
// first; the directive — when present — overrides its outcome
// (positive forces detection, negative suppresses it). The method
// back-link is recorded whenever the heuristic matched AND the
// final detection is true, so a directive-driven match without a
// real Write method records an empty back-link alongside
// detected=true.
func (*Plugin) OnStruct(_ *sdk.AnnotatorContext, s *node.Struct) {
	method, matched := matchSignature(s)
	detected := matched
	if d := s.Directive(DirectiveName); d != nil {
		detected = !d.Negated
	}
	var qname string
	if detected && matched {
		qname = methodQName(s, method)
	}
	Detected.Set(s.Meta(), detected, Name)
	MethodQName.Set(s.Meta(), qname, Name)
}

// matchSignature returns the method that matches the canonical
// io.Writer signature, or nil + false when none does. The match
// is exact: parameter type `[]byte`, return types `(int, error)`,
// method name `Write`.
func matchSignature(s *node.Struct) (*node.Method, bool) {
	for _, m := range s.Methods {
		if m.Name != writeMethodName || len(m.Params) != 1 || len(m.Returns) != 2 {
			continue
		}
		if !isByteSlice(m.Params[0].Type) {
			continue
		}
		if !isBuiltin(m.Returns[0].Type, "int") || !isBuiltin(m.Returns[1].Type, "error") {
			continue
		}
		return m, true
	}
	return nil, false
}

// isByteSlice reports whether ref is the unqualified `[]byte`
// type — a Slice variant carrying an Elem of `byte` (or its alias
// `uint8`). Callers pass refs sourced from a parsed Go method
// signature, so the frontend invariants guarantee non-nil refs
// and non-nil Elem on slice variants.
func isByteSlice(ref *node.TypeRef) bool {
	if !ref.IsSlice() {
		return false
	}
	return isBuiltin(ref.Elem, "byte") || isBuiltin(ref.Elem, "uint8")
}

// isBuiltin reports whether ref is the unqualified builtin named
// want — Named variant, no package, exactly the given Name.
func isBuiltin(ref *node.TypeRef, want string) bool {
	return ref.IsBuiltin() && ref.Name == want
}

// methodQName recomposes the store's canonical method-bucket key
// (`<ownerQName>.<methodName>`) for the matched method. Mirrors
// the format [store.NodeView.addMethod] uses.
func methodQName(s *node.Struct, m *node.Method) string {
	return fmt.Sprintf("%s.%s", s.QName(), m.Name)
}
