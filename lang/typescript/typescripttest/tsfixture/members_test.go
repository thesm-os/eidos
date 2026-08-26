// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/lang/typescript/typescripttest/tsfixture"
	"go.thesmos.sh/eidos/node"
)

// The sub-builders that configure a declaration's members, tested
// together: each is a set of setters over a node the parent already
// wired, and the questions worth asking span them — does the owner
// get set, does the member inherit the file, does a modifier reach the
// key the frontend stamps.

func TestInterfaceMembers(t *testing.T) {
	t.Parallel()

	t.Run("a property and a method both land on the interface", func(t *testing.T) {
		t.Parallel()
		// A TypeScript interface declares both, which is why
		// node.Interface carries a field list at all.
		i := iface(t, func(b *tsfixture.InterfaceBuilder) {
			b.Field("id", tsfixture.Named("string"), nil)
			b.Method("greet", func(m *tsfixture.MethodBuilder) {
				m.Return(tsfixture.Named("string"))
			})
		})
		if len(i.Fields) != 1 || len(i.Methods) != 1 {
			t.Fatalf("%d properties, %d methods", len(i.Fields), len(i.Methods))
		}
	})

	t.Run("members carry their owner and inherit the file", func(t *testing.T) {
		t.Parallel()
		// A real frontend records every member at the file the
		// declaration was parsed from, and Layout routes from whichever
		// of them a plugin picks as its origin.
		i := iface(t, func(b *tsfixture.InterfaceBuilder) {
			b.Field("id", tsfixture.Named("string"), nil)
			b.Method("greet", nil)
			b.Extends(tsfixture.Named("Base"))
		})
		if i.Fields[0].Owner != i || i.Methods[0].Owner != i || i.Embeds[0].Owner != i {
			t.Fatal("a member was built without its owner")
		}
		for what, got := range map[string]string{
			"property": i.Fields[0].Pos().File,
			"method":   i.Methods[0].Pos().File,
			"heritage": i.Embeds[0].Pos().File,
		} {
			if got != "test/user.ts" {
				t.Errorf("%s file = %q", what, got)
			}
		}
	})

	t.Run("a member's file does not depend on declaration order", func(t *testing.T) {
		t.Parallel()
		// Read from the value the parent computed rather than from the
		// node's own position, so a Pos call inside the callback cannot
		// move some members and not others.
		i := iface(t, func(b *tsfixture.InterfaceBuilder) {
			b.Field("before", tsfixture.Named("string"), nil)
			b.Pos(position.Pos{File: "elsewhere.ts"})
			b.Field("after", tsfixture.Named("string"), nil)
		})
		if i.Fields[0].Pos().File != i.Fields[1].Pos().File {
			t.Fatalf("members split across %q and %q",
				i.Fields[0].Pos().File, i.Fields[1].Pos().File)
		}
	})

	t.Run("heritage is stamped extends", func(t *testing.T) {
		t.Parallel()
		// Also what an unmarked embed reads as, stamped anyway so a
		// consumer never has to know that.
		i := iface(t, func(b *tsfixture.InterfaceBuilder) {
			b.Extends(tsfixture.Named("Base"))
		})
		kind, _ := typescript.MetaHeritage.Get(i.Embeds[0].Meta())
		if kind != typescript.HeritageExtends {
			t.Fatalf("heritage = %q", kind)
		}
	})

	t.Run("the verbatim signatures are carried", func(t *testing.T) {
		t.Parallel()
		// The model has no variant for either: an index signature
		// declares the shape of any key rather than a named member.
		i := iface(t, func(b *tsfixture.InterfaceBuilder) {
			b.IndexSignature("[key: string]: unknown").
				ConstructSignature("new (id: string): User")
		})
		if got, _ := typescript.MetaIndexSignature.Get(i.Meta()); got == "" {
			t.Error("the index signature was dropped")
		}
		if got, _ := typescript.MetaConstructSignature.Get(i.Meta()); got == "" {
			t.Error("the construct signature was dropped")
		}
	})

	t.Run("docs, directives and generics reach the node", func(t *testing.T) {
		t.Parallel()
		i := iface(t, func(b *tsfixture.InterfaceBuilder) {
			b.Docs("A user.").
				Directive(tsfixture.Directive("repo")).
				TypeParam("T", tsfixture.Constraint(tsfixture.Named("object")))
		})
		if len(i.DocLines) != 1 || len(i.DirectiveList) != 1 || len(i.TypeParams) != 1 {
			t.Fatalf("interface = %+v", i)
		}
		if i.TypeParams[0].Owner != i {
			t.Error("a type parameter was built without its owner")
		}
	})
}

