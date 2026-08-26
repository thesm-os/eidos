// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// classes is a resolver over a fixed set of declarations, keyed by
// the name a heritage clause writes.
type classes map[string]node.Node

func (c classes) Resolve(t *node.TypeRef) (node.Node, bool) {
	if t == nil {
		return nil, false
	}
	n, ok := c[t.Name]
	return n, ok
}

// extending returns a class whose heritage extends the named types.
func extending(name string, bases ...string) *node.Struct {
	s := &node.Struct{Name: name}
	for _, b := range bases {
		e := &node.Embed{Type: named(b)}
		typescript.MetaHeritage.Set(e.EnsureMeta(), typescript.HeritageExtends, "test")
		s.Embeds = append(s.Embeds, e)
	}
	return s
}

func TestSentinelName(t *testing.T) {
	t.Parallel()

	t.Run("a declared error takes the Error suffix", func(t *testing.T) {
		t.Parallel()
		// A suffix rather than Go's `Err` prefix: TypeScript's errors
		// are classes, and the standard library names every one this
		// way.
		if got := typescript.SentinelName("not found"); got != "NotFoundError" {
			t.Fatalf("SentinelName = %q, want NotFoundError", got)
		}
	})

	t.Run("applying the rule twice changes nothing", func(t *testing.T) {
		t.Parallel()
		// A generator naming a declared error and a detector finding
		// one run the same rule from opposite ends.
		once := typescript.SentinelName("NotFound")
		if twice := typescript.SentinelName(once); twice != once {
			t.Fatalf("second pass = %q, want %q", twice, once)
		}
	})

	t.Run("an empty base names nothing", func(t *testing.T) {
		t.Parallel()
		if got := typescript.SentinelName(""); got != "" {
			t.Fatalf("SentinelName(\"\") = %q, want empty", got)
		}
	})

	t.Run("the convention is recognised from the other end", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"NotFoundError", "TypeError"} {
			if !typescript.IsSentinelName(name) {
				t.Errorf("%s is not recognised", name)
			}
		}
		// The bare word is the base class; a generator treating it as a
		// declared error would emit checks against the language's own
		// type.
		for _, name := range []string{"Error", "", "NotFound", "Errors"} {
			if typescript.IsSentinelName(name) {
				t.Errorf("%q is recognised and should not be", name)
			}
		}
	})
}

