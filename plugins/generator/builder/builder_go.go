// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder

import (
	"embed"
	"errors"
	"fmt"
	"go/token"
	"strings"
	"text/template"

	"go.thesmos.sh/eidos/lang/golang"
	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// GoSuffix is the per-source-basename suffix the Go adapter
// reports through [GoOutputs]. All `+gen:builder`
// structs declared in `article.go` collate into a single
// `article_builder.go`.
const GoSuffix = "_builder.go"

// ErrMalformedDefaults is the sentinel surfaced by
// [GoDefaultsExpr] when a `defaults=` value is neither a bare
// function identifier nor a non-empty import path plus a
// non-empty function identifier.
// Template execution fails with this error wrapped so the
// offending struct's whole render attempt errors out rather
// than silently dropping the `New<Name>WithDefaults` branch.
var ErrMalformedDefaults = errors.New(
	`builder: defaults value must be "FuncName" or "import/path.FuncName"`,
)

// goTemplatesFS is the Go adapter's template tree. The Go
// backend reads it once at Build time and registers every
// `*.tmpl` it finds under `templates/golang/` by base
// filename.
//
//go:embed templates/golang/*.tmpl
var goTemplatesFS embed.FS

// GoOutputs returns the Go adapter's output set — a single
// `<basename>_builder.go` file per source-file the Layout
// phase routes contributions to.
func GoOutputs() []sdk.Output {
	return []sdk.Output{{Suffix: GoSuffix}}
}

// goSupport is everything this plugin declares for Go: the template
// tree, the output set, and the one helper those templates call.
//
// The plugin's core names no language and reads this as a pair, so
// a second target language is a sibling file and one more For call
// rather than an edit to what the plugin *is*.
func goSupport() (string, sdk.LanguageSupport) {
	lang, s := sdkgo.Support(goTemplatesFS, GoOutputs()...)
	s.Funcs = template.FuncMap{"defaultsExpr": GoDefaultsExpr}
	return lang, s
}

// GoDefaultsExpr parses a `defaults=` value into an
// [sdk.External] expression suitable for the rendered
// `New<Name>WithDefaults` body. Two forms are accepted:
//
//   - A bare identifier — `defaults=defaultUser` — naming a
//     function beside the annotated struct. It resolves
//     against srcPkg, the source struct's package. This is the
//     common case, and the one a source author writes without
//     thinking about it.
//   - `<import-path>.<FuncName>` — `defaults=example.com/x.New`
//     — naming a factory in another package.
//
// Both render through [sdk.External], so the same-package
// elision rule picks the spelling: a builder emitted alongside
// its source renders the bare name, and one redirected by
// `out=` / `pkg=` renders it qualified against the import the
// backend registers. Resolving the bare form to a package
// rather than emitting a bare identifier is what makes the
// redirected case work at all — and when the named factory is
// unexported, turns a reference that would bind to nothing
// into a compile error naming the symbol.
//
// Malformed input returns [ErrMalformedDefaults] wrapped with the
// offending value, surfaced as a render-time error. Both halves are
// checked, not just the split: the function half must be a Go
// identifier and the package half a legal import path. Testing only
// for a leading or trailing dot accepted `defaults=3.14` and emitted
// a reference to the symbol `14` in the package `3` — a value that is
// not a function name at all, rendered as though it were.
//
// The template guards the call with `{{if .DefaultsArg}}` so
// the empty case never reaches the parser under normal
// rendering; the empty-input rejection is defence-in-depth
// for direct callers.
//
// The parser is plugin-local because the two-form convention
// is specific to the builder's `defaults=` arg; shared
// identifier-convention helpers live in
// [go.thesmos.sh/eidos/lang/golang]. The base registers this
// one under the plugin's name prefix, so the template calls it
// as `builder_defaultsExpr`.
func GoDefaultsExpr(raw, srcPkg string) (*sdk.Expr, error) {
	malformed := func() (*sdk.Expr, error) {
		return nil, fmt.Errorf("%w (got %q)", ErrMalformedDefaults, raw)
	}
	i := strings.LastIndex(raw, ".")
	if i < 0 {
		if !token.IsIdentifier(raw) {
			return malformed()
		}
		return sdk.NewExternal(srcPkg, raw), nil
	}
	pkg, fn := raw[:i], raw[i+1:]
	if !golang.IsValidImportPath(pkg) || !token.IsIdentifier(fn) {
		return malformed()
	}
	return sdk.NewExternal(pkg, fn), nil
}
