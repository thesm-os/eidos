// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

func TestStampDecorators(t *testing.T) {
	t.Parallel()

	t.Run("a class decorator is stamped with its arguments", func(t *testing.T) {
		t.Parallel()
		// A decorator is TypeScript's struct tag: the mechanism a
		// framework uses to attach machine-readable metadata to a
		// declaration. Dropping it leaves the SDK's tag contract with
		// no answer for TypeScript.
		s := onlyStruct(t, `@Entity({ name: 'users' }) class User {}`)

		d, ok := typescript.DecoratorNamed(s, "Entity")
		if !ok {
			t.Fatalf("Entity decorator not stamped; have %v", typescript.DecoratorNames(s))
		}
		if d.Args != "({ name: 'users' })" {
			t.Fatalf("args = %q", d.Args)
		}
	})

	t.Run("a decorator above an export line still attaches", func(t *testing.T) {
		t.Parallel()
		// The grammar puts the decorator under the export statement
		// here, not under the class — and this is the form every
		// framework's documentation uses, so it is the common case.
		for _, src := range []string{
			"@Entity({n:1})\nexport class User {}",
			"export @Entity({n:1}) class User {}",
			"@Entity({n:1})\nclass User {}",
		} {
			s := onlyStruct(t, src)
			if !typescript.HasDecorator(s, "Entity") {
				t.Errorf("%q: decorator lost; have %v", src, typescript.DecoratorNames(s))
			}
		}
	})

	t.Run("a decorator does not leak to a later declaration", func(t *testing.T) {
		t.Parallel()
		decls, _ := convert(t, "@A\nexport class One {}\nexport class Two {}")
		if len(decls) != 2 {
			t.Fatalf("declarations = %d, want 2", len(decls))
		}
		if !typescript.HasDecorator(decls[0].(*node.Struct), "A") {
			t.Error("One lost its decorator")
		}
		if got := typescript.DecoratorNames(decls[1].(*node.Struct)); len(got) != 0 {
			t.Errorf("Two picked up %v from the previous declaration", got)
		}
	})

	t.Run("a bare decorator is present with empty arguments", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `@sealed class C {}`)

		d, ok := typescript.DecoratorNamed(s, "sealed")
		if !ok {
			t.Fatal("bare decorator not stamped")
		}
		if d.Args != "" {
			t.Fatalf("args = %q, want empty", d.Args)
		}
	})

	t.Run("a qualified decorator keeps its whole name", func(t *testing.T) {
		t.Parallel()
		// `a.b.c` is the decorator's identity; the last segment alone
		// would collide with any other `c`.
		s := onlyStruct(t, `@ns.deco({x:1}) class C {}`)
		if !typescript.HasDecorator(s, "ns.deco") {
			t.Fatalf("qualified name lost; have %v", typescript.DecoratorNames(s))
		}
	})

	t.Run("field and method decorators land on the member", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `class C {
			@Column({ type: 'varchar' }) name!: string;
			@Get('/x') handle(): void {}
		}`)

		if !typescript.HasDecorator(s.Fields[0], "Column") {
			t.Errorf("field decorator missing; have %v", typescript.DecoratorNames(s.Fields[0]))
		}
		if !typescript.HasDecorator(s.Methods[0], "Get") {
			t.Errorf("method decorator missing; have %v", typescript.DecoratorNames(s.Methods[0]))
		}
	})

	t.Run("a parameter decorator lands on the parameter", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `class C { constructor(@Inject('TOK') private dep: Dep) {} }`)

		p := s.Methods[0].Params[0]
		d, ok := typescript.DecoratorNamed(p, "Inject")
		if !ok {
			t.Fatalf("parameter decorator missing; have %v", typescript.DecoratorNames(p))
		}
		if d.Args != "('TOK')" {
			t.Fatalf("args = %q", d.Args)
		}
	})

	t.Run("source order is preserved", func(t *testing.T) {
		t.Parallel()
		// Decorator expressions evaluate top-down and apply bottom-up,
		// so `@A @B` and `@B @A` compose differently. A representation
		// reading them alike describes the wrong composition.
		first := typescript.DecoratorNames(onlyStruct(t, `@A() @B() class C {}`))
		second := typescript.DecoratorNames(onlyStruct(t, `@B() @A() class C {}`))

		if !slices.Equal(first, []string{"A", "B"}) {
			t.Fatalf("@A @B -> %v, want [A B]", first)
		}
		if !slices.Equal(second, []string{"B", "A"}) {
			t.Fatalf("@B @A -> %v, want [B A]", second)
		}
	})

	t.Run("a repeated decorator keeps every application", func(t *testing.T) {
		t.Parallel()
		// A route documenting several responses applies the same
		// decorator once per status code; collapsing them would
		// describe an endpoint returning one status.
		s := onlyStruct(t, `class C {
			@ApiResponse({status:200}) @ApiResponse({status:404}) m(): void {}
		}`)

		all := typescript.DecoratorsNamed(s.Methods[0], "ApiResponse")
		if len(all) != 2 {
			t.Fatalf("ApiResponse applications = %d, want 2", len(all))
		}
		if all[0].Args != "({status:200})" || all[1].Args != "({status:404})" {
			t.Fatalf("args = %q, %q; want 200 then 404", all[0].Args, all[1].Args)
		}
	})

	t.Run("decorators from the wrapper and the declaration both land", func(t *testing.T) {
		t.Parallel()
		// `@A` belongs to the export statement and `@B` to the class;
		// both are the class's.
		s := onlyStruct(t, "@A()\nexport @B() class C {}")

		got := typescript.DecoratorNames(s)
		if len(got) != 2 || !slices.Contains(got, "A") || !slices.Contains(got, "B") {
			t.Fatalf("decorators = %v, want both A and B", got)
		}
	})

	t.Run("an undecorated declaration reports none", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `class C { x: string = ''; }`)
		if got := typescript.Decorators(s); len(got) != 0 {
			t.Fatalf("decorators = %v, want none", got)
		}
		if typescript.HasDecorator(s, "Anything") {
			t.Error("reported a decorator that is not there")
		}
	})

	t.Run("definite assignment is distinguished from optional", func(t *testing.T) {
		t.Parallel()
		// `x!: string` is neither optional nor initialised. Without a
		// marker those two absences read as a plain required field.
		s := onlyStruct(t, `class C { a!: string; b?: string; c: string = ''; }`)

		if da, _ := typescript.MetaDefiniteAssignment.Get(s.Fields[0].Meta()); !da {
			t.Error("a!: definite assignment not stamped")
		}
		if typescript.MetaDefiniteAssignment.Has(s.Fields[1].Meta()) {
			t.Error("b?: marked as definite assignment")
		}
		if typescript.MetaDefiniteAssignment.Has(s.Fields[2].Meta()) {
			t.Error("c = '': marked as definite assignment")
		}
	})
}

