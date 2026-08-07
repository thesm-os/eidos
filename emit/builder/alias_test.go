// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestAliasBuilder_MethodOnTrueAliasRecordsError covers the
// language-rule guard: methods are not allowed on `type X = Y`
// aliases. The violation accumulates onto the parent
// [builder.PackageBuilder] and surfaces via
// [builder.PackageBuilder.Build], leaving the resulting graph
// clean of the offending method.
func TestAliasBuilder_MethodOnTrueAliasRecordsError(t *testing.T) {
	t.Parallel()

	t.Run("Method on a true alias records ErrAliasMethodForbidden", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen").WithTarget(defaultTarget)
		var a *emit.Alias
		pkg, err := c.Package("users", "example.com/users").
			Alias("UserID", emit.Builtin("string"), func(ab *builder.AliasBuilder) {
				a = ab.Node()
				ab.Method("Validate", nil)
			}).
			Build()
		if !errors.Is(err, builder.ErrAliasMethodForbidden) {
			t.Fatalf("expected ErrAliasMethodForbidden; got %v", err)
		}
		if len(a.Methods) != 0 {
			t.Fatalf("method should be dropped on a true alias; got %d", len(a.Methods))
		}
		if len(pkg.Aliases) != 1 {
			t.Fatalf("alias itself should still land in the package; got %d", len(pkg.Aliases))
		}
	})

	t.Run("Method on a named-type alias succeeds", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen").WithTarget(defaultTarget)
		var a *emit.Alias
		_, err := c.Package("users", "example.com/users").
			NamedType("UserID", emit.Builtin("string"), func(ab *builder.AliasBuilder) {
				a = ab.Node()
				ab.Method("Validate", nil)
			}).
			Build()
		if err != nil {
			t.Fatalf("Build returned %v", err)
		}
		if len(a.Methods) != 1 || a.Methods[0].Owner != a {
			t.Fatalf("expected one method owned by the alias; got %+v", a.Methods)
		}
	})
}

// TestAliasBuilder_MethodWithCallbackRuns covers the non-nil
// method callback path on a named-type alias — the callback runs
// and the resulting method's Owner is wired to the alias.
func TestAliasBuilder_MethodWithCallbackRuns(t *testing.T) {
	t.Parallel()

	t.Run("non-nil method callback runs with method-builder", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen").WithTarget(defaultTarget)
		var captured *emit.Method
		c.Package("p", "p").
			NamedType("UserID", emit.Builtin("string"), func(ab *builder.AliasBuilder) {
				ab.Method("Validate", func(mb *builder.MethodBuilder) {
					captured = mb.Node()
					mb.Return(emit.Builtin("error"))
				})
			})
		if captured == nil || len(captured.Returns) != 1 {
			t.Fatalf("method callback did not run; got %+v", captured)
		}
	})
}

// TestAliasBuilder_Accessors covers the Pos / Docs / Directive /
// File / TypeParam accessors on the alias builder. File takes the
// `Target` semantics for aliases (the field is named File on
// [emit.Alias] to disambiguate from the Target Ref).
func TestAliasBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs / Directive / File / TypeParam / Origin thread through", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		other := otherTarget()
		d := fixtureDirective()
		pos := fixturePos()
		origin := fixtureOrigin()
		var n *emit.Alias
		c.Package("p", "p").
			Alias("A", emit.Builtin("string"), func(b *builder.AliasBuilder) {
				n = b.Node()
				b.Pos(pos).Docs("docs").Directive(d).File(other).TypeParam("T", nil).Origin(origin)
			})
		assertCommon(t, n.SourcePos, n.DocLines, n.DirectiveList, pos, d)
		if n.File != other {
			t.Fatalf("alias File override failed; got %v", n.File)
		}
		if len(n.TypeParams) != 1 {
			t.Fatalf("type param not appended")
		}
		if n.Origin() != origin {
			t.Fatalf("Origin not threaded; got %v, want %v", n.Origin(), origin)
		}
	})
}
