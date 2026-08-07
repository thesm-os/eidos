// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// errorRef returns a reference to the predeclared error interface,
// stamped the way a Go frontend stamps it.
func errorRef() *node.TypeRef {
	r := builtinRef("error")
	golang.MetaIsError.Set(r.EnsureMeta(), true, "test")
	return r
}

func TestParamIdent(t *testing.T) {
	t.Parallel()

	t.Run("a declared name is kept", func(t *testing.T) {
		t.Parallel()
		// Renaming beyond necessity breaks the correspondence between
		// the source signature and the generated body.
		if got := golang.ParamIdent(&node.Param{Name: "buf"}, 0); got != "buf" {
			t.Fatalf("ParamIdent = %q, want buf", got)
		}
	})

	t.Run("a reserved name is made safe", func(t *testing.T) {
		t.Parallel()
		// `func Convert(type string)` is not legal Go, but a name that
		// is merely predeclared — `len`, `copy`, `any` — is, and a body
		// binding it shadows the builtin.
		if got := golang.ParamIdent(&node.Param{Name: "len"}, 0); got != "len_" {
			t.Fatalf("ParamIdent = %q, want len_", got)
		}
	})

	t.Run("an anonymous parameter falls back to its position", func(t *testing.T) {
		t.Parallel()
		// `Read([]byte)` is a legal signature; a body referencing the
		// parameter still has to call it something.
		if got := golang.ParamIdent(&node.Param{}, 3); got != "arg3" {
			t.Fatalf("ParamIdent = %q, want arg3", got)
		}
	})

	t.Run("a blank parameter falls back to its position", func(t *testing.T) {
		t.Parallel()
		if got := golang.ParamIdent(&node.Param{Name: "_"}, 1); got != "arg1" {
			t.Fatalf("ParamIdent = %q, want arg1", got)
		}
	})

	t.Run("a nil parameter falls back rather than panicking", func(t *testing.T) {
		t.Parallel()
		// Called from templates and per-parameter loops, where a nil is
		// a data gap and a panic surfaces as a framework fault against
		// the caller's plugin.
		if got := golang.ParamIdent(nil, 2); got != "arg2" {
			t.Fatalf("ParamIdent(nil) = %q, want arg2", got)
		}
	})
}

func TestParamIdents(t *testing.T) {
	t.Parallel()

	t.Run("names a whole list in order", func(t *testing.T) {
		t.Parallel()
		got := golang.ParamIdents([]*node.Param{{Name: "ctx"}, {}, {Name: "opts"}})
		if !slices.Equal(got, []string{"ctx", "arg1", "opts"}) {
			t.Fatalf("ParamIdents = %v, want [ctx arg1 opts]", got)
		}
	})

	t.Run("a declared name cannot collide with a positional fallback", func(t *testing.T) {
		t.Parallel()
		// `Read(arg1 []byte, []byte)` names the first parameter exactly
		// what the second falls back to, and two parameters of one name
		// do not compile.
		got := golang.ParamIdents([]*node.Param{{Name: "arg1"}, {}})
		if got[0] == got[1] {
			t.Fatalf("ParamIdents = %v, want distinct identifiers", got)
		}
	})

	t.Run("two parameters of one declared name are separated", func(t *testing.T) {
		t.Parallel()
		got := golang.ParamIdents([]*node.Param{{Name: "v"}, {Name: "v"}})
		if got[0] != "v" || got[1] == "v" {
			t.Fatalf("ParamIdents = %v, want the second renamed", got)
		}
	})

	t.Run("an empty list names nothing", func(t *testing.T) {
		t.Parallel()
		if got := golang.ParamIdents(nil); len(got) != 0 {
			t.Fatalf("ParamIdents(nil) = %v, want empty", got)
		}
	})
}

