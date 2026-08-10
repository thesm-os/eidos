// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/golangtest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/node"
)

// projectionFixture carries every declaration kind the projection
// emits, so the compile assertion below covers the whole surface in
// the one toolchain invocation it can afford.
//
// Deliberately stdlib-only in its cross-package references: the
// throwaway module the round trip builds has no dependencies, so a
// reference to anything else would fail on resolution rather than on
// the spelling under test.
func projectionFixture() *storefixture.Builder {
	return storefixture.New().
		Package("users", "example.com/app/users").
		Alias("UserID", func(a *storefixture.AliasBuilder) {
			a.Docs("UserID identifies a user.").Target(storefixture.Named("string")).True()
		}).
		Alias("Counter", func(a *storefixture.AliasBuilder) {
			a.Target(storefixture.Named("int"))
		}).
		Enum("Status", func(e *storefixture.EnumBuilder) {
			e.Underlying(storefixture.Named("int")).
				Variant("StatusDraft", "").
				Variant("StatusLive", "")
		}).
		Struct("User", func(s *storefixture.StructBuilder) {
			s.Docs("User is the entity a repository stores.")
			s.Embed(storefixture.PkgNamed("sync", "Mutex"))
			s.Field("ID", storefixture.Named("UserID"), func(f *storefixture.FieldBuilder) {
				f.Docs("ID is the primary key.").Tag(`json:"id"`)
			})
			s.Field("Tags", storefixture.Slice(storefixture.Named("string")), nil)
			s.Field("Meta", storefixture.Map(
				storefixture.Named("string"), storefixture.Named("any")), nil)
			s.Method("Validate", func(m *storefixture.MethodBuilder) {
				m.ReceiverName("u")
				m.Param("ctx", storefixture.PkgNamed("context", "Context"))
				m.NamedReturn("err", storefixture.Named("error"))
			})
		}).
		Struct("List", func(s *storefixture.StructBuilder) {
			s.TypeParam("T", nil)
			s.Field("Items", storefixture.Slice(storefixture.TypeParamRef("T")), nil)
			s.Method("First", func(m *storefixture.MethodBuilder) {
				m.Return(storefixture.TypeParamRef("T"))
			})
		}).
		Interface("Store", func(i *storefixture.InterfaceBuilder) {
			i.Embed(storefixture.PkgNamed("io", "Closer"))
			i.Method("Get", func(m *storefixture.MethodBuilder) {
				m.Param("ctx", storefixture.PkgNamed("context", "Context"))
				m.Param("id", storefixture.Named("string"))
				m.NamedReturn("item", storefixture.Pointer(storefixture.Named("User")))
				m.NamedReturn("err", storefixture.Named("error"))
			})
			i.Method("Put", func(m *storefixture.MethodBuilder) {
				m.Param("", storefixture.Named("string"))
				m.Return(storefixture.Named("error"))
			})
			i.Method("Log", func(m *storefixture.MethodBuilder) {
				m.Param("format", storefixture.Named("string"))
				m.Variadic("args", storefixture.Named("any"))
			})
			// A void method. Not spelled `Close`: the embed above
			// already carries one, and a duplicate method set is the
			// fixture's mistake rather than the projection's.
			i.Method("Flush", nil)
		}).
		Interface("Cache", func(i *storefixture.InterfaceBuilder) {
			i.TypeParam("T", storefixture.Constraint(storefixture.Named("any")))
			i.Method("Load", func(m *storefixture.MethodBuilder) {
				m.Param("key", storefixture.Named("string"))
				m.Return(storefixture.TypeParamRef("T"))
				m.Return(storefixture.Named("bool"))
			})
		}).
		Constant("MaxUsers", func(c *storefixture.ConstantBuilder) {
			c.Type(storefixture.Named("int")).Value("100")
		}).
		Variable("Registry", func(v *storefixture.VariableBuilder) {
			v.Type(storefixture.Map(storefixture.Named("string"),
				storefixture.Pointer(storefixture.Named("User"))))
		}).
		Function("New", func(f *storefixture.FunctionBuilder) {
			f.Param("id", storefixture.Named("string"))
			f.Return(storefixture.Pointer(storefixture.Named("User")))
			f.Return(storefixture.Named("error"))
		})
}

