// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/lang/typescript/typescripttest/tsfixture"
)

func TestTSSourceDeclarations(t *testing.T) {
	t.Parallel()

	t.Run("an alias renders exported with its generics", func(t *testing.T) {
		t.Parallel()
		src := project(t, tsfixture.New().Alias("Box", func(a *tsfixture.AliasBuilder) {
			a.TypeParam("T", tsfixture.Constraint(tsfixture.Named("object"))).
				Target(tsfixture.Array(tsfixture.TypeParamRef("T")))
		}))
		assertLine(t, src, "export type Box<T extends object> = T[];")
	})

	t.Run("an enum renders its members with trailing commas", func(t *testing.T) {
		t.Parallel()
		src := project(t, tsfixture.New().Enum("Role", func(e *tsfixture.EnumBuilder) {
			e.Strings().Variant("Admin", "'admin'").Variant("Guest", "")
		}))
		assertLine(t, src, "export enum Role {")
		assertLine(t, src, "  Admin = 'admin',")
		assertLine(t, src, "  Guest,")
	})

	t.Run("a const enum keeps its keyword", func(t *testing.T) {
		t.Parallel()
		src := project(t, tsfixture.New().Enum("Role", func(e *tsfixture.EnumBuilder) {
			e.Const().Variant("Admin", "'admin'")
		}))
		assertLine(t, src, "export const enum Role {")
	})

	t.Run("an interface renders properties and signatures", func(t *testing.T) {
		t.Parallel()
		src := project(t, tsfixture.New().Interface("User", func(i *tsfixture.InterfaceBuilder) {
			i.Extends(tsfixture.Named("Base")).
				IndexSignature("[key: string]: unknown").
				Field("id", tsfixture.Named("string"), nil).
				Field("nick", tsfixture.Named("string"), func(f *tsfixture.FieldBuilder) {
					f.Optional().Readonly()
				}).
				Method("greet", func(m *tsfixture.MethodBuilder) {
					m.Param("loud", tsfixture.Named("boolean")).Return(tsfixture.Named("string"))
				})
		}))
		assertLine(t, src, "export interface User extends Base {")
		assertLine(t, src, "  [key: string]: unknown;")
		assertLine(t, src, "  id: string;")
		assertLine(t, src, "  readonly nick?: string;")
		assertLine(t, src, "  greet(loud: boolean): string;")
	})

	t.Run("a class is declared rather than defined", func(t *testing.T) {
		t.Parallel()
		// The projection emits declarations, and a class with bodiless
		// methods is not one TypeScript will load.
		src := project(t, tsfixture.New().Class("Repo", func(c *tsfixture.ClassBuilder) {
			c.Abstract().
				Extends(tsfixture.Named("Base")).
				Implements(tsfixture.Named("Store")).
				Field("cache", tsfixture.Named("string"), func(f *tsfixture.FieldBuilder) {
					f.Visibility(typescript.VisibilityPrivate).Static()
				}).
				Method("find", func(m *tsfixture.MethodBuilder) {
					m.Abstract().Return(tsfixture.Named("string"))
				})
		}))
		assertLine(t, src, "export declare abstract class Repo extends Base implements Store {")
		assertLine(t, src, "  private static cache: string;")
		assertLine(t, src, "  abstract find(): string;")
	})

	t.Run("async is dropped from a declared method", func(t *testing.T) {
		t.Parallel()
		// It describes how a body produces its result, and a
		// declaration carrying it is rejected outright. The return type
		// is what a caller binds to, so nothing visible is lost.
		src := project(t, tsfixture.New().Class("Repo", func(c *tsfixture.ClassBuilder) {
			c.Method("load", func(m *tsfixture.MethodBuilder) {
				m.Async().Return(tsfixture.Named("string"))
			})
		}))
		if strings.Contains(src, "async") {
			t.Fatalf("a declaration carries async:\n%s", src)
		}
	})

	t.Run("a getter keeps its keyword", func(t *testing.T) {
		t.Parallel()
		// Its use site is a property read, so projecting one as an
		// ordinary method would give the support file a surface the
		// source does not have.
		src := project(t, tsfixture.New().Class("Repo", func(c *tsfixture.ClassBuilder) {
			c.Method("size", func(m *tsfixture.MethodBuilder) {
				m.Accessor(typescript.AccessorGet).Return(tsfixture.Named("number"))
			})
		}))
		assertLine(t, src, "  get size(): number;")
	})

	t.Run("the plain declarations render", func(t *testing.T) {
		t.Parallel()
		src := project(t, tsfixture.New().
			Constant("MAX", func(c *tsfixture.ConstantBuilder) {
				c.Type(tsfixture.Named("number"))
			}).
			Variable("cache", func(v *tsfixture.VariableBuilder) {
				v.Type(tsfixture.Named("string"))
			}).
			Function("createUser", func(f *tsfixture.FunctionBuilder) {
				f.TypeParam("T", nil).
					Param("id", tsfixture.Named("string")).
					OptionalParam("nick", tsfixture.Named("string")).
					Rest("tags", tsfixture.Named("string")).
					Return(tsfixture.TypeParamRef("T"))
			}))
		assertLine(t, src, "export declare const MAX: number;")
		assertLine(t, src, "export declare let cache: string;")
		assertLine(t, src,
			"export declare function createUser<T>(id: string, nick?: string, ...tags: string[]): T;")
	})

	t.Run("a signature with no return is void", func(t *testing.T) {
		t.Parallel()
		src := project(t, tsfixture.New().Function("reset", nil))
		assertLine(t, src, "export declare function reset(): void;")
	})

	t.Run("several returns become the tuple", func(t *testing.T) {
		t.Parallel()
		// TypeScript returns one value, and a signature carrying more
		// than one is the tuple that holds them.
		src := project(t, tsfixture.New().Function("split", func(f *tsfixture.FunctionBuilder) {
			f.Return(tsfixture.Named("string")).Return(tsfixture.Named("number"))
		}))
		assertLine(t, src, "export declare function split(): [string, number];")
	})

	t.Run("an unnamed parameter is named positionally", func(t *testing.T) {
		t.Parallel()
		src := project(t, tsfixture.New().Function("f", func(b *tsfixture.FunctionBuilder) {
			b.Param("", tsfixture.Named("string"))
		}))
		assertLine(t, src, "export declare function f(arg0: string): void;")
	})

	t.Run("a reserved parameter name is made bindable", func(t *testing.T) {
		t.Parallel()
		src := project(t, tsfixture.New().Function("f", func(b *tsfixture.FunctionBuilder) {
			b.Param("class", tsfixture.Named("string"))
		}))
		assertLine(t, src, "export declare function f(class_: string): void;")
	})

	t.Run("docs are projected as JSDoc", func(t *testing.T) {
		t.Parallel()
		// A support file turns up in every toolchain failure dump, and
		// a reader checking it against the fixture is reading two
		// things that should say the same words.
		src := project(t, tsfixture.New().Interface("User", func(i *tsfixture.InterfaceBuilder) {
			i.Docs("A user of the system.", "", "Second paragraph.").
				Field("id", tsfixture.Named("string"), func(f *tsfixture.FieldBuilder) {
					f.Docs("The identifier.")
				})
		}))
		assertLine(t, src, "/**")
		assertLine(t, src, " * A user of the system.")
		assertLine(t, src, " *")
		assertLine(t, src, "   * The identifier.")
	})
}

