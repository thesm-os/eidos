// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store

import (
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
)

// diagName is the plugin identifier store attributes its diagnostics
// to. Store is not a plugin, but the diagnostic surface is keyed by
// name and "store" is what a reader needs to see.
const diagName = "store"

// SetDiag installs the sink the store reports through.
//
// Store has no diagnostics of its own to report about the data it
// holds — it is a container, and its errors are returned rather than
// emitted. The one thing it does report is a caller mistake it can
// otherwise only answer with silence: reading an index that is
// switched off. See [NodeView.ByMetaKey].
//
// Optional by design. A nil sink — the default, and what every
// library and test caller gets — disables the notice rather than
// panicking, so a store constructed outside a pipeline behaves
// exactly as it did before the sink existed. The pipeline installs
// one when it builds the run's store.
func (s *Store) SetDiag(d *diag.Sink) {
	s.nodes.diag = d
	s.emit.diag = d
}

// warnDisabledMetaIndex reports that ByMetaKey was consulted while
// the by-metadata-key index is disabled.
//
// Info rather than Warn: nothing is wrong yet. The index answers with
// what it saw at ingest, which is correct for the majority of callers
// and complete for any caller that does not stamp metadata after
// ingest. The message exists because the alternative — an out-of-tree
// annotator discovering the change by diffing its generated output —
// is the worst way to learn it.
func (v *NodeView) warnDisabledMetaIndex(enable string) {
	emitDisabledMetaIndex(v.diag, enable)
}

// warnDisabledMetaIndex mirrors [NodeView.warnDisabledMetaIndex] for
// the emit side.
func (v *EmitView) warnDisabledMetaIndex(enable string) {
	emitDisabledMetaIndex(v.diag, enable)
}

// emitDisabledMetaIndex writes the notice, or does nothing when no
// sink is installed.
func emitDisabledMetaIndex(d *diag.Sink, enable string) {
	if d == nil {
		return
	}
	d.For(diagName).Infof(position.Pos{},
		"ByMetaKey consulted while the metadata-key index is disabled; "+
			"it holds only keys present at ingest — call %s to record later writes", enable)
}