// TestBuilder_GoSource_RoundTrips is the assertion the projection
// exists for and the one every structural check below is a proxy for.
//
// The pair a fixture author used to keep in step by review — the node
// graph that drives the run and the Go the generated output
// references — is here derived from one source, and the only claim
// worth making about the derived half is that a Go compiler accepts
// it. Nothing cheaper reaches a dropped brace, an import the
// expressions need and the block omits, or a generic receiver missing
// its type arguments.
//
// One toolchain invocation over one fixture carrying every
// declaration kind, deliberately: `go build` costs seconds, and a
// fixture per kind would cost a minute for the same coverage.
func TestBuilder_GoSource_RoundTrips(t *testing.T) {
	t.Parallel()

	t.Run("the projected package compiles", func(t *testing.T) {
		t.Parallel()
		golangtest.Of(golangtest.GoFile(projectionFixture().GoSource())).
			AssertCompiles(t)
	})
}

func TestBuilder_GoSource(t *testing.T) {
	t.Parallel()

	t.Run("lands in the directory the fixture routes its output into", func(t *testing.T) {
		t.Parallel()
		// The support file has to share a directory with the generated
		// output for the toolchain to compile them as one package, and
		// the fixture stamps `<pkg>/…` on every declaration it builds.
		path, _ := projectionFixture().GoSource()
		if path != "users/source.go" {
			t.Errorf("GoSource path = %q, want users/source.go", path)
		}
	})

	t.Run("follows declarations repointed at the module root", func(t *testing.T) {
		t.Parallel()
		// A fixture that pins its positions decides where the run's
		// output lands, and the support file only compiles beside that
		// output if it moves with it. Assuming `<pkg>/` here would put
		// it one directory away and the failure would read as undefined
		// identifiers.
		path, _ := storefixture.New().
			Struct("Article", func(s *storefixture.StructBuilder) {
				s.Pos(position.Pos{File: "blog.go"})
			}).
			GoSource()
		if path != "source.go" {
			t.Errorf("GoSource path = %q, want source.go", path)
		}
	})

	t.Run("refuses declarations spread across directories", func(t *testing.T) {
		t.Parallel()
		// More than one directory is more than one package, which the
		// builder does not model and one file cannot carry.
		requireUnspellable(t, "spread across 2 directories", "the fixture package", func() {
			storefixture.New().
				Struct("Here", nil).
				Struct("There", func(s *storefixture.StructBuilder) {
					s.Pos(position.Pos{File: "elsewhere/there.go"})
				}).
				GoSource()
		})
	})

	t.Run("declares the package the fixture names", func(t *testing.T) {
		t.Parallel()
		_, src := projectionFixture().GoSource()
		if !strings.Contains(src, "\npackage users\n") {
			t.Errorf("projection does not declare package users:\n%s", src)
		}
	})

	t.Run("emits every declaration the builder accepted", func(t *testing.T) {
		t.Parallel()
		// Matched against the whitespace-collapsed form: gofmt aligns a
		// struct's field types and tags across their neighbours, so an
		// unrelated field added to the fixture would otherwise break
		// every assertion about the ones beside it.
		_, src := projectionFixture().GoSource()
		flat := strings.Join(strings.Fields(src), " ")
		for _, want := range []string{
			"type UserID = string",
			"type Counter int",
			"type Status int",
			"StatusDraft Status = iota",
			"type User struct {",
			"ID UserID `json:\"id\"`",
			"func (u *User) Validate(ctx context.Context) (err error)",
			"type List[T any] struct {",
			"func (*List[T]) First() T",
			"type Store interface {",
			"Log(format string, args ...any)",
			"type Cache[T any] interface {",
			"const MaxUsers int = 100",
			"var Registry map[string]*User",
			"func New(id string) (*User, error)",
		} {
			if !strings.Contains(flat, want) {
				t.Errorf("projection is missing %q:\n%s", want, src)
			}
		}
	})

	t.Run("follows a rename in the fixture rather than going stale", func(t *testing.T) {
		t.Parallel()
		// The whole point: the hand-written support package this
		// replaces could not do this, and the failure it produced
		// instead was a compile error naming code nobody wrote.
		_, src := storefixture.New().
			Struct("User", func(s *storefixture.StructBuilder) {
				s.Field("Identifier", storefixture.Named("string"), nil)
			}).
			GoSource()
		if strings.Contains(src, "ID ") || !strings.Contains(src, "Identifier string") {
			t.Errorf("projection did not follow the field rename:\n%s", src)
		}
	})

	t.Run("carries no generated-code marker", func(t *testing.T) {
		t.Parallel()
		// It stands in for code a consumer hand-wrote. Stamping it as
		// generated would make it lie to every assertion keyed off the
		// marker, and to whoever reads it in a failure dump.
		_, src := projectionFixture().GoSource()
		if strings.Contains(src, "Code generated") {
			t.Errorf("projection claims to be generated code:\n%s", src)
		}
	})
}