func TestClassMembers(t *testing.T) {
	t.Parallel()

	t.Run("extends and implements are different clauses", func(t *testing.T) {
		t.Parallel()
		// Extending inherits members, implementing only asserts they
		// are present — a consumer treating them alike would report
		// members the class never declares.
		c := class(t, func(b *tsfixture.ClassBuilder) {
			b.Extends(tsfixture.Named("Base")).Implements(tsfixture.Named("Store"))
		})
		first, _ := typescript.MetaHeritage.Get(c.Embeds[0].Meta())
		second, _ := typescript.MetaHeritage.Get(c.Embeds[1].Meta())
		if first != typescript.HeritageExtends || second != typescript.HeritageImplements {
			t.Fatalf("clauses = %q, %q", first, second)
		}
	})

	t.Run("a method carries no receiver", func(t *testing.T) {
		t.Parallel()
		// TypeScript binds `this` implicitly and the frontend leaves
		// Receiver nil, so a fixture setting one would build a graph no
		// TypeScript run produces.
		c := class(t, func(b *tsfixture.ClassBuilder) { b.Method("find", nil) })
		if c.Methods[0].Receiver != nil {
			t.Fatalf("receiver = %+v, want none", c.Methods[0].Receiver)
		}
	})

	t.Run("abstract and decorators reach the node", func(t *testing.T) {
		t.Parallel()
		c := class(t, func(b *tsfixture.ClassBuilder) {
			b.Abstract().Decorator("Injectable").Decorator("Entity", "{ name: 'users' }")
		})
		if a, _ := typescript.MetaAbstract.Get(c.Meta()); !a {
			t.Error("the class is not abstract")
		}
		decorators := typescript.Decorators(c)
		if len(decorators) != 2 {
			t.Fatalf("decorators = %+v", decorators)
		}
		if decorators[0].Args != "" {
			t.Errorf("a bare decorator gained arguments: %q", decorators[0].Args)
		}
		if decorators[1].Args != "({ name: 'users' })" {
			t.Errorf("arguments = %q", decorators[1].Args)
		}
	})

	t.Run("decorator order is preserved", func(t *testing.T) {
		t.Parallel()
		// Decorators apply bottom-up, so `@A @B` and `@B @A` compose
		// differently and a set would read them alike.
		c := class(t, func(b *tsfixture.ClassBuilder) {
			b.Decorator("A").Decorator("B").Decorator("A")
		})
		got := typescript.DecoratorNames(c)
		if strings.Join(got, " ") != "A B A" {
			t.Fatalf("names = %v, want A B A", got)
		}
	})
}

func TestFieldModifiers(t *testing.T) {
	t.Parallel()

	t.Run("each modifier reaches the key the frontend stamps", func(t *testing.T) {
		t.Parallel()
		c := class(t, func(b *tsfixture.ClassBuilder) {
			b.Field("id", tsfixture.Named("string"), func(f *tsfixture.FieldBuilder) {
				f.Optional().Readonly().Static().DefiniteAssignment().
					Visibility(typescript.VisibilityPrivate).
					Initialiser("'anon'").
					Decorator("Column").
					Docs("The identifier.").
					Directive(tsfixture.Directive("skip")).
					Pos(position.Pos{File: "pinned.ts"})
			})
		})
		f := c.Fields[0]
		for name, key := range map[string]meta.Key[bool]{
			"optional":  typescript.MetaOptional,
			"readonly":  typescript.MetaReadonly,
			"static":    typescript.MetaStatic,
			"definite!": typescript.MetaDefiniteAssignment,
		} {
			if on, _ := key.Get(f.Meta()); !on {
				t.Errorf("%s is not marked", name)
			}
		}
		if v, _ := typescript.MetaVisibility.Get(f.Meta()); v != typescript.VisibilityPrivate {
			t.Errorf("visibility = %q", v)
		}
		if init, _ := typescript.MetaInitialiser.Get(f.Meta()); init != "'anon'" {
			t.Errorf("initialiser = %q", init)
		}
		if !typescript.HasDecorator(f, "Column") {
			t.Error("the decorator was dropped")
		}
		if len(f.DocLines) != 1 || len(f.DirectiveList) != 1 {
			t.Errorf("docs = %v, directives = %d", f.DocLines, len(f.DirectiveList))
		}
		if f.Pos().File != "pinned.ts" {
			t.Errorf("pinned position = %q", f.Pos().File)
		}
	})
}

