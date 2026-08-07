// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
)

// ptrTo wraps a ref in a pointer.
func ptrTo(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: elem}
}

func TestDeclares(t *testing.T) {
	t.Parallel()

	methods := []*node.Method{{Name: "Get"}, nil, {Name: "Put"}}

	t.Run("finds a declared method", func(t *testing.T) {
		t.Parallel()
		if !node.Declares(methods, "Put") {
			t.Fatalf("Declares(Put) = false")
		}
	})

	t.Run("reports false for an absent method", func(t *testing.T) {
		t.Parallel()
		if node.Declares(methods, "Delete") {
			t.Fatalf("Declares(Delete) = true")
		}
	})

	t.Run("reports false for the empty name", func(t *testing.T) {
		t.Parallel()
		// A caller passing an unset field must not match a method
		// whose own name is somehow empty.
		if node.Declares([]*node.Method{{Name: ""}}, "") {
			t.Fatalf("the empty name must never match")
		}
	})

	t.Run("tolerates a nil entry in the set", func(t *testing.T) {
		t.Parallel()
		// Hand-built graphs and partially-populated fixtures carry
		// them; a panic here would be a framework fault reported
		// against the caller's plugin.
		if !node.Declares(methods, "Get") {
			t.Fatalf("a nil entry must not stop the walk")
		}
	})
}

func TestPointerReceiver(t *testing.T) {
	t.Parallel()

	methods := []*node.Method{
		{Name: "Value", Receiver: namedRef("x", "T")},
		{Name: "Ptr", Receiver: ptrTo(namedRef("x", "T"))},
		{Name: "Bare"},
	}

	t.Run("reports true for a pointer receiver", func(t *testing.T) {
		t.Parallel()
		if !node.PointerReceiver(methods, "Ptr") {
			t.Fatalf("PointerReceiver(Ptr) = false")
		}
	})

	t.Run("reports false for a value receiver", func(t *testing.T) {
		t.Parallel()
		if node.PointerReceiver(methods, "Value") {
			t.Fatalf("PointerReceiver(Value) = true")
		}
	})

	t.Run("reports false for an absent method", func(t *testing.T) {
		t.Parallel()
		// The value form is the safer default: it fails to compile
		// against a pointer-receiver set, which is loud and locatable.
		if node.PointerReceiver(methods, "Missing") {
			t.Fatalf("an absent method must not claim a pointer receiver")
		}
	})

	t.Run("reports false for a method carrying no receiver", func(t *testing.T) {
		t.Parallel()
		// Interface methods have none — the receiver is implicit.
		if node.PointerReceiver(methods, "Bare") {
			t.Fatalf("a receiverless method must not claim a pointer receiver")
		}
	})
}

func TestFieldOfType(t *testing.T) {
	t.Parallel()

	s := &node.Struct{
		Name: "Record",
		Fields: []*node.Field{
			{Name: "unexported", Type: namedRef("", "string")},
			{Name: "Name", Type: namedRef("", "string")},
			{Name: "Alt", Type: namedRef("", "string")},
			{Name: "Count", Type: namedRef("", "int")},
			{Name: "Ref", Type: namedRef("pkg", "Other")},
		},
	}

	t.Run("returns the first exported field of the named builtin", func(t *testing.T) {
		t.Parallel()
		got := node.FieldOfType(s, "string")
		if got == nil || got.Name != "Name" {
			t.Fatalf("FieldOfType(string) = %+v, want Name", got)
		}
	})

	t.Run("skips an unexported field of the right type", func(t *testing.T) {
		t.Parallel()
		// A generated consumer cannot reach it, so offering it would
		// produce code that does not compile.
		if got := node.FieldOfType(s, "string"); got.Name == "unexported" {
			t.Fatalf("FieldOfType returned an unexported field")
		}
	})

	t.Run("returns nil when no field carries the type", func(t *testing.T) {
		t.Parallel()
		if got := node.FieldOfType(s, "float64"); got != nil {
			t.Fatalf("FieldOfType(float64) = %+v, want nil", got)
		}
	})

	t.Run("does not match a non-builtin field of the same name", func(t *testing.T) {
		t.Parallel()
		if got := node.FieldOfType(s, "Other"); got != nil {
			t.Fatalf("FieldOfType(Other) = %+v, want nil for a qualified type", got)
		}
	})

	t.Run("returns nil for a nil struct", func(t *testing.T) {
		t.Parallel()
		if got := node.FieldOfType(nil, "string"); got != nil {
			t.Fatalf("FieldOfType(nil) = %+v, want nil", got)
		}
	})

	t.Run("returns nil for the empty type name", func(t *testing.T) {
		t.Parallel()
		if got := node.FieldOfType(s, ""); got != nil {
			t.Fatalf("FieldOfType(\"\") = %+v, want nil", got)
		}
	})
}