func TestErrorOf(t *testing.T) {
	t.Parallel()

	t.Run("a class extending Error carries the contract", func(t *testing.T) {
		t.Parallel()
		s := extending("NotFoundError", "Error")
		got, ok := typescript.ErrorOf(s, nil)
		if !ok {
			t.Fatal("a class extending Error carries no contract")
		}
		if got.Addressed || got.Compares {
			t.Errorf("projection claims a contract TypeScript has no form for: %+v", got)
		}
	})

	t.Run("a class extending nothing does not", func(t *testing.T) {
		t.Parallel()
		// `throw` accepts it and every consumer catching by
		// `instanceof Error` misses it, so a check calling it an error
		// asserts something the declaration does not say.
		if _, ok := typescript.ErrorOf(&node.Struct{Name: "ValidationError"}, nil); ok {
			t.Fatal("a bare class was read as an error")
		}
	})

	t.Run("implementing an error-shaped interface is not extending Error", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "Fake"}
		e := &node.Embed{Type: named("Error")}
		typescript.MetaHeritage.Set(e.EnsureMeta(), typescript.HeritageImplements, "test")
		s.Embeds = append(s.Embeds, e)

		if _, ok := typescript.ErrorOf(s, nil); ok {
			t.Fatal("an implements clause was read as inheritance")
		}
	})

	t.Run("the chain is walked through the resolver", func(t *testing.T) {
		t.Parallel()
		// `class NotFoundError extends BaseError` is the dominant idiom
		// for a family of errors; reading only its own heritage finds a
		// name this projection has never heard of.
		base := extending("BaseError", "Error")
		r := classes{"BaseError": base}

		if _, ok := typescript.ErrorOf(extending("NotFoundError", "BaseError"), r); !ok {
			t.Fatal("an inherited contract was not found")
		}
	})

	t.Run("a base the run did not load is named on the refusal", func(t *testing.T) {
		t.Parallel()
		// Which is what separates a declaration that certainly is not
		// an error from one whose base the run never loaded. Both
		// answer false, and only one is worth telling the author about.
		got, ok := typescript.ErrorOf(extending("Wrapped", "BaseError"), classes{})
		if ok {
			t.Fatal("an unresolvable base was read as an error")
		}
		if len(got.Unresolved) != 1 || got.Unresolved[0] != "BaseError" {
			t.Fatalf("Unresolved = %v, want the base named", got.Unresolved)
		}
	})

	t.Run("an unfollowed link deeper in the chain is reported", func(t *testing.T) {
		t.Parallel()
		base := extending("BaseError", "Missing")
		got, ok := typescript.ErrorOf(extending("E", "BaseError"), classes{"BaseError": base})
		if ok {
			t.Fatal("a chain ending nowhere was read as an error")
		}
		if len(got.Unresolved) != 1 || got.Unresolved[0] != "Missing" {
			t.Fatalf("Unresolved = %v, want the unfollowed link", got.Unresolved)
		}
	})

	t.Run("a cause member is what the declaration unwraps through", func(t *testing.T) {
		t.Parallel()
		// Matched on the name: `cause` is typed `unknown` on the
		// built-in, so a type-directed search finds nothing on the one
		// declaration that certainly has it.
		s := extending("WrapError", "Error")
		s.Fields = []*node.Field{{Name: "cause", Type: named("unknown")}}

		got, ok := typescript.ErrorOf(s, nil)
		if !ok {
			t.Fatal("the contract was refused")
		}
		if !got.Unwraps || got.Cause != "cause" {
			t.Fatalf("Unwraps = %v, Cause = %q", got.Unwraps, got.Cause)
		}
	})

	t.Run("a declaration with no cause unwraps nothing", func(t *testing.T) {
		t.Parallel()
		got, _ := typescript.ErrorOf(extending("E", "Error"), nil)
		if got.Unwraps || got.Cause != "" {
			t.Fatalf("Unwraps = %v, Cause = %q", got.Unwraps, got.Cause)
		}
	})

	t.Run("only the assignable members reach a generated literal", func(t *testing.T) {
		t.Parallel()
		// A readonly member is assignable only in its own class's
		// constructor, so a check that set one would fail in the
		// consuming repository rather than here.
		s := extending("E", "Error")
		ro := &node.Field{Name: "code", Type: named("string")}
		typescript.MetaReadonly.Set(ro.EnsureMeta(), true, "test")
		s.Fields = []*node.Field{{Name: "detail", Type: named("string")}, ro}

		got, _ := typescript.ErrorOf(s, nil)
		if len(got.Members) != 1 || got.Members[0].Name != "detail" {
			t.Fatalf("members = %+v", got.Members)
		}
		if !got.Members[0].Verbatim {
			t.Error("a string member is not marked as reaching a message unchanged")
		}
	})

	t.Run("a number is not carried into a message unchanged", func(t *testing.T) {
		t.Parallel()
		// `1e21` is a faithful rendering of a value a check would look
		// for as its long form.
		s := extending("E", "Error")
		s.Fields = []*node.Field{{Name: "status", Type: named("number")}}

		got, _ := typescript.ErrorOf(s, nil)
		if len(got.Members) != 1 || got.Members[0].Verbatim {
			t.Fatalf("members = %+v", got.Members)
		}
	})

	t.Run("nil projects to nothing", func(t *testing.T) {
		t.Parallel()
		if _, ok := typescript.ErrorOf(nil, nil); ok {
			t.Fatal("nil carried a contract")
		}
	})

	t.Run("a heritage cycle terminates", func(t *testing.T) {
		t.Parallel()
		// Not expressible in TypeScript, and expressible in a graph
		// assembled from several runs.
		a := extending("A", "B")
		b := extending("B", "A")
		if _, ok := typescript.ErrorOf(a, classes{"A": a, "B": b}); ok {
			t.Fatal("a cycle was read as an error")
		}
	})
}
