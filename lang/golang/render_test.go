// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

func TestArgs(t *testing.T) {
	t.Parallel()

	t.Run("renders the parameters as a call's arguments", func(t *testing.T) {
		t.Parallel()
		if got := golang.Args(golang.SigOf(getMethod())); got != "ctx, id" {
			t.Fatalf("Args = %q, want \"ctx, id\"", got)
		}
	})

	t.Run("spreads a variadic tail", func(t *testing.T) {
		t.Parallel()
		// Forwarding without the ellipsis passes the slice as a single
		// element, which type-checks against `...any`.
		if got := golang.Args(printSig()); got != "prefix, args..." {
			t.Fatalf("Args = %q, want the tail spread", got)
		}
	})

	t.Run("the declaration form carries no spread", func(t *testing.T) {
		t.Parallel()
		// An ellipsis in a declaration list is a syntax error.
		if got := golang.ParamNames(printSig()); got != "prefix, args" {
			t.Fatalf("ParamNames = %q, want no spread", got)
		}
	})

	t.Run("no parameters render nothing", func(t *testing.T) {
		t.Parallel()
		if got := golang.Args(golang.SigOf(&node.Method{Name: "Close"})); got != "" {
			t.Fatalf("Args = %q, want empty", got)
		}
	})
}

func TestIdentLists(t *testing.T) {
	t.Parallel()

	t.Run("renders positional identifiers", func(t *testing.T) {
		t.Parallel()
		if got := golang.Idents("a", 3); got != "a0, a1, a2" {
			t.Fatalf("Idents = %q, want a0, a1, a2", got)
		}
	})

	t.Run("renders discards", func(t *testing.T) {
		t.Parallel()
		if got := golang.Blanks(2); got != "_, _" {
			t.Fatalf("Blanks = %q, want \"_, _\"", got)
		}
	})

	t.Run("a zero count renders nothing", func(t *testing.T) {
		t.Parallel()
		if golang.Idents("a", 0) != "" || golang.Blanks(0) != "" {
			t.Fatalf("a zero count must render nothing")
		}
	})

	t.Run("positional arguments spread a variadic tail", func(t *testing.T) {
		t.Parallel()
		// A call site needs the spread; a declaration list must not
		// have it, which is why the two are separate entries.
		if got := golang.IdentArgs("a", printSig()); got != "a0, a1..." {
			t.Fatalf("IdentArgs = %q, want a0, a1...", got)
		}
	})
}

func TestFieldLists(t *testing.T) {
	t.Parallel()

	sig := golang.SigOf(getMethod())

	t.Run("renders recorded-call assignments", func(t *testing.T) {
		t.Parallel()
		if got := golang.CallFields(sig); got != "Ctx: ctx, ID: id" {
			t.Fatalf("CallFields = %q", got)
		}
	})

	t.Run("renders the capture locals", func(t *testing.T) {
		t.Parallel()
		if got := golang.Locals(sig); got != "item, err" {
			t.Fatalf("Locals = %q, want item, err", got)
		}
	})

	t.Run("renders the tuple built from those captures", func(t *testing.T) {
		t.Parallel()
		if got := golang.LocalFields(sig); got != "Item: item, Err: err" {
			t.Fatalf("LocalFields = %q", got)
		}
	})

	t.Run("renders the tuple built from positional identifiers", func(t *testing.T) {
		t.Parallel()
		if got := golang.IdentFields("got", sig); got != "Item: got0, Err: got1" {
			t.Fatalf("IdentFields = %q", got)
		}
	})

	t.Run("renders the consumer-facing tuple", func(t *testing.T) {
		t.Parallel()
		// Named after the recorded-call fields rather than the
		// internal locals: this is the surface a caller reads.
		if got := golang.NamedFields(sig); got != "Item: item, Err: err" {
			t.Fatalf("NamedFields = %q", got)
		}
	})

	t.Run("renders a read-back off a held value", func(t *testing.T) {
		t.Parallel()
		if got := golang.Reads("r", sig); got != "r.Item, r.Err" {
			t.Fatalf("Reads = %q, want r.Item, r.Err", got)
		}
	})
}

func TestFails(t *testing.T) {
	t.Parallel()

	t.Run("binds only the error slot", func(t *testing.T) {
		t.Parallel()
		if got := golang.Fails(golang.SigOf(getMethod())); got != "_, err" {
			t.Fatalf("Fails = %q, want \"_, err\"", got)
		}
	})

	t.Run("finds a leading error by flag, not position", func(t *testing.T) {
		t.Parallel()
		// `(error, string)` is unusual but legal Go, and a positional
		// rule would bind the wrong local without failing to compile.
		m := &node.Method{Name: "F", Returns: []*node.Return{
			{Name: "err", Type: errorRef()}, {Name: "text", Type: builtinRef("string")},
		}}
		if got := golang.Fails(golang.SigOf(m)); got != "err, _" {
			t.Fatalf("Fails = %q, want \"err, _\"", got)
		}
	})

	t.Run("a signature with no error discards everything", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "F", Returns: []*node.Return{{Type: builtinRef("string")}}}
		if got := golang.Fails(golang.SigOf(m)); got != "_" {
			t.Fatalf("Fails = %q, want \"_\"", got)
		}
	})
}

func TestZeroArgs(t *testing.T) {
	t.Parallel()

	t.Run("fills every position with its zero", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "F", Params: []*node.Param{
			{Name: "a", Type: builtinRef("int")},
			{Name: "b", Type: builtinRef("string")},
		}}
		got, ok := golang.ZeroArgs(golang.SigOf(m))
		if !ok || got != `0, ""` {
			t.Fatalf("ZeroArgs = %q, %v", got, ok)
		}
	})

	t.Run("one underivable position kills the whole list", func(t *testing.T) {
		t.Parallel()
		// A partial argument list does not compile, so a caller that
		// cannot fill every position must emit no call at all.
		m := &node.Method{Name: "F", Params: []*node.Param{
			{Name: "a", Type: builtinRef("int")},
			{Name: "b", Type: namedTypeRef("time", "Duration")},
		}}
		if got, ok := golang.ZeroArgs(golang.SigOf(m)); ok {
			t.Fatalf("ZeroArgs = %q, true; want not derivable", got)
		}
	})
}

func TestRenderNilProjection(t *testing.T) {
	t.Parallel()

	t.Run("every renderer tolerates a nil projection", func(t *testing.T) {
		t.Parallel()
		// These are called from templates, where a nil is a data gap
		// and a panic surfaces as a framework fault against the
		// caller's plugin.
		var s *golang.Sig
		for name, got := range map[string]string{
			"Args":        golang.Args(s),
			"ParamNames":  golang.ParamNames(s),
			"IdentArgs":   golang.IdentArgs("a", s),
			"CallFields":  golang.CallFields(s),
			"Locals":      golang.Locals(s),
			"LocalFields": golang.LocalFields(s),
			"IdentFields": golang.IdentFields("g", s),
			"NamedFields": golang.NamedFields(s),
			"Reads":       golang.Reads("r", s),
			"Fails":       golang.Fails(s),
		} {
			if got != "" {
				t.Errorf("%s(nil) = %q, want empty", name, got)
			}
		}
		if _, ok := golang.ZeroArgs(s); ok {
			t.Errorf("ZeroArgs(nil) reported derivable")
		}
	})
}
