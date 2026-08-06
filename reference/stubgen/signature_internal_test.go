// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package stubgen

import (
	"testing"

	"go.thesmos.sh/eidos/node"
)

// method builds a [node.Method] from parameter and return names.
// An empty name means the source did not name that slot.
func method(t *testing.T, params, returns []string) *node.Method {
	t.Helper()
	str := func() *node.TypeRef {
		return &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"}
	}
	m := &node.Method{Name: "Do"}
	for _, p := range params {
		m.Params = append(m.Params, &node.Param{Name: p, Type: str()})
	}
	for _, r := range returns {
		m.Returns = append(m.Returns, &node.Return{Name: r, Type: str()})
	}
	return m
}

// TestNamedReturnsUsable pins the all-or-nothing rule for
// propagating a source signature's return names onto the generated
// method.
//
// The rule is whitebox-tested because both reasons it exists are
// invisible from the rendered output: a mixed slice fails the render
// with emit.ErrMixedNamedReturns rather than producing something a
// golden file would show, and a name colliding with the receiver
// produces code that does not compile rather than code that looks
// wrong.
func TestNamedReturnsUsable(t *testing.T) {
	t.Parallel()

	t.Run("all returns named is usable", func(t *testing.T) {
		t.Parallel()
		if !namedReturnsUsable(method(t, []string{"ctx"}, []string{"item", "err"})) {
			t.Errorf("fully named returns should propagate")
		}
	})

	t.Run("no returns is not usable", func(t *testing.T) {
		t.Parallel()
		// There is nothing to name, and declaring named results on an
		// empty list is not valid Go.
		if namedReturnsUsable(method(t, []string{"ctx"}, nil)) {
			t.Errorf("a void signature cannot carry named results")
		}
	})

	t.Run("a mixed slice is not usable", func(t *testing.T) {
		t.Parallel()
		// `(_ User, err error)` is legal Go and normalises to one
		// named and one unnamed slot; Go requires results to be all
		// named or all anonymous, so the whole signature drops back.
		if namedReturnsUsable(method(t, nil, []string{"", "err"})) {
			t.Errorf("a partly named return list must fall back to anonymous")
		}
	})

	t.Run("a return colliding with the receiver is not usable", func(t *testing.T) {
		t.Parallel()
		// The generated method binds its receiver to `s`; a result
		// named `s` shadows it and the body stops compiling.
		if namedReturnsUsable(method(t, nil, []string{receiverIdent})) {
			t.Errorf("a return named %q collides with the receiver", receiverIdent)
		}
	})

	t.Run("a return colliding with a parameter is not usable", func(t *testing.T) {
		t.Parallel()
		if namedReturnsUsable(method(t, []string{"id"}, []string{"id"})) {
			t.Errorf("a return sharing a parameter name does not compile")
		}
	})
}

// TestWithLocals pins the identifiers the generated body binds each
// return slot to when capturing the delegate's result.
func TestWithLocals(t *testing.T) {
	t.Parallel()

	t.Run("named returns bind to their declared name", func(t *testing.T) {
		t.Parallel()
		// The signature already declares them, so introducing a
		// second identifier would shadow the result being returned.
		got := withLocals([]Return{{Name: "item"}, {Name: "err"}}, nil, true)
		if got[0].Local != "item" || got[1].Local != "err" {
			t.Errorf("locals = %q/%q, want item/err", got[0].Local, got[1].Local)
		}
	})

	t.Run("anonymous returns bind to positional locals", func(t *testing.T) {
		t.Parallel()
		got := withLocals([]Return{{}, {}}, nil, false)
		if got[0].Local != "r0" || got[1].Local != "r1" {
			t.Errorf("locals = %q/%q, want r0/r1", got[0].Local, got[1].Local)
		}
	})

	t.Run("a positional local colliding with a parameter is prefixed", func(t *testing.T) {
		t.Parallel()
		// Shadowing a parameter would record the wrong value into the
		// call log, which is the one thing this stub exists to do.
		got := withLocals([]Return{{}}, []Param{{Name: "r0"}}, false)
		if got[0].Local != "_r0" {
			t.Errorf("local = %q, want _r0", got[0].Local)
		}
	})
}
