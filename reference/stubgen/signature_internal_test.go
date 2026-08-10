// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package stubgen

import (
	"testing"

	"go.thesmos.sh/eidos/sdk"
)

// method builds a [sdk.Method] from parameter and return names.
// An empty name means the source did not name that slot.
func method(t *testing.T, params, returns []string) *sdk.Method {
	t.Helper()
	str := func() *sdk.TypeRef {
		return &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "string"}
	}
	m := &sdk.Method{Name: "Do"}
	for _, p := range params {
		m.Params = append(m.Params, &sdk.Param{Name: p, Type: str()})
	}
	for _, r := range returns {
		m.Returns = append(m.Returns, &sdk.Return{Name: r, Type: str()})
	}
	return m
}

// TestNamedReturnsUsable pins the all-or-nothing rule for
// propagating a source signature's return names onto the generated
// method.
//
// The rule is whitebox-tested because both reasons it exists are
// invisible from the rendered output: a mixed slice fails the render
// with sdk.ErrMixedNamedReturns rather than producing something a
// golden file would show, and a name colliding with the receiver
// produces code that does not compile rather than code that looks
// wrong.
func TestNamedReturnsUsable(t *testing.T) {
	t.Parallel()

	t.Run("all returns named is usable", func(t *testing.T) {
		t.Parallel()
		m := method(t, []string{"ctx"}, []string{"item", "err"})
		if !namedReturnsUsable(m, "s", paramsOf(m)) {
			t.Errorf("fully named returns should propagate")
		}
	})

	t.Run("no returns is not usable", func(t *testing.T) {
		t.Parallel()
		// There is nothing to name, and declaring named results on an
		// empty list is not valid Go.
		m := method(t, []string{"ctx"}, nil)
		if namedReturnsUsable(m, "s", paramsOf(m)) {
			t.Errorf("a void signature cannot carry named results")
		}
	})

	t.Run("a mixed slice is not usable", func(t *testing.T) {
		t.Parallel()
		// `(_ User, err error)` is legal Go and normalises to one
		// named and one unnamed slot; Go requires results to be all
		// named or all anonymous, so the whole signature drops back.
		m := method(t, nil, []string{"", "err"})
		if namedReturnsUsable(m, "s", paramsOf(m)) {
			t.Errorf("a partly named return list must fall back to anonymous")
		}
	})

	t.Run("a return colliding with the receiver is not usable", func(t *testing.T) {
		t.Parallel()
		// A result named after the receiver shadows it and the body
		// stops compiling.
		m := method(t, nil, []string{"s"})
		if namedReturnsUsable(m, "s", paramsOf(m)) {
			t.Errorf("a return named %q collides with the receiver", "s")
		}
	})

	t.Run("a return colliding with a parameter is not usable", func(t *testing.T) {
		t.Parallel()
		m := method(t, []string{"id"}, []string{"id"})
		if namedReturnsUsable(m, "s", paramsOf(m)) {
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
		got := withLocals([]Return{{Name: "item"}, {Name: "err"}}, nil, "s", true)
		if got[0].Local != "item" || got[1].Local != "err" {
			t.Errorf("locals = %q/%q, want item/err", got[0].Local, got[1].Local)
		}
	})

	t.Run("anonymous returns bind to positional locals", func(t *testing.T) {
		t.Parallel()
		got := withLocals([]Return{{}, {}}, nil, "s", false)
		if got[0].Local != "r0" || got[1].Local != "r1" {
			t.Errorf("locals = %q/%q, want r0/r1", got[0].Local, got[1].Local)
		}
	})

	t.Run("a positional local colliding with a parameter is prefixed", func(t *testing.T) {
		t.Parallel()
		// Shadowing a parameter would record the wrong value into the
		// call log, which is the one thing this stub exists to do.
		got := withLocals([]Return{{}}, []Param{{Name: "r0"}}, "s", false)
		if got[0].Local != "_r0" {
			t.Errorf("local = %q, want _r0", got[0].Local)
		}
	})
}

// TestReceiverIdentFor pins the receiver identifier the generated
// methods bind to, and the one case where it moves.
//
// Whitebox because the failure it prevents is a compile error in
// generated code rather than a wrong-looking rendering: an interface
// declaring `Recv(s string)` gave `func (s *StoreStub) Recv(s string)`,
// where every `s.<Field>` in the body resolved to the parameter.
func TestReceiverIdentFor(t *testing.T) {
	t.Parallel()

	t.Run("the receiver is the stub type's initial", func(t *testing.T) {
		t.Parallel()
		if got := receiverIdentFor("StoreStub", []Param{{Name: "id"}}); got != "s" {
			t.Errorf("receiver = %q, want s", got)
		}
	})

	t.Run("a parameter holding the initial moves the receiver", func(t *testing.T) {
		t.Parallel()
		// The source names the parameters and this generator names
		// nothing, so the receiver is what has to give way.
		got := receiverIdentFor("StoreStub", []Param{{Name: "s"}})
		if got == "s" {
			t.Fatalf("receiver = %q, which the parameter already holds", got)
		}
	})

	t.Run("a suffix that yields no letter still names the receiver", func(t *testing.T) {
		t.Parallel()
		// `suffix` is user-supplied and the interface name is the
		// source's; neither is guaranteed to start with a letter.
		if got := receiverIdentFor("_42", nil); got == "" {
			t.Errorf("receiver is empty; the generated method would not parse")
		}
	})
}

// TestParamsOf_VariadicSurvives pins the flag the recorded double's
// interface satisfaction depends on.
//
// A dropped variadic marker renders `Put(opts string)` where the
// source declared `Put(opts ...string)`. That compiles standalone and
// satisfies nothing, which is why it survived every substring
// assertion the plugin had.
func TestParamsOf_VariadicSurvives(t *testing.T) {
	t.Parallel()

	str := &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "string"}
	m := &sdk.Method{Name: "Put", Params: []*sdk.Param{
		{Name: "id", Type: str},
		{Name: "opts", Type: str, Variadic: true},
	}}

	got := paramsOf(m)
	if got[0].Variadic {
		t.Errorf("leading parameter %q reported variadic", got[0].Name)
	}
	if !got[1].Variadic {
		t.Errorf("trailing parameter %q lost its variadic marker", got[1].Name)
	}
}
