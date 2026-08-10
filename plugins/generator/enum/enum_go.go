// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package enum

import (
	"embed"

	"go.thesmos.sh/eidos/sdk"
)

// GoPrimarySuffix is the per-source-basename suffix the Go
// adapter reports for the primary output. Source enums
// declared in `status.go` produce `status_enum.go`.
const GoPrimarySuffix = "_enum.go"

// GoTestSuffix is the per-source-basename suffix the Go
// adapter reports for the test-tagged output. The
// `_test.go` ending triggers the Go backend's automatic
// `<pkg>_test` package shift so the rendered tests live in
// an external test package.
const GoTestSuffix = "_enum_test.go"

// GoTestOutputTag is the tag the test-tagged output
// advertises. Source-side `+gen:out tag=test …` directives
// and CLI `-o enum:test=…` overrides match against this
// value.
const GoTestOutputTag = "test"

// GoDefaultParsePrefix is the Go-idiomatic prefix appended
// to the enum's type name to form the parse function's
// identifier when [Options.ParsePrefix] is unset.
// `ParseStatus` matches the canonical Go pattern.
const GoDefaultParsePrefix = "Parse"

// GoDefaultSentinelPrefix is the Go-idiomatic prefix
// appended to the enum's type name to form the parse-error
// sentinel's identifier when [Options.SentinelPrefix] is
// unset. `ErrUnknownStatus` reads as a typed sentinel
// callers compare via [errors.Is].
const GoDefaultSentinelPrefix = "ErrUnknown"

// goTemplatesFS is the Go adapter's template tree. The Go
// backend reads it once at Build time and registers every
// `*.tmpl` it finds under `templates/golang/` by base
// filename.
//
//go:embed templates/golang/*.tmpl
var goTemplatesFS embed.FS

// GoOutputs returns the Go adapter's output set — the
// primary `<basename>_enum.go` file plus the
// "test"-tagged `<basename>_enum_test.go` companion.
func GoOutputs() []sdk.Output {
	return []sdk.Output{
		{Suffix: GoPrimarySuffix},
		{Tag: GoTestOutputTag, Suffix: GoTestSuffix},
	}
}
