// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package middlewaregen

import (
	"embed"
	"io/fs"
	"text/template"

	"go.thesmos.sh/eidos/lang/golang"
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
func GoOutputs() []sdk.Output {
	return []sdk.Output{{Suffix: GoSuffix}}
}

// GoTemplates returns the embedded Go template set.
func GoTemplates() (fs.FS, bool) {
	sub, err := fs.Sub(goTemplates, "templates/golang")
	if err != nil {
		return nil, false
	}
	return sub, true
}

// GoFuncMap contributes the shared Go ref-conversion helpers.
func GoFuncMap() template.FuncMap { return golang.FuncMap() }