func TestBuilder_GoSource_Imports(t *testing.T) {
	t.Parallel()

	t.Run("imports the packages the type expressions name", func(t *testing.T) {
		t.Parallel()
		_, src := projectionFixture().GoSource()
		for _, want := range []string{`"context"`, `"io"`, `"sync"`} {
			if !strings.Contains(src, want) {
				t.Errorf("projection does not import %s:\n%s", want, src)
			}
		}
	})

	t.Run("leaves a reference to the fixture's own package unqualified", func(t *testing.T) {
		t.Parallel()
		// Every declaration the fixture builds is stamped with the
		// package's own path, so a struct's own method receiver arrives
		// as `example.com/app/users.User`. Qualified, that is a package
		// importing itself.
		_, src := projectionFixture().GoSource()
		if strings.Contains(src, "users.User") {
			t.Errorf("projection qualified a self-reference:\n%s", src)
		}
	})

	t.Run("aliases a second import whose last segment collides", func(t *testing.T) {
		t.Parallel()
		// Both would otherwise render as `models`, and the file would
		// compile while naming the wrong package's types.
		_, src := storefixture.New().
			Struct("Pair", func(s *storefixture.StructBuilder) {
				s.Field("A", storefixture.PkgNamed("example.com/one/models", "T"), nil)
				s.Field("B", storefixture.PkgNamed("example.com/two/models", "T"), nil)
			}).
			GoSource()
		if !strings.Contains(src, `models2 "example.com/two/models"`) {
			t.Errorf("projection did not alias the colliding import:\n%s", src)
		}
		if !strings.Contains(src, "B models2.T") {
			t.Errorf("projection did not use the alias at the reference:\n%s", src)
		}
	})

	t.Run("drops an import nothing in the package references", func(t *testing.T) {
		t.Parallel()
		// Builder.Import seeds the frontend's import view for tests
		// that inspect it. Emitted here it would be an unused import,
		// which is a compile error rather than a faithful projection.
		_, src := storefixture.New().Import("context").Struct("S", nil).GoSource()
		if strings.Contains(src, "context") {
			t.Errorf("projection emitted an unreferenced import:\n%s", src)
		}
	})

	t.Run("omits the import block when nothing needs one", func(t *testing.T) {
		t.Parallel()
		_, src := storefixture.New().Struct("S", nil).GoSource()
		if strings.Contains(src, "import") {
			t.Errorf("projection emitted an empty import block:\n%s", src)
		}
	})
}