func TestReExport(t *testing.T) {
	t.Parallel()

	t.Run("a named re-export records the module and the names", func(t *testing.T) {
		t.Parallel()
		// Barrel files are built entirely from these. Treating one as
		// declaring nothing reports that a package's index declares
		// nothing at all.
		imp := onlyImport(t, `export { X, Y as Z } from './y';`)

		if imp.Path != "./y" {
			t.Fatalf("Path = %q", imp.Path)
		}
		if re, _ := typescript.MetaReExport.Get(imp.Meta()); !re {
			t.Error("re-export not marked")
		}
		names, _ := typescript.MetaReExportNames.Get(imp.Meta())
		if !slices.Equal(names, []string{"X", "Y as Z"}) {
			t.Fatalf("names = %v, want [X, Y as Z]", names)
		}
	})

	t.Run("a star re-export records the module and no names", func(t *testing.T) {
		t.Parallel()
		// The forwarded set cannot be enumerated without resolving the
		// target, and recording "no names" as a list would be a claim
		// the frontend cannot make.
		imp := onlyImport(t, `export * from './y';`)

		if imp.Path != "./y" {
			t.Fatalf("Path = %q", imp.Path)
		}
		if names, ok := typescript.MetaReExportNames.Get(imp.Meta()); ok && len(names) > 0 {
			t.Fatalf("names = %v, want none for a star export", names)
		}
	})

	t.Run("a namespaced star re-export keeps its local name", func(t *testing.T) {
		t.Parallel()
		imp := onlyImport(t, `export * as NS from './y';`)
		if imp.Alias != "NS" {
			t.Fatalf("Alias = %q, want NS", imp.Alias)
		}
	})

	t.Run("a type-only re-export is marked", func(t *testing.T) {
		t.Parallel()
		imp := onlyImport(t, `export type { T } from './t';`)
		if to, _ := typescript.MetaTypeOnly.Get(imp.Meta()); !to {
			t.Error("type-only re-export not marked")
		}
	})

	t.Run("a local export list declares no dependency", func(t *testing.T) {
		t.Parallel()
		// `export { a }` forwards a binding already in this module;
		// there is no module to depend on.
		decls, _ := convert(t, `const a = 1; export { a };`)
		for _, d := range decls {
			if _, ok := d.(*node.Import); ok {
				t.Fatal("a local export list produced an import")
			}
		}
	})

	t.Run("a plain import is not marked as a re-export", func(t *testing.T) {
		t.Parallel()
		imp := onlyImport(t, `import { X } from './y';`)
		if typescript.MetaReExport.Has(imp.Meta()) {
			t.Error("a plain import was marked as a re-export")
		}
	})
}
