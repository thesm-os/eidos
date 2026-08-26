// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"fmt"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/plugin"
)

// Render implements [plugin.Backend].
//
// Every non-empty target becomes one file. A target that fails to
// render attaches an Error diagnostic and the loop continues, so one
// broken template reports itself rather than hiding every other
// file's problems behind the first failure. Only a sink write that
// fails aborts the run: at that point nothing further can be
// delivered.
func (b *Backend) Render(ctx *plugin.BackendContext) error {
	if b.tmplErr != nil {
		return b.tmplErr
	}
	ps := ctx.Diag.For(Name)

	for _, group := range groupByTarget(ctx.Store) {
		body, ok := b.renderTarget(group, ps)
		if !ok {
			continue
		}
		if err := ctx.Sink.Write(group.target, body); err != nil {
			return fmt.Errorf("backend: write %s: %w", group.target.JoinPath(), err)
		}
	}
	return nil
}

// renderTarget renders one file, reporting whether it produced
// anything to write.
func (b *Backend) renderTarget(group targetGroup, ps *diag.PluginSink) ([]byte, bool) {
	pos := position.Pos{File: group.target.JoinPath()}

	state, err := newRenderState(b.tmpl, group.target, ps)
	if err != nil {
		ps.Errorf(pos, "%v", err)
		return nil, false
	}

	body, err := state.renderBody(group.decls)
	if err != nil {
		ps.Errorf(pos, "%v", err)
		return nil, false
	}
	if body == "" {
		// A target whose entities all rendered to nothing is not an
		// error and is not a file: writing an empty one would leave a
		// stub in the tree that the next run would have to prune.
		return nil, false
	}

	return finalise(header(group.target) + body + footer()), true
}
