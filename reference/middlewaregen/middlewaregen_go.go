// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package middlewaregen

import (
	"embed"

	"go.thesmos.sh/eidos/sdk"
)

// GoSuffix is the per-source trailer Layout appends to compose this
// plugin's filename.
//
// A contributor wanting to place its own declarations into this same
// file declares an Output with this exact suffix — sharing the suffix
// is the coupling, and there is no other mechanism. Contributors that
// only fill the chain slot need no Output at all.
const GoSuffix = "_middleware.go"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// GoOutputs declares the single file this plugin owns.
//
// Exported rather than inlined into [New] so a contributor sharing
// the file can name the same declaration instead of re-spelling
// [GoSuffix] in a literal the two could drift apart on.
func GoOutputs() []sdk.Output {
	return []sdk.Output{{Suffix: GoSuffix}}
}
