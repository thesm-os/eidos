// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"embed"
	"fmt"
	"text/template"

	"go.thesmos.sh/eidos/emit"
)

// canonical holds the template set this backend ships.
//
//go:embed templates/*.tmpl
var canonical embed.FS

// placeholders satisfies the parser's name-and-arity check at parse
// time.
//
// text/template validates that every function a template calls is
// registered when the tree is parsed, but the real helpers bind to a
// per-target render state that does not exist yet. The parent tree is
// parsed against these and every clone replaces them, so a
// placeholder never runs — and returning an error rather than a value
// is what makes that true loudly if one ever does.
func placeholders() template.FuncMap {
	unbound := func(name string) any {
		return func(...any) (string, error) {
			return "", fmt.Errorf("backend: %s called on the unbound parent template", name)
		}
	}
	names := []string{
		"render", "renderType", "renderDocs", "renderMembers", "renderMethods",
		"renderParams", "renderReturn", "renderTypeParams", "renderHeritage",
		"renderVariants", "quote", "ident", "propKey", "exported", "indent",
		"abstractKw", "constKw", "signatures", "renderInit", "annotation",
		"overloadLines", "camel", "pascal", "scream", "meta", "metaBool",
	}
	out := make(template.FuncMap, len(names))
	for _, n := range names {
		out[n] = unbound(n)
	}
	return out
}

// loadTemplates parses the canonical set.
//
// The templates are embedded, so a parse error means this package
// shipped broken rather than that a user did anything — but a library
// still may not take the process down over it. [New] keeps the error
// and [Backend.Render] returns it, which puts the failure where the
// pipeline already handles one.
func loadTemplates() (*template.Template, error) {
	t, err := template.New("typescript").Funcs(placeholders()).ParseFS(canonical, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("backend: parse canonical templates: %w", err)
	}
	return t, nil
}

// renderableKinds lists the emit kinds the canonical set renders.
//
// Used to tell "this backend does not render that" from "the graph
// held something unexpected", so a target carrying only sub-elements
// is reported as empty rather than as a missing template.
var renderableKinds = map[string]struct{}{ //nolint:gochecknoglobals // immutable lookup table
	string(emit.KindStruct):    {},
	string(emit.KindInterface): {},
	string(emit.KindEnum):      {},
	string(emit.KindAlias):     {},
	string(emit.KindFunction):  {},
	string(emit.KindVariable):  {},
	string(emit.KindConstant):  {},
}

// renderable reports whether the canonical set renders n's kind.
func renderable(n emit.Node) bool {
	if n == nil {
		return false
	}
	_, ok := renderableKinds[string(n.Kind())]
	return ok
}