func TestErrorSlot(t *testing.T) {
	t.Parallel()

	t.Run("finds the conventional trailing slot", func(t *testing.T) {
		t.Parallel()
		rets := []*node.Return{{Type: builtinRef("string")}, {Type: errorRef()}}
		if got := golang.ErrorSlot(rets); got != 1 {
			t.Fatalf("ErrorSlot = %d, want 1", got)
		}
	})

	t.Run("finds an error declared first", func(t *testing.T) {
		t.Parallel()
		// `(error, string)` is unusual but legal, and a positional rule
		// binds the wrong slot without failing to compile — the
		// generated code checks a string for nil-ness.
		rets := []*node.Return{{Type: errorRef()}, {Type: builtinRef("string")}}
		if got := golang.ErrorSlot(rets); got != 0 {
			t.Fatalf("ErrorSlot = %d, want 0", got)
		}
	})

	t.Run("takes the first when several are declared", func(t *testing.T) {
		t.Parallel()
		// The first is the one a caller checks.
		rets := []*node.Return{{Type: errorRef()}, {Type: errorRef()}}
		if got := golang.ErrorSlot(rets); got != 0 {
			t.Fatalf("ErrorSlot = %d, want 0", got)
		}
	})

	t.Run("a signature returning no error reports none", func(t *testing.T) {
		t.Parallel()
		rets := []*node.Return{{Type: builtinRef("string")}}
		if got := golang.ErrorSlot(rets); got != -1 {
			t.Fatalf("ErrorSlot = %d, want -1", got)
		}
	})

	t.Run("no returns report none", func(t *testing.T) {
		t.Parallel()
		if got := golang.ErrorSlot(nil); got != -1 {
			t.Fatalf("ErrorSlot(nil) = %d, want -1", got)
		}
	})

	t.Run("a nil slot is skipped rather than panicking", func(t *testing.T) {
		t.Parallel()
		rets := []*node.Return{nil, {Type: errorRef()}}
		if got := golang.ErrorSlot(rets); got != 1 {
			t.Fatalf("ErrorSlot = %d, want 1", got)
		}
	})

	t.Run("an unstamped error is found by its spelling", func(t *testing.T) {
		t.Parallel()
		// The union: a graph no Go frontend produced — a fixture, a
		// bridge, a synthesised node — carries no stamp, and a
		// stamp-only answer would report that such a signature returns
		// no error at all.
		rets := []*node.Return{{Type: builtinRef("error")}}
		if got := golang.ErrorSlot(rets); got != 0 {
			t.Fatalf("ErrorSlot = %d, want 0 for an unstamped builtin error", got)
		}
	})

	t.Run("a qualified error type is not the builtin", func(t *testing.T) {
		t.Parallel()
		// The gate on the union's spelling half: `mypkg.error` is a
		// different type and must not be taken for the predeclared one.
		rets := []*node.Return{{Type: namedTypeRef("example.com/x", "error")}}
		if got := golang.ErrorSlot(rets); got != -1 {
			t.Fatalf("ErrorSlot = %d, want -1 for a qualified error", got)
		}
	})
}

func TestReturnsError(t *testing.T) {
	t.Parallel()

	t.Run("agrees with ErrorSlot", func(t *testing.T) {
		t.Parallel()
		for name, rets := range map[string][]*node.Return{
			"trailing": {{Type: builtinRef("string")}, {Type: errorRef()}},
			"leading":  {{Type: errorRef()}},
			"none":     {{Type: builtinRef("string")}},
			"empty":    nil,
		} {
			if golang.ReturnsError(rets) != (golang.ErrorSlot(rets) >= 0) {
				t.Errorf("ReturnsError disagrees with ErrorSlot for %s", name)
			}
		}
	})
}

func TestNamedReturnsUsable(t *testing.T) {
	t.Parallel()

	t.Run("a fully named signature is usable", func(t *testing.T) {
		t.Parallel()
		rets := []*node.Return{{Name: "user", Type: builtinRef("string")}, {Name: "err", Type: errorRef()}}
		if !golang.NamedReturnsUsable(rets) {
			t.Fatalf("a fully named signature must be usable")
		}
	})

	t.Run("a partly named signature is not", func(t *testing.T) {
		t.Parallel()
		// `(_ User, err error)` is valid Go; the blank normalises to
		// unnamed, so the model holds one named slot and one unnamed —
		// a state Go's all-or-nothing rule forbids in output.
		rets := []*node.Return{{Type: builtinRef("string")}, {Name: "err", Type: errorRef()}}
		if golang.NamedReturnsUsable(rets) {
			t.Fatalf("a partly named signature must fall back to anonymous")
		}
	})

	t.Run("a name colliding with the receiver is not usable", func(t *testing.T) {
		t.Parallel()
		// Renaming around the collision breaks the correspondence the
		// names exist to carry, so the whole signature drops back.
		rets := []*node.Return{{Name: "s", Type: builtinRef("string")}}
		if golang.NamedReturnsUsable(rets, "s") {
			t.Fatalf("a return colliding with the receiver must not be usable")
		}
	})

	t.Run("two returns of one name are not usable", func(t *testing.T) {
		t.Parallel()
		rets := []*node.Return{{Name: "v", Type: builtinRef("string")}, {Name: "v", Type: builtinRef("int")}}
		if golang.NamedReturnsUsable(rets) {
			t.Fatalf("duplicate return names must not be usable")
		}
	})

	t.Run("a nil slot is not usable", func(t *testing.T) {
		t.Parallel()
		if golang.NamedReturnsUsable([]*node.Return{nil}) {
			t.Fatalf("a nil slot must not read as named")
		}
	})

	t.Run("no returns are not usable", func(t *testing.T) {
		t.Parallel()
		// There is nothing to name, and an empty named list would
		// render `()` where the signature wants nothing at all.
		if golang.NamedReturnsUsable(nil) {
			t.Fatalf("an empty return list must not be usable")
		}
	})
}
