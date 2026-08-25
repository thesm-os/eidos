// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shapewriter

import (
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier surfaced through
// [plugin.Plugin.Name] for ordering tie-breaks, diagnostic
// attribution, and cache-key derivation.
const Name = "shape-writer"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

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
var Detected = sdk.NewKey("shape.writer.detected", sdk.BoolParser)

// MethodQName is the meta key the plugin stamps with the matched
// method's fully-qualified name (`<ownerQName>.<methodName>`) on
// detected structs. Empty when the heuristic does not match — the
// directive-driven positive override sets detected=true without a
// method, in which case the key value is left empty and consumers
// must guard accordingly.
//
//nolint:gochecknoglobals // package-level meta key, registered once at init.
var MethodQName = sdk.NewKey("shape.writer.method", sdk.StringParser)

// Plugin is the writer-shape annotator. Construct it with [New] —
// the embedded base carries the declaration and the zero value has
// none.
type Plugin struct{ *sdk.Base }

// New returns a ready-to-register plugin.
//
// It ships no template tree and declares no [sdk.Output]: an
// annotator's whole product is metadata on the node store, and it
// renders nothing.
//
// It publishes no capability either. Nothing orders against this
// plugin, because downstream consumers reach the shape metadata
// directly through the exported meta keys rather than through the
// topo graph — and it names no requirement, having no upstream
// dependency of its own.
func New() *Plugin {
	return &Plugin{Base: sdk.NewPlugin(Name).
		Version(Version).
		// The shape-detector bucket, so it runs alongside the other
		// annotators that stamp `shape.*` metadata.
		Priority(sdk.AnnotatorShape).
		// Declaring the schema is what lets directive validation
		// reject a malformed `+gen:writer` at frontend-parse time,
		// rather than silently ignoring it here.
		Directives(
			sdk.NewDirective(DirectiveName).
				On(sdk.NodeKindStruct).
				Describe("Forces (+) or suppresses (-) writer-shape detection on the host struct.").
				Build(),
		).
		Build()}
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
func (*Plugin) OnStruct(_ *sdk.AnnotatorContext, s *sdk.Struct) {
	method, matched := matchSignature(s)
	detected := matched
	if d := s.Directive(DirectiveName); d != nil {
		detected = !d.Negated
	}
	var qname string
	if detected && matched {
		qname = methodQName(s, method)
	}
	Detected.Set(s.EnsureMeta(), detected, Name)
	MethodQName.Set(s.EnsureMeta(), qname, Name)
}

// matchSignature returns the method satisfying [io.Writer], or nil +
// false when none does.
//
// The rule — `Write([]byte) (int, error)`, non-variadic, either
// spelling of the byte element — is a fact about Go rather than about
// this plugin, so it is asked of [sdk.IsWriteMethod] rather than
// restated here. A plugin that restates it owns a second copy of a
// language rule and gets to disagree with the first: this one did,
// and for a while it was the copy that was right.
func matchSignature(s *sdk.Struct) (*sdk.Method, bool) {
	for _, m := range s.Methods {
		if golang.IsWriteMethod(m) {
			return m, true
		}
	}
	return nil, false
}

// methodQName composes the store's canonical method-bucket key for the
// matched method.
//
// Through [sdk.MethodQName] rather than a local format string: the
// store composes the same key privately, and a plugin matching it by
// hand stamps a back-link that resolves to nothing the day the
// separator changes — silently, because nothing validates a meta
// value against the index it points into.
func methodQName(s *sdk.Struct, m *sdk.Method) string {
	return golang.MethodQName(s.QName(), m.Name)
}