func TestBuilder_GoSource_Signatures(t *testing.T) {
	t.Parallel()

	t.Run("keeps an all-anonymous parameter list anonymous", func(t *testing.T) {
		t.Parallel()
		if got := methodSpecOf(t, func(m *storefixture.MethodBuilder) {
			m.Param("", storefixture.Named("int"))
			m.Param("", storefixture.Named("string"))
		}); !strings.Contains(got, "M(int, string)") {
			t.Errorf("anonymous parameters were named: %s", got)
		}
	})

	t.Run("promotes an anonymous slot to _ beside a named one", func(t *testing.T) {
		t.Parallel()
		// Go rejects a list mixing the two forms, and a fixture is free
		// to build one. `_` says what the fixture said — this slot has
		// no name — in the only syntax Go has for saying it.
		if got := methodSpecOf(t, func(m *storefixture.MethodBuilder) {
			m.Param("id", storefixture.Named("string"))
			m.Param("", storefixture.Named("int"))
		}); !strings.Contains(got, "M(id string, _ int)") {
			t.Errorf("mixed parameter list was not promoted: %s", got)
		}
	})

	t.Run("marks a variadic tail", func(t *testing.T) {
		t.Parallel()
		// A dropped marker produces a double that compiles and
		// satisfies nothing, which is the failure the support package
		// is supposed to expose.
		if got := methodSpecOf(t, func(m *storefixture.MethodBuilder) {
			m.Variadic("opts", storefixture.Named("string"))
		}); !strings.Contains(got, "M(opts ...string)") {
			t.Errorf("variadic marker missing: %s", got)
		}
	})

	t.Run("leaves a lone unnamed return unbracketed", func(t *testing.T) {
		t.Parallel()
		if got := methodSpecOf(t, func(m *storefixture.MethodBuilder) {
			m.Return(storefixture.Named("error"))
		}); !strings.Contains(got, "M() error") {
			t.Errorf("lone return was bracketed: %s", got)
		}
	})

	t.Run("brackets a lone named return", func(t *testing.T) {
		t.Parallel()
		if got := methodSpecOf(t, func(m *storefixture.MethodBuilder) {
			m.NamedReturn("err", storefixture.Named("error"))
		}); !strings.Contains(got, "M() (err error)") {
			t.Errorf("named return was not bracketed: %s", got)
		}
	})

	t.Run("composes a generic receiver with its type arguments", func(t *testing.T) {
		t.Parallel()
		// The node's receiver is a bare `*Pkg.List`, which on a generic
		// type does not compile. The arguments can only come from the
		// owner.
		_, src := storefixture.New().
			Struct("List", func(s *storefixture.StructBuilder) {
				s.TypeParam("T", nil)
				s.Method("Len", func(m *storefixture.MethodBuilder) {
					m.ReceiverName("l")
					m.Return(storefixture.Named("int"))
				})
			}).
			GoSource()
		if !strings.Contains(src, "func (l *List[T]) Len() int") {
			t.Errorf("generic receiver missing its type arguments:\n%s", src)
		}
	})

	t.Run("honours a value receiver the fixture pinned", func(t *testing.T) {
		t.Parallel()
		_, src := storefixture.New().
			Struct("User", func(s *storefixture.StructBuilder) {
				s.Method("String", func(m *storefixture.MethodBuilder) {
					m.ReceiverName("u")
					m.Receiver(storefixture.PkgNamed("example.com/test", "User"))
					m.Return(storefixture.Named("string"))
				})
			}).
			GoSource()
		if !strings.Contains(src, "func (u User) String() string") {
			t.Errorf("value receiver was not honoured:\n%s", src)
		}
	})
}

