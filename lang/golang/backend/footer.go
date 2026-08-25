// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"go.thesmos.sh/eidos/plugin"
)

// renderFooter produces the trailing comment block written at the
// end of every generated file. The standard block carries two
// lines: the end-of-generated-content marker on its own line, then
// the provenance hash on its own line. Both lines carry the brand
// prefix so embedders that re-brand their output produce a footer
// whose every line is unambiguously theirs. Splitting the marker
// from the hash keeps each statement scannable on its own and lets
// tools grep `^// <brand>:provenance ` to recover the hash without
// parsing a composite line.
//
// Library embedders extend the footer via
// [plugin.BackendContext.FooterSuffix] — additional lines emitted
// verbatim after the standard two-line block. Use for org-specific
// signing markers or additional trailing metadata.
//
// The output starts with a blank line so the footer separates
// cleanly from the rendered declarations above it; each line ends
// with `\n` so the file terminates per Go convention. The
// provenance hash is SHA-256 of the body bytes (the byte slice
// supplied here), rendered as lowercase hex.
func renderFooter(ctx *plugin.BackendContext, body []byte) string {
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	brand := brandOf(ctx)
	var b strings.Builder
	b.WriteByte('\n')
	fmt.Fprintf(&b, "// %s: end of generated content.\n", brand)
	fmt.Fprintf(&b, "// %s:provenance %s\n", brand, hash)
	for _, line := range ctx.FooterSuffix {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
