// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
)

// exprState returns a render state a bare expression or type can be
// spelled through.
func exprState(t *testing.T) *renderState {
	t.Helper()
	s, err := newRenderState(New().tmpl, emit.Target{ImportPath: "./self"}, nil)
	if err != nil {
		t.Fatalf("newRenderState: %v", err)
	}
	return s
}

func TestRenderStateClone(t *testing.T) {
	t.Parallel()

	t.Run("each target gets its own import set", func(t *testing.T) {
		t.Parallel()
		// The clone-per-target pattern is what makes concurrent
		// rendering safe by construction rather than by locking: two
		// targets rendering at once cannot see each other's imports.
		parent := New().tmpl

		first, err := newRenderState(parent, emit.Target{ImportPath: "./a"}, nil)
		if err != nil {
			t.Fatalf("newRenderState: %v", err)
		}
		second, err := newRenderState(parent, emit.Target{ImportPath: "./b"}, nil)
		if err != nil {
			t.Fatalf("newRenderState: %v", err)
		}

		first.imports.Named("./x", "X", true)
		if second.imports.Len() != 0 {
			t.Fatal("one target's import reached another's file")
		}
		if first.tmpl == second.tmpl {
			t.Fatal("two targets share one template tree")
		}
	})
}

func TestRenderDispatch(t *testing.T) {
	t.Parallel()

	t.Run("a kind with no template is refused by name", func(t *testing.T) {
		t.Parallel()
		// A plugin that shipped an emit kind without its template
		// learns which one from the diagnostic rather than a stack
		// trace.
		s := exprState(t)
		_, err := s.render(&emit.Method{Name: "m"})
		if !errors.Is(err, ErrTemplateMissing) {
			t.Fatalf("render = %v, want ErrTemplateMissing", err)
		}
		if !strings.Contains(err.Error(), string(emit.KindMethod)) {
			t.Fatalf("error does not name the kind: %v", err)
		}
	})

	t.Run("a nil node renders nothing", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		if got, err := s.render(nil); got != "" || err != nil {
			t.Fatalf("render(nil) = %q, %v", got, err)
		}
	})
}
