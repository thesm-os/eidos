// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"fmt"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// recorder is a [golang.Reporter] that keeps what it was told, so a
// test can assert on severity as well as on wording.
type recorder struct {
	warns  []string
	errors []string
	poss   []position.Pos
}

func (r *recorder) Warnf(pos position.Pos, format string, args ...any) {
	r.warns = append(r.warns, fmt.Sprintf(format, args...))
	r.poss = append(r.poss, pos)
}

func (r *recorder) Errorf(pos position.Pos, format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
	r.poss = append(r.poss, pos)
}

// issueOver builds a result carrying one embed that contributed
// nothing, positioned so the reported location can be asserted on.
func issueOver(reason node.MethodSetReason) node.MethodSetResult {
	embed := &node.Embed{
		Type: &node.TypeRef{
			TypeKind: node.TypeRefNamed, Package: "io", Name: "Closer",
		},
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: "store.go", Line: 12}},
	}
	return node.MethodSetResult{Issues: []node.MethodSetIssue{{Embed: embed, Reason: reason}}}
}

func iface() *node.Interface {
	return &node.Interface{Name: "Store", Package: "example.com/store"}
}

// TestReportMethodSet covers the half of method-set resolution that is
// a generator's rather than the model's: what a caller is told, and
// whether it may emit.
//
// The severity split is the substance. An unloaded embed and a
// non-interface one are both short sets, but only one of them is the
// author's fault — the same source under a wider pattern resolves —
// and reporting both as errors tells an author to fix something they
// did not do wrong.
func TestReportMethodSet(t *testing.T) {
	t.Parallel()

	why := golang.Consequence{
		Partial:    "the double carries what was already contributed",
		Incomplete: "a double missing a method cannot stand in",
	}

	t.Run("a clean set reports nothing and is usable", func(t *testing.T) {
		t.Parallel()
		var r recorder
		if !golang.ReportMethodSet(&r, node.MethodSetResult{}, iface(), "stub", why) {
			t.Error("a set with no issues was reported unusable")
		}
		if len(r.warns)+len(r.errors) != 0 {
			t.Errorf("reported %v / %v over a clean set", r.warns, r.errors)
		}
	})

	t.Run("an unloaded embed warns and stops generation", func(t *testing.T) {
		t.Parallel()
		// A wider pattern resolves the same source, so the author has
		// written nothing wrong — but the set is genuinely short, so
		// nothing may be emitted from it.
		var r recorder
		if golang.ReportMethodSet(&r, issueOver(node.ReasonUnresolved), iface(), "stub", why) {
			t.Error("an unloaded embed was reported usable")
		}
		if len(r.warns) != 1 || len(r.errors) != 0 {
			t.Fatalf("got %d warnings and %d errors, want 1 and 0", len(r.warns), len(r.errors))
		}
		if !strings.Contains(r.warns[0], why.Incomplete) {
			t.Errorf("the warning does not end on the caller's consequence: %q", r.warns[0])
		}
	})

	t.Run("a cycle warns and does not stop generation", func(t *testing.T) {
		t.Parallel()
		// The walk broke out only after the interface it points back at
		// had contributed, so the set is short of nothing.
		var r recorder
		if !golang.ReportMethodSet(&r, issueOver(node.ReasonCyclic), iface(), "stub", why) {
			t.Error("a cycle was reported unusable, though the set is complete")
		}
		if len(r.warns) != 1 || len(r.errors) != 0 {
			t.Fatalf("got %d warnings and %d errors, want 1 and 0", len(r.warns), len(r.errors))
		}
		if !strings.Contains(r.warns[0], why.Partial) {
			t.Errorf("the cycle warning does not carry the partial consequence: %q", r.warns[0])
		}
	})

	t.Run("anything else is an error", func(t *testing.T) {
		t.Parallel()
		// No run of any width resolves an embed that is not an
		// interface, so there is nothing for the author to widen.
		for _, reason := range []node.MethodSetReason{node.ReasonNonInterface, node.ReasonGeneric} {
			var r recorder
			if golang.ReportMethodSet(&r, issueOver(reason), iface(), "stub", why) {
				t.Errorf("%v was reported usable", reason)
			}
			if len(r.errors) != 1 || len(r.warns) != 0 {
				t.Errorf("%v gave %d errors and %d warnings, want 1 and 0",
					reason, len(r.errors), len(r.warns))
			}
		}
	})

	t.Run("the embed is named as the source spelled it", func(t *testing.T) {
		t.Parallel()
		// The improvement over the bare name the reference carries:
		// `Closer` is not something an author can search their file for,
		// and two embedded packages can both declare one.
		var r recorder
		golang.ReportMethodSet(&r, issueOver(node.ReasonUnresolved), iface(), "stub", why)
		if !strings.Contains(r.warns[0], `"io.Closer"`) {
			t.Errorf("got %q, want the qualified spelling io.Closer", r.warns[0])
		}
	})

	t.Run("the diagnostic points at the embed, not the interface", func(t *testing.T) {
		t.Parallel()
		// An interface embedding several is one position; the embed is
		// the line the author has to change.
		var r recorder
		golang.ReportMethodSet(&r, issueOver(node.ReasonUnresolved), iface(), "stub", why)
		if got := r.poss[0]; got.File != "store.go" || got.Line != 12 {
			t.Errorf("reported at %v, want the embed's own position", got)
		}
	})

	t.Run("a nil sink reports the set as usable", func(t *testing.T) {
		t.Parallel()
		// An absent sink is a caller mistake this package cannot
		// diagnose. Reporting the set unusable would stop a generator
		// for a fault that is not in its source.
		if !golang.ReportMethodSet(nil, issueOver(node.ReasonNonInterface), iface(), "stub", why) {
			t.Error("a nil sink made the set unusable")
		}
	})
}