func TestMethodBuilder(t *testing.T) {
	t.Parallel()

	t.Run("a rest parameter carries its element type", func(t *testing.T) {
		t.Parallel()
		// A fixture storing the array would describe `...items: T[][]`.
		m := method(t, func(b *tsfixture.MethodBuilder) {
			b.Rest("items", tsfixture.Named("string"))
		})
		p := m.Params[0]
		if !p.Variadic {
			t.Fatal("the rest marker was dropped")
		}
		if p.Type.TypeKind != node.TypeRefNamed || p.Type.Name != "string" {
			t.Fatalf("type = %+v, want the element", p.Type)
		}
	})

	t.Run("an optional parameter is marked and a rest one is not", func(t *testing.T) {
		t.Parallel()
		m := method(t, func(b *tsfixture.MethodBuilder) {
			b.Param("a", tsfixture.Named("string")).
				OptionalParam("b", tsfixture.Named("string")).
				Rest("c", tsfixture.Named("string"))
		})
		if opt, _ := typescript.MetaOptional.Get(m.Params[0].Meta()); opt {
			t.Error("a required parameter is marked optional")
		}
		if opt, _ := typescript.MetaOptional.Get(m.Params[1].Meta()); !opt {
			t.Error("the optional parameter is not marked")
		}
		if opt, _ := typescript.MetaOptional.Get(m.Params[2].Meta()); opt {
			t.Error("a rest parameter is marked optional")
		}
	})

	t.Run("parameters carry their owner", func(t *testing.T) {
		t.Parallel()
		m := method(t, func(b *tsfixture.MethodBuilder) {
			b.Param("a", tsfixture.Named("string")).TypeParam("T", nil)
		})
		if m.Params[0].Owner != m || m.TypeParams[0].Owner != m {
			t.Fatal("a member was built without its owner")
		}
	})

	t.Run("each modifier reaches its key", func(t *testing.T) {
		t.Parallel()
		m := method(t, func(b *tsfixture.MethodBuilder) {
			b.Optional().Async().Generator().Static().Abstract().
				Accessor(typescript.AccessorGet).
				Visibility(typescript.VisibilityProtected).
				Decorator("Log").
				Docs("Finds one.").
				Directive(tsfixture.Directive("skip")).
				Pos(position.Pos{File: "pinned.ts"})
		})
		for name, key := range map[string]meta.Key[bool]{
			"optional":  typescript.MetaOptional,
			"async":     typescript.MetaAsync,
			"generator": typescript.MetaGenerator,
			"static":    typescript.MetaStatic,
			"abstract":  typescript.MetaAbstract,
		} {
			if on, _ := key.Get(m.Meta()); !on {
				t.Errorf("%s is not marked", name)
			}
		}
		if a, _ := typescript.MetaAccessor.Get(m.Meta()); a != typescript.AccessorGet {
			t.Errorf("accessor = %q", a)
		}
		if v, _ := typescript.MetaVisibility.Get(m.Meta()); v != typescript.VisibilityProtected {
			t.Errorf("visibility = %q", v)
		}
		if !typescript.HasDecorator(m, "Log") {
			t.Error("the decorator was dropped")
		}
		if len(m.DocLines) != 1 || len(m.DirectiveList) != 1 || m.Pos().File != "pinned.ts" {
			t.Errorf("method = %+v", m)
		}
	})

	t.Run("overloads accumulate in source order", func(t *testing.T) {
		t.Parallel()
		// TypeScript matches a call top-down and takes the first that
		// fits, so a fixture reordering them describes a different
		// callable.
		m := method(t, func(b *tsfixture.MethodBuilder) {
			b.Overload("find(id: string): User").Overload("find(id: number): User")
		})
		got, _ := typescript.MetaOverloads.Get(m.Meta())
		if len(got) != 2 || got[0].Text != "find(id: string): User" {
			t.Fatalf("overloads = %+v", got)
		}
	})
}

