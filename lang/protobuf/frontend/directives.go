// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"errors"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/plugin"
)

// parseDirectiveBlock walks the lines of raw and asks the
// pipeline-supplied parser to extract directives from each line.
// Lines that don't carry the `+gen:` / `-gen:` prefixes parse to
// (nil, nil) and are silently ignored; the returned slice is the
// per-line concatenation in source order.
//
// Malformed-directive errors surface as positioned diagnostics
// attributed to the frontend's plugin name. The per-line offset
// rides on the diagnostic position so a typo five lines into a
// comment block anchors near the offending line rather than at
// the block's start. protocompile does not surface per-line
// positions for comment blocks, so the offset is approximate
// (block start + line index) — close enough for users to find
// the line in a text editor.
func parseDirectiveBlock(
	parser *directive.Parser, ps *diag.PluginSink,
	raw string, blockPos position.Pos,
) []*directive.Directive {
	if raw == "" || parser == nil {
		return nil
	}
	var out []*directive.Directive
	lineIdx := 0
	for line := range strings.SplitSeq(raw, "\n") {
		linePos := blockPos
		linePos.Line += lineIdx
		lineIdx++
		ds, err := parser.ParseComment(line, linePos)
		if err != nil {
			if errors.Is(err, directive.ErrMalformedDirective) && ps != nil {
				ps.Errorf(linePos, "directive parse: %v", err)
			}
			continue
		}
		out = append(out, ds...)
	}
	return out
}

// directivesFor parses every directive declared on sl's leading
// comments — both the immediate LeadingComments string and any
// LeadingDetachedComments blocks — using ctx's pipeline parser.
// Returns nil when ctx is nil (test paths that bypass the
// pipeline) or when no directives are present.
func directivesFor(
	ctx *plugin.FrontendContext, sl protoreflect.SourceLocation, pos position.Pos,
) []*directive.Directive {
	if ctx == nil || ctx.Parser == nil {
		return nil
	}
	ps := ctx.Diag.For(FrontendName)
	out := parseDirectiveBlock(ctx.Parser, ps, sl.LeadingComments, pos)
	for _, detached := range sl.LeadingDetachedComments {
		out = append(out, parseDirectiveBlock(ctx.Parser, ps, detached, pos)...)
	}
	return out
}