func TestEmbedName(t *testing.T) {
	t.Parallel()

	t.Run("returns the identifier of a value embed", func(t *testing.T) {
		t.Parallel()
		name, byPtr := node.EmbedName(&node.Embed{Type: namedRef("io", "Reader")})
		if name != "Reader" || byPtr {
			t.Fatalf("EmbedName = (%q, %v), want (Reader, false)", name, byPtr)
		}
	})

	t.Run("unwraps a pointer embed to reach its identifier", func(t *testing.T) {
		t.Parallel()
		// The name rides on the pointee, so reading the reference's
		// own name yields empty and the field is silently dropped from
		// anything derived from it.
		name, byPtr := node.EmbedName(&node.Embed{Type: ptrTo(namedRef("io", "Reader"))})
		if name != "Reader" || !byPtr {
			t.Fatalf("EmbedName = (%q, %v), want (Reader, true)", name, byPtr)
		}
	})

	t.Run("reports by-pointer for a pointer with no pointee", func(t *testing.T) {
		t.Parallel()
		name, byPtr := node.EmbedName(&node.Embed{Type: ptrTo(nil)})
		if name != "" || !byPtr {
			t.Fatalf("EmbedName = (%q, %v), want (\"\", true)", name, byPtr)
		}
	})

	t.Run("returns empty for a nil embed", func(t *testing.T) {
		t.Parallel()
		if name, byPtr := node.EmbedName(nil); name != "" || byPtr {
			t.Fatalf("EmbedName(nil) = (%q, %v), want empty", name, byPtr)
		}
	})

	t.Run("returns empty for an embed carrying no type", func(t *testing.T) {
		t.Parallel()
		if name, _ := node.EmbedName(&node.Embed{}); name != "" {
			t.Fatalf("EmbedName = %q, want empty", name)
		}
	})
}

func TestLocalName(t *testing.T) {
	t.Parallel()

	t.Run("takes the trailing identifier off a qualified name", func(t *testing.T) {
		t.Parallel()
		// Resolution rewrites a sibling reference into the qualified
		// form the store keys on, which a call expression cannot use.
		if got := node.LocalName("example.com/pkg.Store.Get"); got != "Get" {
			t.Fatalf("LocalName = %q, want Get", got)
		}
	})

	t.Run("returns an unqualified name unchanged", func(t *testing.T) {
		t.Parallel()
		// A name resolution could not resolve is left as written and
		// already reported; failing twice for one cause helps nobody.
		if got := node.LocalName("Get"); got != "Get" {
			t.Fatalf("LocalName = %q, want Get", got)
		}
	})

	t.Run("returns empty for the empty name", func(t *testing.T) {
		t.Parallel()
		if got := node.LocalName(""); got != "" {
			t.Fatalf("LocalName = %q, want empty", got)
		}
	})

	t.Run("returns empty for a name ending in a separator", func(t *testing.T) {
		t.Parallel()
		if got := node.LocalName("pkg.Store."); got != "" {
			t.Fatalf("LocalName = %q, want empty", got)
		}
	})
}

func TestIsExportedName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want bool
	}{
		{"Name", true},
		{"ID", true},
		{"name", false},
		{"_name", false},
		{"", false},
		{"Ünicode", false},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/"+map[bool]string{true: "exported", false: "not"}[tc.want], func(t *testing.T) {
			t.Parallel()
			if got := node.IsExportedName(tc.name); got != tc.want {
				t.Fatalf("IsExportedName(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
