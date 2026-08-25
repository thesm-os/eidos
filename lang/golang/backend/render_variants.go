// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"strings"

	"go.thesmos.sh/eidos/emit"
)

// renderVariants assembles the body of an enum's const block — one
// variant per line.
//
// For typed enums (those with [emit.Enum.Underlying] populated) the
// first variant carries the enum's own name between its identifier
// and value (`Red Color = iota`) so Go's typed-iota promotion
// extends through subsequent variants. Untyped enums omit that
// type slot entirely, producing bare `Name = Value` lines.
//
// Variants whose Value is nil emit just the bare name, letting Go's
// auto-increment supply the value; variants with an explicit Value
// render as `Name = Value`.
//
// [emit.EnumVariant.DocLines] render above the variant line; a
// blank line separates documented variants from undocumented
// siblings — the same convention [renderState.renderFields] uses.
// gofmt inside the enclosing `const ( ... )` block aligns the
// names and equals signs.
//
// Internal helper: [renderState.renderEnumVariants] (the
// funcmap-exposed entry) merges typed variants with slot
// contributions then calls this helper to format the resulting
// enum.
func (s *renderState) renderVariants(e *emit.Enum) (string, error) {
	typeName := ""
	if e.Underlying != nil {
		typeName = e.Name
	}
	var b strings.Builder
	for i, v := range e.Variants {
		if i > 0 && (len(v.DocLines) > 0 || len(e.Variants[i-1].DocLines) > 0) {
			b.WriteByte('\n')
		}
		b.WriteString(renderDocs(v.DocLines))
		b.WriteString(v.Name)
		if i == 0 && typeName != "" {
			b.WriteByte(' ')
			b.WriteString(typeName)
		}
		if v.Value != nil {
			val, err := s.renderExpr(v.Value)
			if err != nil {
				return "", err
			}
			b.WriteString(" = ")
			b.WriteString(val)
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}
