// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/emit"
)

// renderFields produces the body content of a Go struct
// declaration — every field rendered on its own tab-indented line,
// separated by blank lines on the doc-boundary so each documented
// field forms a visually grouped unit per Go convention.
//
// Per-field line shape: `\t<Name> <renderType(.Type)>[ ` + "`" + `<tags>` + "`" + `][ // <.LineComment>]`.
// DocLines render above each field via [renderDocs] (with the same
// "//"-prefixed-lines-pass-through" semantics used at decl level).
//
// Tag aggregation: the rendered tag blob unions [emit.Field.Tag]
// (the host generator's base tag, rendered verbatim) with every
// [*emit.Tag] appended to [emit.Field.Tags] (cross-cutting plugin
// contributions). Slot entries render as `Key:"EscapedValue"` —
// values are Go-string-escaped via [strconv.Quote]. Order: base
// tag first, then slot entries in append order. Both empty produces
// no tag at all (not even the backticks).
//
// Separation rule: emit a blank line BETWEEN any two adjacent
// fields when either carries DocLines — this groups documented
// fields with their docs and visually separates them from
// undocumented siblings, without introducing a trailing blank
// before the closing brace.
//
// Internal helper: [renderState.renderStructFields] (the
// funcmap-exposed entry) merges typed fields with slot contributions
// then calls this helper to format the resulting slice.
func (s *renderState) renderFields(fields []*emit.Field) (string, error) {
	var b strings.Builder
	for i, f := range fields {
		if i > 0 && (len(f.DocLines) > 0 || len(fields[i-1].DocLines) > 0) {
			b.WriteByte('\n')
		}
		b.WriteString(renderDocs(f.DocLines))
		b.WriteByte('\t')
		b.WriteString(fieldNameFor(f))
		b.WriteByte(' ')
		t, err := s.renderType(f.Type)
		if err != nil {
			return "", err
		}
		b.WriteString(t)
		if tag := fieldTagBlob(f); tag != "" {
			b.WriteString(" `")
			b.WriteString(tag)
			b.WriteByte('`')
		}
		if f.LineComment != "" {
			b.WriteString(" // ")
			b.WriteString(f.LineComment)
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// fieldTagBlob assembles the backtick-wrapped struct-tag content
// for a single field. The base [emit.Field.Tag] (raw text the host
// generator declared) renders first, followed by each [*emit.Tag]
// appended to the field's tags slot in append order; non-Tag slot
// entries are ignored so the helper degrades gracefully when a
// plugin appends an unrelated node by mistake. Returns the empty
// string when neither the base tag nor the slot carry content; the
// caller omits the backtick wrap entirely in that case.
func fieldTagBlob(f *emit.Field) string {
	var b strings.Builder
	if f.Tag != "" {
		b.WriteString(f.Tag)
	}
	for _, item := range f.Tags().Items {
		t, ok := item.(*emit.Tag)
		if !ok {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(t.Key)
		b.WriteByte(':')
		b.WriteString(strconv.Quote(t.Value))
	}
	return b.String()
}

// renderEmbeds produces the rendered embed lines that go inside a
// struct or interface body — one tab-indented `renderType(.Type)`
// per embed, terminated by a newline. Empty input returns the
// empty string; the caller emits no leading or trailing whitespace
// for missing embeds.
//
// Internal helper: [renderState.renderStructEmbeds] and
// [renderState.renderInterfaceEmbeds] (the funcmap-exposed entries)
// merge typed embeds with slot contributions then call this helper
// to format the resulting slice.
func (s *renderState) renderEmbeds(embeds []*emit.Embed) (string, error) {
	var b strings.Builder
	for _, e := range embeds {
		t, err := s.renderType(e.Type)
		if err != nil {
			return "", err
		}
		b.WriteByte('\t')
		b.WriteString(t)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// renderTypeParams produces the bracketed generic-parameter list of
// a Go declaration — `[T1 C1, T2 C2]` joined by ", ". Each entry is
// `Name renderType(<bound>)` for type parameters with an explicit
// constraint, falling back to `Name any` for [emit.Constraint.IsAny]
// parameters (preserving Go's grammar requirement that every
// parameter declare a bound).
//
// A constraint with a single [emit.Constraint.Embedded] ref
// renders the ref directly; multiple embeds render as an inline
// interface (`{ E1; E2 }`) — the intersection form Go allows for
// composite constraints.
//
// Empty input returns the empty string; the caller emits no
// brackets when the declaration has no type parameters. The helper
// is reserved canonical-render.
func (s *renderState) renderTypeParams(params []*emit.TypeParam) (string, error) {
	if len(params) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(params))
	for _, p := range params {
		entry, err := s.renderTypeParam(p)
		if err != nil {
			return "", err
		}
		parts = append(parts, entry)
	}
	return "[" + strings.Join(parts, ", ") + "]", nil
}

// renderTypeParam renders a single type-parameter entry as it
// appears in the bracket list — name + space + rendered bound.
// Unconstrained parameters (nil or `IsAny` Constraint) render
// with the predeclared `any` constraint.
func (s *renderState) renderTypeParam(p *emit.TypeParam) (string, error) {
	if p.Constraint.IsAny() {
		return p.Name + " any", nil
	}
	embedded := p.Constraint.Embedded
	if len(embedded) == 1 {
		bound, err := s.renderType(embedded[0])
		if err != nil {
			return "", err
		}
		return p.Name + " " + bound, nil
	}
	parts := make([]string, 0, len(embedded))
	for _, e := range embedded {
		r, err := s.renderType(e)
		if err != nil {
			return "", err
		}
		parts = append(parts, r)
	}
	return p.Name + " interface { " + strings.Join(parts, "; ") + " }", nil
}
