// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"fmt"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestContext_ForBindsIdentityAndTarget covers the foundational
// [builder.For] constructor: the Context exposes the bound plugin
// identity and target unchanged, and provenance helpers stamp the
// same plugin name.
func TestContext_ForBindsIdentityAndTarget(t *testing.T) {
	t.Parallel()

	t.Run("SetBy and Target return the constructor arguments", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen", defaultTarget)
		if got := c.SetBy(); got != "repogen" {
			t.Fatalf("SetBy = %q, want %q", got, "repogen")
		}
		if got := c.Target(); got != defaultTarget {
			t.Fatalf("Target = %v, want %v", got, defaultTarget)
		}
	})

	t.Run("Provenance stamps SetBy and (optionally) ID", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen", defaultTarget)
		bare := c.Provenance()
		if bare.SetBy != "repogen" || bare.ID != "" {
			t.Fatalf("Provenance() = %+v; want SetBy=repogen ID=empty", bare)
		}
		withID := c.Provenance("seed")
		if withID.SetBy != "repogen" || withID.ID != "seed" {
			t.Fatalf("Provenance(\"seed\") = %+v; want SetBy=repogen ID=seed", withID)
		}
	})

	t.Run("WithTarget returns a copy without mutating the receiver", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen", defaultTarget)
		other := emit.Target{Dir: "audit", Filename: "audit.go", Package: "users"}
		c2 := c.WithTarget(other)
		if c.Target() != defaultTarget {
			t.Fatalf("receiver should be untouched; got %v", c.Target())
		}
		if c2.Target() != other {
			t.Fatalf("clone should carry the new target; got %v", c2.Target())
		}
	})
}

// TestContext_For_TargetOptional pins the variadic [emit.Target]
// argument: passing no target leaves the Context's default at the
// zero value. Production plugins call [builder.For] without a
// target so the pipeline's Layout phase composes placement.
func TestContext_For_TargetOptional(t *testing.T) {
	t.Parallel()

	t.Run("no target leaves Target at the zero value", func(t *testing.T) {
		t.Parallel()
		c := builder.For("p")
		if got := c.Target(); got != (emit.Target{}) {
			t.Fatalf("Target = %v, want zero", got)
		}
	})
}

// ExampleFor is the whole generator-side flow a plugin runs inside
// its Generate method: bind the plugin identity once, declare a
// package, populate a struct with fields and a method, and surface
// the graph with Build.
//
// Two things the example does by omission are the point of the
// builder. No Target is passed to [builder.For] — production plugins
// leave placement to the pipeline's Layout phase and stamp Origin
// per decl instead — and no `Owner` back-pointer is set by hand:
// the method's Owner is wired to the struct as the nested callback
// returns, and every decl inherits `SetBy` from the Context.
//
// The graph is inspected rather than rendered because rendering
// belongs to a backend, which lives in a different module; the
// assertions below are on the shape a backend would consume.
func ExampleFor() {
	c := builder.For("user-repo-gen")

	pkg, err := c.Package("users", "example.com/users").
		Struct("Repo", func(s *builder.StructBuilder) {
			s.Field("db", emit.Ptr(emit.External("database/sql", "DB")), nil)
			s.Method("Get", func(m *builder.MethodBuilder) {
				m.Receiver("r", emit.Ptr(emit.Internal(s.Node())))
				m.Param("ctx", emit.External("context", "Context"))
				m.Param("id", emit.Builtin("string"))
				m.Return(emit.Ptr(emit.External("example.com/users", "User")))
				m.Return(emit.Builtin("error"))
			})
		}).
		Build()
	if err != nil {
		fmt.Println("build:", err)
		return
	}

	repo := pkg.Structs[0]
	get := repo.MethodByName("Get")
	fmt.Println(repo.QName(), "set by", repo.SetBy())
	fmt.Println("field:", repo.Fields[0].Name)
	fmt.Printf("method: %s(%s, %s) returning %d values\n",
		get.Name, get.ParamAt(0).Name, get.ParamAt(1).Name, get.ReturnCount())
	fmt.Println("owner wired to:", get.OwnerQName())

	// Output:
	// example.com/users.Repo set by user-repo-gen
	// field: db
	// method: Get(ctx, id) returning 2 values
	// owner wired to: example.com/users.Repo
}
