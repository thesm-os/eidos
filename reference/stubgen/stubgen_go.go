// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package stubgen

import (
	"embed"

	"go.thesmos.sh/eidos/sdk"
)

// GoPrimarySuffix is the per-source-basename trailer for the primary
// output. Interfaces declared in `store.go` produce `store_stub.go`.
const GoPrimarySuffix = "_stub.go"

// GoTestSuffix is the trailer for the tagged test output. The
// `_test.go` ending triggers the framework's automatic `<pkg>_test`
// package shift, so the generated test lands in an external test
// package and cannot reach package-private state.
const GoTestSuffix = "_stub_test.go"

// GoTestOutputTag is the tag the companion output advertises.
// Source-side `+gen:out tag=test …` directives, project config under
// the plugin's `tags:` block, and CLI `-o stubgen:test=…` overrides
// all match against this value.
const GoTestOutputTag = "test"

// goTemplates is the tree [New] hands the plugin base, which roots
// it at the conventional `templates/golang` and hands the backend
// every `*.tmpl` under it. The backend reads it once at Build time.
//
//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// GoOutputs returns the Go adapter's output set: the primary
// `<basename>_stub.go` plus the `test`-tagged companion.
//
// The untagged entry comes first because the framework reserves the
// empty tag for a plugin's primary output and requires it at index 0
// when present.
func GoOutputs() []sdk.Output {
	return []sdk.Output{
		{Tag: "", Suffix: GoPrimarySuffix},
		{Tag: GoTestOutputTag, Suffix: GoTestSuffix},
	}
}
