// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"bytes"
	"fmt"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/emit"
)

// renderState is the per-target rendering context: a cloned template
// tree whose funcmap closures bind this state, and the import set
// every type the target names accumulates into.
//
// One per target, and that is what makes concurrent rendering safe by
// construction rather than by locking. The parent tree is parsed once
// and never mutated; a clone shares the parse tree and owns its
// funcmap, so two targets rendering at once cannot see each other's
// imports.
type renderState struct {
	tmpl    *template.Template
	imports *importSet
	target  emit.Target
	ps      *diag.PluginSink
}

// newRenderState clones parent and binds a fresh state to it.
func newRenderState(parent *template.Template, target emit.Target, ps *diag.PluginSink) (*renderState, error) {
	clone, err := parent.Clone()
	if err != nil {
		return nil, fmt.Errorf("backend: clone templates: %w", err)
	}

	s := &renderState{
		imports: newImportSet(target.ImportPath),
		target:  target,
		ps:      ps,
	}
	s.tmpl = clone.Funcs(s.funcMap())
	return s, nil
}

// render dispatches one emit entity through the template registered
// for its kind.
//
// Templates are named verbatim from [emit.Node.Kind], so a plugin
// shipping its own kind ships a template defining that kind's name
// and this dispatch finds it without knowing anything about it.
func (s *renderState) render(n emit.Node) (string, error) {
	if n == nil {
		return "", nil
	}
	name := string(n.Kind())
	tmpl := s.tmpl.Lookup(name)
	if tmpl == nil {
		return "", fmt.Errorf("%w: %s", ErrTemplateMissing, name)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, n); err != nil {
		return "", fmt.Errorf("backend: render %s: %w", name, err)
	}
	return buf.String(), nil
}
