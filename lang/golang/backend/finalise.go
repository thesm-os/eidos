// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"go/format"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
)

// finaliseBody runs body through [go/format.Source] for gofmt-clean
// formatting. It does not abort the surrounding render loop: on
// failure it falls back to the unformatted bytes so the user can
// read what was produced.
//
// # Why this is the whole chain
//
// It used to end with the goimports library pass, for import
// resolution and stdlib/external regrouping. Both jobs now belong to
// the backend — [writeImportBlock] owns the arrangement,
// [pruneImports] owns the deletion, and the unresolved-qualifier
// report owns what the resolver used to repair by guessing.
//
// The pass was not merely redundant, it was the framework's largest
// single cost and a reproducibility hole. `imports.Process` builds a
// fresh ProcessEnv per call with no environment supplied, which
// reaches `go env -json` — one fork and exec of the real toolchain
// per generated file, amortised across nothing, because the
// resolver and its caches are discarded when the call returns. A
// single unresolved reference escalated that into an uncached walk
// of the module cache, 112-129 ms, which then *invented* an import
// from whatever happened to be on the developer's disk. Generated
// bytes depended on the machine that generated them, which the
// provenance hash exists to make impossible.
//
// There is no way to keep the pass and avoid the fork: the escape
// hatch is to pre-populate ProcessEnv, and the public Options struct
// exposes no Env field while internal/imports is unimportable.
func finaliseBody(body []byte, target emit.Target, ps *diag.PluginSink) []byte {
	formatted, ok := runGoFormat(body, target, ps)
	if !ok {
		return body
	}
	return formatted
}

// runGoFormat runs body through [go/format.Source]. Returns the
// formatted bytes and ok=true on success; on failure attaches an
// Error diagnostic and returns (body, false) so the caller falls
// back to the unformatted bytes.
//
// Error rather than Warn: the formatter refusing the body means the
// body is not syntactically valid Go, so the file cannot compile
// however it is written out. The bytes are still emitted — reading
// the broken output is how a template bug gets diagnosed — but the
// run fails, which is what stops unparseable generated code from
// reaching a build that has no idea where it came from.
//
// [go/ast.SortImports], which format.Source calls, re-sorts each
// blank-line-delimited run of the import block by path. That is why
// [writeImportBlock] only has to place the blank lines correctly:
// a within-group order it got wrong would be corrected here.
func runGoFormat(body []byte, target emit.Target, ps *diag.PluginSink) ([]byte, bool) {
	out, err := format.Source(body)
	if err != nil {
		ps.Errorf(position.Pos{}, "%s: format.Source failed, generated output is not valid Go: %v",
			target.JoinPath(), err)
		return body, false
	}
	return out, true
}
