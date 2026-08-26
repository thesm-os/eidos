// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
)

// line returns the rendered declaration line containing want, so an
// assertion names the one construct it is about rather than the whole
// file.
//
// Comment lines are skipped. The generated marker contains "eidos",
// so a search for a short identifier like "id" matches the header
// before it reaches the property — which is a false pass waiting to
// happen rather than a failure, since the header is always there.
func line(t *testing.T, body, want string) string {
	t.Helper()
	for l := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(l)
		if isCommentOrBlank(trimmed) {
			continue
		}
		if strings.Contains(trimmed, want) {
			return trimmed
		}
	}
	t.Fatalf("no declaration line containing %q in:\n%s", want, body)
	return ""
}

// isCommentOrBlank reports a line carrying no declaration.
func isCommentOrBlank(trimmed string) bool {
	return trimmed == "" ||
		strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "*")
}

func TestRenderMethodSignatures(t *testing.T) {
	t.Parallel()

	t.Run("params and return render into the signature", func(t *testing.T) {
		t.Parallel()
		got := render(t, pkgWith(&emit.Interface{
			Name: "Repo", Target: target,
			Methods: []*emit.Method{{
				Name: "find",
				Params: []*emit.Param{
					{Name: "id", Type: emit.Builtin("string")},
					{Name: "limit", Type: emit.Builtin("int")},
				},
				Returns: []*emit.Return{{Type: emit.Builtin("bool")}},
			}},
		}))

		if want := "find(id: string, limit: number): boolean;"; line(t, got, "find") != want {
			t.Fatalf("signature = %q, want %q", line(t, got, "find"), want)
		}
	})

	t.Run("no return renders void", func(t *testing.T) {
		t.Parallel()
		got := render(t, pkgWith(&emit.Interface{
			Name: "R", Target: target,
			Methods: []*emit.Method{{Name: "run"}},
		}))
		if want := "run(): void;"; line(t, got, "run") != want {
			t.Fatalf("signature = %q, want %q", line(t, got, "run"), want)
		}
	})

	t.Run("several returns render as a tuple", func(t *testing.T) {
		t.Parallel()
		// TypeScript has one return value, so a signature carrying
		// more than one is spelled as the tuple that holds them.
		got := render(t, pkgWith(&emit.Interface{
			Name: "R", Target: target,
			Methods: []*emit.Method{{
				Name: "pair",
				Returns: []*emit.Return{
					{Type: emit.Builtin("string")},
					{Type: emit.Builtin("error")},
				},
			}},
		}))
		if want := "pair(): [string, Error];"; line(t, got, "pair") != want {
			t.Fatalf("signature = %q, want %q", line(t, got, "pair"), want)
		}
	})

	t.Run("an unnamed parameter is named positionally", func(t *testing.T) {
		t.Parallel()
		// TypeScript requires a name in a signature: `(string) => void`
		// declares a parameter *named* string with an inferred type,
		// which is a different signature from the one meant.
		got := render(t, pkgWith(&emit.Interface{
			Name: "R", Target: target,
			Methods: []*emit.Method{{
				Name:   "take",
				Params: []*emit.Param{{Type: emit.Builtin("string")}},
			}},
		}))
		if want := "take(arg0: string): void;"; line(t, got, "take") != want {
			t.Fatalf("signature = %q, want %q", line(t, got, "take"), want)
		}
	})

	t.Run("a rest parameter regains its array type", func(t *testing.T) {
		t.Parallel()
		// node.Param records the element type; TypeScript annotates
		// the array, so the array is restored at the render site.
		got := render(t, pkgWith(&emit.Interface{
			Name: "R", Target: target,
			Methods: []*emit.Method{{
				Name:   "all",
				Params: []*emit.Param{{Name: "rest", Type: emit.Builtin("string"), Variadic: true}},
			}},
		}))
		if want := "all(...rest: string[]): void;"; line(t, got, "all") != want {
			t.Fatalf("signature = %q, want %q", line(t, got, "all"), want)
		}
	})

	t.Run("an async method carries the keyword", func(t *testing.T) {
		t.Parallel()
		m := &emit.Method{Name: "load", Returns: []*emit.Return{{Type: emit.Builtin("string")}}}
		typescript.MetaAsync.Set(m.EnsureMeta(), true, "test")

		got := render(t, pkgWith(&emit.Interface{
			Name: "R", Target: target, Methods: []*emit.Method{m},
		}))
		if !strings.Contains(line(t, got, "load"), "async load(") {
			t.Fatalf("signature = %q, want the async keyword", line(t, got, "load"))
		}
	})
}

func TestRenderMemberModifiers(t *testing.T) {
	t.Parallel()

	t.Run("readonly and optional render on the property", func(t *testing.T) {
		t.Parallel()
		ro := &emit.Field{Name: "id", Type: emit.Builtin("string")}
		typescript.MetaReadonly.Set(ro.EnsureMeta(), true, "test")
		opt := &emit.Field{Name: "name", Type: emit.Builtin("string")}
		typescript.MetaOptional.Set(opt.EnsureMeta(), true, "test")

		got := render(t, pkgWith(&emit.Interface{
			Name: "U", Target: target, Fields: []*emit.Field{ro, opt},
		}))

		if want := "readonly id: string;"; line(t, got, "id") != want {
			t.Errorf("readonly = %q, want %q", line(t, got, "id"), want)
		}
		// `?` and `| undefined` are different claims: an optional
		// property may be absent entirely.
		if want := "name?: string;"; line(t, got, "name") != want {
			t.Errorf("optional = %q, want %q", line(t, got, "name"), want)
		}
	})

	t.Run("a key needing quotes is quoted", func(t *testing.T) {
		t.Parallel()
		got := render(t, pkgWith(&emit.Interface{
			Name: "H", Target: target,
			Fields: []*emit.Field{{Name: "content-type", Type: emit.Builtin("string")}},
		}))
		if want := "'content-type': string;"; line(t, got, "content-type") != want {
			t.Fatalf("property = %q, want %q", line(t, got, "content-type"), want)
		}
	})
}