func TestEnumBuilder(t *testing.T) {
	t.Parallel()

	t.Run("a member with no value took the counter", func(t *testing.T) {
		t.Parallel()
		// Recording a value the source never wrote would make the
		// fixture disagree with what the frontend produces from the
		// same declaration.
		e := enum(t, func(b *tsfixture.EnumBuilder) {
			b.Numbers().Variant("Low", "").Variant("High", "5")
		})
		if e.Variants[0].Value != "" {
			t.Errorf("implicit member value = %q, want none", e.Variants[0].Value)
		}
		if e.Variants[1].Value != "5" {
			t.Errorf("explicit member value = %q", e.Variants[1].Value)
		}
		if e.Variants[0].Owner != e {
			t.Error("a member was built without its owner")
		}
	})

	t.Run("Strings and Numbers set what the frontend derives", func(t *testing.T) {
		t.Parallel()
		if got := enum(t, func(b *tsfixture.EnumBuilder) { b.Strings() }).Underlying; got.Name != "string" {
			t.Errorf("Strings underlying = %q", got.Name)
		}
		if got := enum(t, func(b *tsfixture.EnumBuilder) { b.Numbers() }).Underlying; got.Name != "number" {
			t.Errorf("Numbers underlying = %q", got.Name)
		}
	})

	t.Run("a const enum is marked", func(t *testing.T) {
		t.Parallel()
		e := enum(t, func(b *tsfixture.EnumBuilder) { b.Const() })
		if c, _ := typescript.MetaConstEnum.Get(e.Meta()); !c {
			t.Fatal("the const marker was dropped")
		}
	})

	t.Run("a member carries docs and a directive", func(t *testing.T) {
		t.Parallel()
		e := enum(t, func(b *tsfixture.EnumBuilder) {
			b.VariantWith("Admin", "'admin'", func(v *tsfixture.VariantBuilder) {
				v.Docs("Full access.").
					Directive(tsfixture.Directive("skip")).
					Pos(position.Pos{File: "pinned.ts"})
			})
		})
		v := e.Variants[0]
		if len(v.DocLines) != 1 || len(v.DirectiveList) != 1 || v.Pos().File != "pinned.ts" {
			t.Fatalf("member = %+v", v)
		}
	})

	t.Run("a method on an enum is expressible", func(t *testing.T) {
		t.Parallel()
		// TypeScript has none, so this builds a shape no `.ts` file
		// produces — which a test asserting that a consumer ignores it
		// needs a way to make.
		e := enum(t, func(b *tsfixture.EnumBuilder) { b.Method("label", nil) })
		if len(e.Methods) != 1 || e.Methods[0].Owner != e {
			t.Fatalf("methods = %+v", e.Methods)
		}
	})
}