// TestBuilder_GoSource_Refuses covers the half of the contract that
// keeps the projection honest.
//
// A projection that quietly skipped what it could not spell would
// reintroduce the drift one file further down — a support package
// missing a field, and a compile error naming the generated code
// instead. Every case here therefore asserts a stop, and asserts that
// the message names the construct: a failure that does not is one the
// author has to bisect the fixture to understand.
func TestBuilder_GoSource_Refuses(t *testing.T) {
	t.Parallel()

	t.Run("a field with no type", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "nil type reference", "field ID", func() {
			storefixture.New().Struct("User", func(s *storefixture.StructBuilder) {
				s.Field("ID", nil, nil)
			}).GoSource()
		})
	})

	t.Run("a field with no name", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "field with no name", "struct User", func() {
			storefixture.New().Struct("User", func(s *storefixture.StructBuilder) {
				s.Field("", storefixture.Named("string"), nil)
			}).GoSource()
		})
	})

	t.Run("a type reference kind it does not know", func(t *testing.T) {
		t.Parallel()
		// The graph gains variants over time; one arriving here must
		// stop the run rather than vanish from the support package.
		requireUnspellable(t, "kind type_ref_kind(?)", "field X", func() {
			storefixture.New().Struct("S", func(s *storefixture.StructBuilder) {
				s.Field("X", &node.TypeRef{TypeKind: node.TypeRefKind(99)}, nil)
			}).GoSource()
		})
	})

	t.Run("a type reference that points at itself", func(t *testing.T) {
		t.Parallel()
		// Nothing a parser could produce, and everything a hand-wired
		// fixture can. Bounded so it names a cycle instead of taking
		// the test binary down with a stack overflow.
		cyclic := &node.TypeRef{TypeKind: node.TypeRefPointer}
		cyclic.Elem = cyclic
		requireUnspellable(t, "nested past", "field X", func() {
			storefixture.New().Struct("S", func(s *storefixture.StructBuilder) {
				s.Field("X", cyclic, nil)
			}).GoSource()
		})
	})

	t.Run("a variadic parameter that is not last", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "variadic parameter that is not the last", "method M", func() {
			storefixture.New().Interface("I", func(i *storefixture.InterfaceBuilder) {
				i.Method("M", func(m *storefixture.MethodBuilder) {
					m.Variadic("opts", storefixture.Named("string"))
					m.Param("id", storefixture.Named("string"))
				})
			}).GoSource()
		})
	})

	t.Run("a type parameter on a method", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "only on the receiver type", "method M", func() {
			storefixture.New().Struct("S", func(s *storefixture.StructBuilder) {
				s.Method("M", func(m *storefixture.MethodBuilder) {
					m.TypeParam("T", nil)
				})
			}).GoSource()
		})
	})

	t.Run("a receiver naming a type other than its owner", func(t *testing.T) {
		t.Parallel()
		// Composing the receiver from the owner is what supplies a
		// generic type's arguments; refusing here is what stops that
		// composition silently discarding what a test pinned.
		requireUnspellable(t, "naming Other rather than User", "method M", func() {
			storefixture.New().Struct("User", func(s *storefixture.StructBuilder) {
				s.Method("M", func(m *storefixture.MethodBuilder) {
					m.Receiver(storefixture.Pointer(
						storefixture.PkgNamed("example.com/test", "Other")))
				})
			}).GoSource()
		})
	})

	t.Run("an enum with no underlying type", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "no underlying type", "enum Status", func() {
			storefixture.New().Enum("Status", func(e *storefixture.EnumBuilder) {
				e.Variant("StatusDraft", "1")
			}).GoSource()
		})
	})

	t.Run("an enum variant with no value after a valued one", func(t *testing.T) {
		t.Parallel()
		// Legal Go — it repeats the previous expression — and almost
		// never what the fixture meant, so it is refused rather than
		// guessed at.
		requireUnspellable(t, "after a valued sibling", "enum Status", func() {
			storefixture.New().Enum("Status", func(e *storefixture.EnumBuilder) {
				e.Underlying(storefixture.Named("int")).
					Variant("StatusDraft", "1").
					Variant("StatusLive", "")
			}).GoSource()
		})
	})

	t.Run("a type declaration with no target", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "no target type", "type Handle", func() {
			storefixture.New().Alias("Handle", nil).GoSource()
		})
	})

	t.Run("a constant with no value", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "constant with no value", "constant Max", func() {
			storefixture.New().Constant("Max", nil).GoSource()
		})
	})

	t.Run("a variable with neither a type nor an initialiser", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "neither a type nor an initialiser", "variable Reg", func() {
			storefixture.New().Variable("Reg", nil).GoSource()
		})
	})

	t.Run("a package with no name", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "package with no name", "the fixture package", func() {
			storefixture.New().Package("", "example.com/test").GoSource()
		})
	})

	t.Run("a struct tag holding a backquote", func(t *testing.T) {
		t.Parallel()
		// No Go raw string literal can hold one, and a tag is only
		// spellable as a raw string.
		requireUnspellable(t, "backquote", "field ID", func() {
			storefixture.New().Struct("User", func(s *storefixture.StructBuilder) {
				s.Field("ID", storefixture.Named("string"), func(f *storefixture.FieldBuilder) {
					f.Tag("json:\"a`b\"")
				})
			}).GoSource()
		})
	})
}

// methodSpecOf projects a one-method interface and returns the whole
// file, so a signature assertion reads as the line an author would
// have written rather than as an assembled fragment.
func methodSpecOf(t *testing.T, fn func(*storefixture.MethodBuilder)) string {
	t.Helper()
	_, src := storefixture.New().
		Interface("I", func(i *storefixture.InterfaceBuilder) { i.Method("M", fn) }).
		GoSource()
	return src
}

// requireUnspellable asserts that fn stops with an
// [storefixture.UnspellableError] naming both the construct and where
// it was found.
//
// Both halves are checked because either alone is unactionable: a
// message naming the construct without the declaration sends the
// author bisecting the fixture, and one naming the declaration
// without the construct says only that something in it is wrong.
func requireUnspellable(t *testing.T, construct, where string, fn func()) {
	t.Helper()
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("GoSource returned normally; want a stop naming %q", construct)
		}
		err, ok := rec.(error)
		if !ok {
			t.Fatalf("panicked with %T (%v); want an error value", rec, rec)
		}
		var unspellable *storefixture.UnspellableError
		if !errors.As(err, &unspellable) {
			t.Fatalf("panicked with %T (%v); want *storefixture.UnspellableError", err, err)
		}
		if !strings.Contains(unspellable.Construct, construct) {
			t.Errorf("construct = %q, want it to name %q", unspellable.Construct, construct)
		}
		if !strings.Contains(unspellable.Where, where) {
			t.Errorf("where = %q, want it to name %q", unspellable.Where, where)
		}
	}()
	fn()
}