func TestTSSourceImports(t *testing.T) {
	t.Parallel()

	t.Run("a referenced module is imported type-only", func(t *testing.T) {
		t.Parallel()
		// Nothing the projection writes constructs a value.
		src := project(t, tsfixture.New().Interface("User", func(i *tsfixture.InterfaceBuilder) {
			i.Field("owner", tsfixture.ModNamed("./models/person", "Person"), nil)
		}))
		assertLine(t, src, "import type { Person } from './models/person';")
	})

	t.Run("two names from one module share a statement", func(t *testing.T) {
		t.Parallel()
		// TypeScript binds a set of names per import statement.
		src := project(t, tsfixture.New().Interface("User", func(i *tsfixture.InterfaceBuilder) {
			i.Field("owner", tsfixture.ModNamed("./m", "Person"), nil).
				Field("role", tsfixture.ModNamed("./m", "Role"), nil)
		}))
		assertLine(t, src, "import type { Person, Role } from './m';")
	})

	t.Run("imports are sorted so the file is byte-stable", func(t *testing.T) {
		t.Parallel()
		src := project(t, tsfixture.New().Interface("User", func(i *tsfixture.InterfaceBuilder) {
			i.Field("z", tsfixture.ModNamed("./z", "Z"), nil).
				Field("a", tsfixture.ModNamed("./a", "A"), nil)
		}))
		if strings.Index(src, "./a") > strings.Index(src, "./z") {
			t.Fatalf("imports are not sorted:\n%s", src)
		}
	})

	t.Run("a package-level import nothing references is dropped", func(t *testing.T) {
		t.Parallel()
		// An import nothing references is an error under
		// noUnusedLocals, and those entries exist for tests inspecting
		// a frontend's import view.
		src := project(t, tsfixture.New().Import("./unused").Interface("User", nil))
		if strings.Contains(src, "./unused") {
			t.Fatalf("an unreferenced import was projected:\n%s", src)
		}
	})

	t.Run("a package referencing nothing writes no import block", func(t *testing.T) {
		t.Parallel()
		src := project(t, tsfixture.New().Interface("User", nil))
		if strings.Contains(src, "import") {
			t.Fatalf("an empty import block was written:\n%s", src)
		}
	})
}