func TestPlainDeclarations(t *testing.T) {
	t.Parallel()

	t.Run("a function carries its whole signature", func(t *testing.T) {
		t.Parallel()
		b := tsfixture.New().Function("createUser", func(f *tsfixture.FunctionBuilder) {
			f.Docs("Builds one.").
				Directive(tsfixture.Directive("skip")).
				Pos(position.Pos{File: "pinned.ts"}).
				TypeParam("T", nil).
				Param("id", tsfixture.Named("string")).
				OptionalParam("nick", tsfixture.Named("string")).
				Rest("tags", tsfixture.Named("string")).
				Return(tsfixture.Named("User")).
				Async().Generator().
				Overload("createUser(id: string): User")
		})
		f := b.PackageNode().Functions[0]
		if len(f.Params) != 3 || len(f.Returns) != 1 || len(f.TypeParams) != 1 {
			t.Fatalf("signature = %+v", f)
		}
		if f.Params[0].Owner != f || f.TypeParams[0].Owner != f {
			t.Error("a member was built without its owner")
		}
		if a, _ := typescript.MetaAsync.Get(f.Meta()); !a {
			t.Error("async was dropped")
		}
		if g, _ := typescript.MetaGenerator.Get(f.Meta()); !g {
			t.Error("the generator marker was dropped")
		}
		if o, _ := typescript.MetaOverloads.Get(f.Meta()); len(o) != 1 {
			t.Error("the overload was dropped")
		}
		if f.Pos().File != "pinned.ts" || len(f.DocLines) != 1 || len(f.DirectiveList) != 1 {
			t.Errorf("function = %+v", f)
		}
	})

	t.Run("an alias carries its target and generics", func(t *testing.T) {
		t.Parallel()
		b := tsfixture.New().Alias("Box", func(a *tsfixture.AliasBuilder) {
			a.Docs("Holds one.").
				Directive(tsfixture.Directive("skip")).
				Pos(position.Pos{File: "pinned.ts"}).
				TypeParam("T", nil).
				Target(tsfixture.Object(tsfixture.Prop("value", tsfixture.TypeParamRef("T"))))
		})
		a := b.PackageNode().Aliases[0]
		if a.Target == nil || len(a.TypeParams) != 1 || a.TypeParams[0].Owner != a {
			t.Fatalf("alias = %+v", a)
		}
		if a.Pos().File != "pinned.ts" || len(a.DocLines) != 1 || len(a.DirectiveList) != 1 {
			t.Errorf("alias = %+v", a)
		}
	})

	t.Run("a binding carries its type and initialiser", func(t *testing.T) {
		t.Parallel()
		b := tsfixture.New().
			Variable("cache", func(v *tsfixture.VariableBuilder) {
				v.Docs("Held.").Directive(tsfixture.Directive("skip")).
					Pos(position.Pos{File: "v.ts"}).
					Type(tsfixture.Named("string")).Value("'x'")
			}).
			Constant("MAX", func(c *tsfixture.ConstantBuilder) {
				c.Docs("Ceiling.").Directive(tsfixture.Directive("skip")).
					Pos(position.Pos{File: "c.ts"}).
					Type(tsfixture.Named("number")).Value("100")
			})
		v := b.PackageNode().Variables[0]
		if v.Type == nil || v.InitExpr != "'x'" || v.Pos().File != "v.ts" {
			t.Fatalf("variable = %+v", v)
		}
		if len(v.DocLines) != 1 || len(v.DirectiveList) != 1 {
			t.Errorf("variable = %+v", v)
		}
		c := b.PackageNode().Constants[0]
		if c.Type == nil || c.Value != "100" || c.Pos().File != "c.ts" {
			t.Fatalf("constant = %+v", c)
		}
		if len(c.DocLines) != 1 || len(c.DirectiveList) != 1 {
			t.Errorf("constant = %+v", c)
		}
	})
}

func TestFileBuilderDocs(t *testing.T) {
	t.Parallel()

	t.Run("a module carries docs, a directive and a composed path", func(t *testing.T) {
		t.Parallel()
		// Composed the way a declaration's synthetic position is, so a
		// fixture pinning a declaration into this file and one
		// declaring the file agree on what its path is.
		b := tsfixture.New().File("user.ts", func(f *tsfixture.FileBuilder) {
			f.Docs("The user module.").Directive(tsfixture.Directive("skip"))
		})
		file := b.PackageNode().Files[0]
		if file.Path != "test/user.ts" || file.Owner != b.PackageNode() {
			t.Fatalf("file = %+v", file)
		}
		if len(file.DocLines) != 1 || len(file.DirectiveList) != 1 {
			t.Errorf("file = %+v", file)
		}
		if b.PackageNode().FileByName("user.ts") == nil {
			t.Error("the module is not reachable by name")
		}
	})
}

// iface builds one interface named User and returns the node.
func iface(t *testing.T, fn func(*tsfixture.InterfaceBuilder)) *node.Interface {
	t.Helper()
	return tsfixture.New().Interface("User", fn).PackageNode().Interfaces[0]
}

// class builds one class named Repo and returns the node.
func class(t *testing.T, fn func(*tsfixture.ClassBuilder)) *node.Struct {
	t.Helper()
	return tsfixture.New().Class("Repo", fn).PackageNode().Structs[0]
}

// method builds one method named find on a class and returns it.
func method(t *testing.T, fn func(*tsfixture.MethodBuilder)) *node.Method {
	t.Helper()
	return class(t, func(b *tsfixture.ClassBuilder) { b.Method("find", fn) }).Methods[0]
}