func TestRenderGenerics(t *testing.T) {
	t.Parallel()

	t.Run("type parameters and their bounds render", func(t *testing.T) {
		t.Parallel()
		got := render(t, pkgWith(&emit.Interface{
			Name: "Box", Target: target,
			TypeParams: []*emit.TypeParam{{
				Name:       "T",
				Constraint: &emit.Constraint{Embedded: []emit.Ref{emit.Builtin("object")}},
			}},
			Fields: []*emit.Field{{Name: "v", Type: emit.Builtin("string")}},
		}))
		if !strings.Contains(got, "interface Box<T extends object>") {
			t.Fatalf("generics missing:\n%s", got)
		}
	})

	t.Run("several bounds render as an intersection", func(t *testing.T) {
		t.Parallel()
		// TypeScript's `extends` takes one type, and a parameter
		// required to satisfy two is bounded by the type that is both.
		got := render(t, pkgWith(&emit.Interface{
			Name: "B", Target: target,
			TypeParams: []*emit.TypeParam{{
				Name: "T",
				Constraint: &emit.Constraint{Embedded: []emit.Ref{
					emit.Builtin("A"), emit.Builtin("B"),
				}},
			}},
			Fields: []*emit.Field{{Name: "v", Type: emit.Builtin("string")}},
		}))
		if !strings.Contains(got, "<T extends A & B>") {
			t.Fatalf("intersection bound missing:\n%s", got)
		}
	})

	t.Run("an unconstrained parameter carries no bound", func(t *testing.T) {
		t.Parallel()
		got := render(t, pkgWith(&emit.Interface{
			Name: "B", Target: target,
			TypeParams: []*emit.TypeParam{{Name: "T"}},
			Fields:     []*emit.Field{{Name: "v", Type: emit.Builtin("string")}},
		}))
		if !strings.Contains(got, "interface B<T>") {
			t.Fatalf("bare type parameter missing:\n%s", got)
		}
	})
}

func TestRenderHeritage(t *testing.T) {
	t.Parallel()

	t.Run("extends and implements render in their own clauses", func(t *testing.T) {
		t.Parallel()
		base := &emit.Embed{Type: emit.Builtin("Base")}
		typescript.MetaHeritage.Set(base.EnsureMeta(), typescript.HeritageExtends, "test")
		iface := &emit.Embed{Type: emit.Builtin("Ident")}
		typescript.MetaHeritage.Set(iface.EnsureMeta(), typescript.HeritageImplements, "test")

		got := render(t, pkgWith(&emit.Struct{
			Name: "User", Target: target,
			Embeds: []*emit.Embed{base, iface},
			Fields: []*emit.Field{{Name: "id", Type: emit.Builtin("string")}},
		}))
		if !strings.Contains(got, "class User extends Base implements Ident {") {
			t.Fatalf("heritage wrong:\n%s", got)
		}
	})

	t.Run("an unmarked embed is an extends", func(t *testing.T) {
		t.Parallel()
		// Which is the reading that holds for an interface, and an
		// interface is where an unmarked embed comes from.
		got := render(t, pkgWith(&emit.Interface{
			Name: "A", Target: target,
			Embeds: []*emit.Embed{{Type: emit.Builtin("B")}},
			Fields: []*emit.Field{{Name: "v", Type: emit.Builtin("string")}},
		}))
		if !strings.Contains(got, "interface A extends B {") {
			t.Fatalf("heritage wrong:\n%s", got)
		}
	})
}

func TestRenderDeclarationKinds(t *testing.T) {
	t.Parallel()

	t.Run("a class renders from a struct", func(t *testing.T) {
		t.Parallel()
		got := render(t, pkgWith(&emit.Struct{
			Name: "User", Target: target,
			Fields: []*emit.Field{{Name: "id", Type: emit.Builtin("string")}},
		}))
		if !strings.Contains(got, "export class User {") {
			t.Fatalf("class missing:\n%s", got)
		}
	})

	t.Run("a constant renders as an ambient declaration", func(t *testing.T) {
		t.Parallel()
		got := render(t, pkgWith(&emit.Constant{
			Name: "MAX", Target: target, Type: emit.Builtin("int"),
		}))
		if !strings.Contains(got, "export declare const MAX: number;") {
			t.Fatalf("constant missing:\n%s", got)
		}
	})

	t.Run("declarations sort by name within a file", func(t *testing.T) {
		t.Parallel()
		// Emit order is whatever order the generators ran in, so a
		// file's contents would otherwise shuffle when an unrelated
		// plugin was registered.
		field := func() []*emit.Field {
			return []*emit.Field{{Name: "v", Type: emit.Builtin("string")}}
		}
		got := render(t, pkgWith(
			&emit.Interface{Name: "Zeta", Target: target, Fields: field()},
			&emit.Interface{Name: "Alpha", Target: target, Fields: field()},
		))
		if strings.Index(got, "interface Alpha") > strings.Index(got, "interface Zeta") {
			t.Fatalf("declarations not sorted:\n%s", got)
		}
	})
}