func TestTSSourcePath(t *testing.T) {
	t.Parallel()

	t.Run("the file lands where the declarations say they live", func(t *testing.T) {
		t.Parallel()
		// The support file is only importable by the generated output
		// if the specifier the output writes resolves to it.
		path, _ := tsfixture.New().Interface("User", nil).TSSource()
		if path != "test/index.ts" {
			t.Fatalf("path = %q", path)
		}
	})

	t.Run("an empty package falls back to the package name", func(t *testing.T) {
		t.Parallel()
		path, _ := tsfixture.New().TSSource()
		if path != "test/index.ts" {
			t.Fatalf("path = %q", path)
		}
	})

	t.Run("a declaration at the root writes to the root", func(t *testing.T) {
		t.Parallel()
		path, _ := tsfixture.New().Interface("User", func(i *tsfixture.InterfaceBuilder) {
			i.Pos(position.Pos{File: "user.ts"})
		}).TSSource()
		if path != "index.ts" {
			t.Fatalf("path = %q", path)
		}
	})

	t.Run("declarations in two directories are refused", func(t *testing.T) {
		t.Parallel()
		// More than one module and more than one file, which one
		// projection cannot be.
		assertUnspellable(t, func() {
			tsfixture.New().
				Interface("A", func(i *tsfixture.InterfaceBuilder) {
					i.Pos(position.Pos{File: "one/a.ts"})
				}).
				Interface("B", func(i *tsfixture.InterfaceBuilder) {
					i.Pos(position.Pos{File: "two/b.ts"})
				}).
				TSSource()
		})
	})
}

func TestTSSourceRefusals(t *testing.T) {
	t.Parallel()

	t.Run("each unspellable shape names itself", func(t *testing.T) {
		t.Parallel()
		// A support module missing a property is precisely the drift
		// the projection exists to kill, so it refuses rather than
		// emits.
		for name, build := range map[string]func(){
			"a package with no name": func() {
				tsfixture.New().PackageName("").TSSource()
			},
			"an alias naming no type": func() {
				tsfixture.New().Alias("ID", nil).TSSource()
			},
			"a property with no name": func() {
				tsfixture.New().Interface("A", func(i *tsfixture.InterfaceBuilder) {
					i.Field("", tsfixture.Named("string"), nil)
				}).TSSource()
			},
			"a method with no name": func() {
				tsfixture.New().Interface("A", func(i *tsfixture.InterfaceBuilder) {
					i.Method("", nil)
				}).TSSource()
			},
			"an enum member with no name": func() {
				tsfixture.New().Enum("E", func(e *tsfixture.EnumBuilder) {
					e.Variant("", "")
				}).TSSource()
			},
			"a generic parameter with no name": func() {
				tsfixture.New().Interface("A", func(i *tsfixture.InterfaceBuilder) {
					i.TypeParam("", nil)
				}).TSSource()
			},
			"a hard-private member": func() {
				tsfixture.New().Class("A", func(c *tsfixture.ClassBuilder) {
					c.Field("x", tsfixture.Named("string"), func(f *tsfixture.FieldBuilder) {
						f.Visibility(typescript.VisibilityHard)
					})
				}).TSSource()
			},
		} {
			t.Run(name+" is refused", func(t *testing.T) {
				t.Parallel()
				assertUnspellable(t, build)
			})
		}
	})

	t.Run("the failure names the construct and the declaration", func(t *testing.T) {
		t.Parallel()
		err := unspellable(t, func() { tsfixture.New().Alias("ID", nil).TSSource() })
		if !strings.Contains(err.Error(), "alias ID") {
			t.Errorf("the message does not name the declaration: %v", err)
		}
		if !strings.Contains(err.Error(), "hand-write") {
			t.Errorf("the message does not say what to do instead: %v", err)
		}
	})
}

// project renders a builder's package and returns the source.
func project(t *testing.T, b *tsfixture.Builder) string {
	t.Helper()
	_, src := b.TSSource()
	return src
}

// assertLine reports a projection missing a whole line, so a failure
// names the construct rather than a substring of one.
func assertLine(t *testing.T, src, want string) {
	t.Helper()
	if slices.Contains(strings.Split(src, "\n"), want) {
		return
	}
	t.Errorf("no line reads %q; got:\n%s", want, src)
}

// assertUnspellable runs build and requires it to refuse.
func assertUnspellable(t *testing.T, build func()) {
	t.Helper()
	_ = unspellable(t, build)
}

// unspellable runs build and returns the refusal it raised.
func unspellable(t *testing.T, build func()) *tsfixture.UnspellableError {
	t.Helper()
	var got *tsfixture.UnspellableError
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("the projection emitted a construct it cannot spell")
			}
			err, ok := r.(error)
			if !ok || !errors.As(err, &got) {
				t.Fatalf("panicked with %v, want an UnspellableError", r)
			}
		}()
		build()
	}()
	return got
}