// enum builds one enum named Role and returns the node.
func enum(t *testing.T, fn func(*tsfixture.EnumBuilder)) *node.Enum {
	t.Helper()
	return tsfixture.New().Enum("Role", fn).PackageNode().Enums[0]
}

func TestNodeAccessors(t *testing.T) {
	t.Parallel()

	t.Run("every sub-builder hands back the node it configured", func(t *testing.T) {
		t.Parallel()
		// The escape hatch the package documents: a modifier with no
		// builder method is set through the typed key on this node, so
		// a sub-builder that did not expose one would put the fixture
		// author back to assembling structs by hand.
		b := tsfixture.New()

		var (
			cls    *node.Struct
			field  *node.Field
			meth   *node.Method
			ifc    *node.Interface
			enm    *node.Enum
			vrnt   *node.EnumVariant
			alias  *node.Alias
			fn     *node.Function
			binder *node.Variable
			konst  *node.Constant
			file   *node.File
		)
		b.Class("Repo", func(c *tsfixture.ClassBuilder) {
			cls = c.Node()
			c.Docs("A repository.").
				Directive(tsfixture.Directive("skip")).
				TypeParam("T", nil).
				Field("id", tsfixture.Named("string"), func(f *tsfixture.FieldBuilder) {
					field = f.Node()
				}).
				Method("find", func(m *tsfixture.MethodBuilder) { meth = m.Node() })
		}).
			Interface("Store", func(i *tsfixture.InterfaceBuilder) { ifc = i.Node() }).
			Enum("Role", func(e *tsfixture.EnumBuilder) {
				enm = e.Node()
				e.Pos(position.Pos{File: "role.ts"}).
					Docs("Access levels.").
					Directive(tsfixture.Directive("skip")).
					VariantWith("Admin", "'admin'", func(v *tsfixture.VariantBuilder) {
						vrnt = v.Node()
					})
			}).
			Alias("ID", func(a *tsfixture.AliasBuilder) { alias = a.Node() }).
			Function("create", func(f *tsfixture.FunctionBuilder) { fn = f.Node() }).
			Variable("cache", func(v *tsfixture.VariableBuilder) { binder = v.Node() }).
			Constant("MAX", func(c *tsfixture.ConstantBuilder) { konst = c.Node() }).
			File("repo.ts", func(f *tsfixture.FileBuilder) { file = f.Node() })

		for name, got := range map[string]bool{
			"class":     cls == b.PackageNode().Structs[0],
			"property":  field == b.PackageNode().Structs[0].Fields[0],
			"method":    meth == b.PackageNode().Structs[0].Methods[0],
			"interface": ifc == b.PackageNode().Interfaces[0],
			"enum":      enm == b.PackageNode().Enums[0],
			"member":    vrnt == b.PackageNode().Enums[0].Variants[0],
			"alias":     alias == b.PackageNode().Aliases[0],
			"function":  fn == b.PackageNode().Functions[0],
			"variable":  binder == b.PackageNode().Variables[0],
			"constant":  konst == b.PackageNode().Constants[0],
			"file":      file == b.PackageNode().Files[0],
		} {
			if !got {
				t.Errorf("%s: Node did not return the configured node", name)
			}
		}

		if len(cls.DocLines) != 1 || len(cls.DirectiveList) != 1 || len(cls.TypeParams) != 1 {
			t.Errorf("class = %+v", cls)
		}
		if enm.Pos().File != "role.ts" || len(enm.DocLines) != 1 || len(enm.DirectiveList) != 1 {
			t.Errorf("enum = %+v", enm)
		}
	})

	t.Run("a package-level directive is recorded", func(t *testing.T) {
		t.Parallel()
		b := tsfixture.New().Directive(tsfixture.Directive("skip"))
		if len(b.PackageNode().DirectiveList) != 1 {
			t.Fatal("the package directive was dropped")
		}
	})

	t.Run("a package with no name still routes its declarations", func(t *testing.T) {
		t.Parallel()
		// Fixture misuse, and the file must still carry a basename.
		b := tsfixture.New().PackageName("").Class("Repo", nil)
		assertFile(t, "class", b.PackageNode().Structs[0].Pos().File, "repo.ts")
	})
}
