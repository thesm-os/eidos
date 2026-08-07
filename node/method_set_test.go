// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/node"
)

// ifaceOf builds an interface declaring the named methods.
func ifaceOf(name string, methods ...string) *node.Interface {
	i := &node.Interface{Name: name, Package: "x"}
	for _, m := range methods {
		i.Methods = append(i.Methods, &node.Method{Name: m})
	}
	return i
}

// embed appends an embed of the named type to i.
func embed(i *node.Interface, pkg, name string) *node.Embed {
	e := &node.Embed{Type: namedRef(pkg, name), Owner: i}
	i.Embeds = append(i.Embeds, e)
	return e
}

// resolverFor returns an [node.InterfaceResolver] over the supplied
// interfaces, keyed by name. Anything absent reports not-found.
func resolverFor(ifaces ...*node.Interface) node.InterfaceResolver {
	byName := map[string]*node.Interface{}
	for _, i := range ifaces {
		byName[i.Name] = i
	}
	return func(t *node.TypeRef) (*node.Interface, bool) {
		i, ok := byName[t.Name]
		return i, ok
	}
}

// names projects a resolved set to its method names.
func names(r node.MethodSetResult) []string {
	out := make([]string, 0, len(r.Methods))
	for _, m := range r.Methods {
		out = append(out, m.Name)
	}
	return out
}

// TestMethodSet pins the transitive resolution a generated double
// depends on.
//
// Reading Interface.Methods alone reads what the source typed, not
// what the interface has. The failure is silent and lands in the
// generated file: a double missing an embedded method does not
// satisfy the interface it doubles, and the compiler reports that
// against the generated code rather than against the run.
func TestMethodSet(t *testing.T) {
	t.Parallel()

	t.Run("an interface with no embeds returns its own methods", func(t *testing.T) {
		t.Parallel()
		got := node.MethodSet(ifaceOf("Store", "Get", "Put"), nil)
		if !slices.Equal(names(got), []string{"Get", "Put"}) {
			t.Fatalf("Methods = %v, want [Get Put]", names(got))
		}
	})

	t.Run("an embedded interface contributes its methods", func(t *testing.T) {
		t.Parallel()
		reader := ifaceOf("Reader", "Read")
		store := ifaceOf("Store", "Put")
		embed(store, "x", "Reader")
		got := node.MethodSet(store, resolverFor(reader))
		if !slices.Equal(names(got), []string{"Put", "Read"}) {
			t.Fatalf("Methods = %v, want [Put Read]", names(got))
		}
	})

	t.Run("declared methods precede embedded ones", func(t *testing.T) {
		t.Parallel()
		// Generators derive field order from this, so a set that
		// reordered would change generated output without the source
		// changing.
		reader := ifaceOf("Reader", "Read")
		store := ifaceOf("Store", "Put")
		embed(store, "x", "Reader")
		if got := names(node.MethodSet(store, resolverFor(reader))); got[0] != "Put" {
			t.Fatalf("Methods = %v, want the declared method first", got)
		}
	})

	t.Run("embeds contribute in declaration order", func(t *testing.T) {
		t.Parallel()
		a, b := ifaceOf("A", "AM"), ifaceOf("B", "BM")
		store := ifaceOf("Store")
		embed(store, "x", "A")
		embed(store, "x", "B")
		got := names(node.MethodSet(store, resolverFor(a, b)))
		if !slices.Equal(got, []string{"AM", "BM"}) {
			t.Fatalf("Methods = %v, want [AM BM]", got)
		}
	})

	t.Run("resolves transitively through a chain of embeds", func(t *testing.T) {
		t.Parallel()
		base := ifaceOf("Base", "Close")
		mid := ifaceOf("Mid", "Read")
		embed(mid, "x", "Base")
		top := ifaceOf("Top", "Write")
		embed(top, "x", "Mid")
		got := names(node.MethodSet(top, resolverFor(base, mid)))
		if !slices.Equal(got, []string{"Write", "Read", "Close"}) {
			t.Fatalf("Methods = %v, want depth-first [Write Read Close]", got)
		}
	})

	t.Run("a name already in the set is not repeated", func(t *testing.T) {
		t.Parallel()
		// Go admits overlapping embedded sets where signatures agree,
		// so a diamond must not yield the shared method twice.
		base := ifaceOf("Base", "Close")
		left, right := ifaceOf("Left"), ifaceOf("Right")
		embed(left, "x", "Base")
		embed(right, "x", "Base")
		top := ifaceOf("Top")
		embed(top, "x", "Left")
		embed(top, "x", "Right")
		got := names(node.MethodSet(top, resolverFor(base, left, right)))
		if !slices.Equal(got, []string{"Close"}) {
			t.Fatalf("Methods = %v, want [Close] once", got)
		}
	})

	t.Run("a declared method wins over an embedded one of the same name", func(t *testing.T) {
		t.Parallel()
		base := ifaceOf("Base", "Read")
		store := ifaceOf("Store", "Read")
		embed(store, "x", "Base")
		got := node.MethodSet(store, resolverFor(base))
		if len(got.Methods) != 1 || got.Methods[0] != store.Methods[0] {
			t.Fatalf("Methods = %v, want the declared Read", names(got))
		}
	})

	t.Run("a nil interface resolves to an empty set", func(t *testing.T) {
		t.Parallel()
		if got := node.MethodSet(nil, nil); len(got.Methods) != 0 || !got.OK() {
			t.Fatalf("MethodSet(nil) = %+v, want empty and OK", got)
		}
	})
}

// TestMethodSet_Issues pins the four ways an embed contributes
// nothing. Each is a different problem for the caller: an
// unresolved embed is usually a narrow run, the rest are defects no
// wider run fixes.
func TestMethodSet_Issues(t *testing.T) {
	t.Parallel()

	t.Run("an embed this run did not load reports Unresolved", func(t *testing.T) {
		t.Parallel()
		store := ifaceOf("Store", "Put")
		embed(store, "other", "Absent")
		got := node.MethodSet(store, resolverFor())
		if len(got.Issues) != 1 || got.Issues[0].Reason != node.ReasonUnresolved {
			t.Fatalf("Issues = %+v, want one ReasonUnresolved", got.Issues)
		}
	})

	t.Run("an embed resolving to a non-interface reports NonInterface", func(t *testing.T) {
		t.Parallel()
		// found=true with a nil interface is how the store says "this
		// name exists and is a struct".
		store := ifaceOf("Store", "Put")
		embed(store, "x", "Config")
		resolve := func(*node.TypeRef) (*node.Interface, bool) { return nil, true }
		got := node.MethodSet(store, resolve)
		if len(got.Issues) != 1 || got.Issues[0].Reason != node.ReasonNonInterface {
			t.Fatalf("Issues = %+v, want one ReasonNonInterface", got.Issues)
		}
	})

	t.Run("an embed cycle reports Cyclic and terminates", func(t *testing.T) {
		t.Parallel()
		a := ifaceOf("A", "AM")
		b := ifaceOf("B", "BM")
		embed(a, "x", "B")
		embed(b, "x", "A")
		got := node.MethodSet(a, resolverFor(a, b))
		if len(got.Issues) != 1 || got.Issues[0].Reason != node.ReasonCyclic {
			t.Fatalf("Issues = %+v, want one ReasonCyclic", got.Issues)
		}
		if !slices.Equal(names(got), []string{"AM", "BM"}) {
			t.Fatalf("Methods = %v, want both reached before the cycle broke", names(got))
		}
	})

	t.Run("a self-embed reports Cyclic", func(t *testing.T) {
		t.Parallel()
		a := ifaceOf("A", "AM")
		embed(a, "x", "A")
		got := node.MethodSet(a, resolverFor(a))
		if len(got.Issues) != 1 || got.Issues[0].Reason != node.ReasonCyclic {
			t.Fatalf("Issues = %+v, want one ReasonCyclic", got.Issues)
		}
	})

	t.Run("a parameterised embed is refused", func(t *testing.T) {
		t.Parallel()
		// Instantiating it needs the type arguments substituted
		// through the embedded signatures, which the model does not
		// carry; reporting the uninstantiated names would render
		// references to parameters the embedding interface does not
		// declare.
		store := ifaceOf("Store", "Put")
		e := embed(store, "x", "Base")
		e.Type.TypeArgs = []*node.TypeRef{namedRef("", "string")}
		got := node.MethodSet(store, resolverFor(ifaceOf("Base", "Read")))
		if len(got.Issues) != 1 || got.Issues[0].Reason != node.ReasonGeneric {
			t.Fatalf("Issues = %+v, want one ReasonGeneric", got.Issues)
		}
	})

	t.Run("a nil resolver reports every embed as unresolved", func(t *testing.T) {
		t.Parallel()
		store := ifaceOf("Store", "Put")
		embed(store, "x", "A")
		embed(store, "x", "B")
		got := node.MethodSet(store, nil)
		if len(got.Issues) != 2 {
			t.Fatalf("Issues = %+v, want two", got.Issues)
		}
	})

	t.Run("the issue carries the embed it concerns", func(t *testing.T) {
		t.Parallel()
		// A reason without the embed leaves a caller unable to say
		// which reference failed.
		store := ifaceOf("Store", "Put")
		e := embed(store, "other", "Absent")
		got := node.MethodSet(store, resolverFor())
		if got.Issues[0].Embed != e {
			t.Fatalf("Issue.Embed = %+v, want the offending embed", got.Issues[0].Embed)
		}
	})

	t.Run("OK is false when any embed failed", func(t *testing.T) {
		t.Parallel()
		store := ifaceOf("Store", "Put")
		embed(store, "other", "Absent")
		if node.MethodSet(store, resolverFor()).OK() {
			t.Fatalf("OK() = true with an unresolved embed")
		}
	})

	t.Run("OK is true when every embed resolved", func(t *testing.T) {
		t.Parallel()
		reader := ifaceOf("Reader", "Read")
		store := ifaceOf("Store", "Put")
		embed(store, "x", "Reader")
		if !node.MethodSet(store, resolverFor(reader)).OK() {
			t.Fatalf("OK() = false with every embed resolved")
		}
	})
}

func TestMethodSetResult_ByName(t *testing.T) {
	t.Parallel()

	t.Run("finds a declared method", func(t *testing.T) {
		t.Parallel()
		got := node.MethodSet(ifaceOf("Store", "Get"), nil)
		if got.ByName("Get") == nil {
			t.Fatalf("ByName(Get) = nil")
		}
	})

	t.Run("finds a method contributed by an embed", func(t *testing.T) {
		t.Parallel()
		reader := ifaceOf("Reader", "Read")
		store := ifaceOf("Store", "Put")
		embed(store, "x", "Reader")
		if node.MethodSet(store, resolverFor(reader)).ByName("Read") == nil {
			t.Fatalf("ByName(Read) = nil for an embedded method")
		}
	})

	t.Run("returns nil for an absent name", func(t *testing.T) {
		t.Parallel()
		if node.MethodSet(ifaceOf("Store", "Get"), nil).ByName("Missing") != nil {
			t.Fatalf("ByName(Missing) should be nil")
		}
	})
}

func TestMethodSetReason_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		r    node.MethodSetReason
		want string
	}{
		{node.ReasonUnresolved, "not loaded by this run"},
		{node.ReasonNonInterface, "resolves to a declaration that is not an interface"},
		{node.ReasonCyclic, "embed cycle"},
		{node.ReasonGeneric, "parameterised embed"},
		{node.MethodSetReason(99), "method_set_reason(?)"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := tc.r.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsConstraint(t *testing.T) {
	t.Parallel()

	t.Run("an interface declaring methods is not a constraint", func(t *testing.T) {
		t.Parallel()
		if node.IsConstraint(ifaceOf("Store", "Get")) {
			t.Fatalf("a method-set interface must not read as a constraint")
		}
	})

	t.Run("an interface embedding builtin terms is a constraint", func(t *testing.T) {
		t.Parallel()
		// A constraint has no method set to double; a generator
		// treating one as an interface emits a type asserting nothing.
		c := &node.Interface{Name: "Numeric", Package: "x"}
		c.Embeds = []*node.Embed{{Type: namedRef("", "int")}, {Type: namedRef("", "int64")}}
		if !node.IsConstraint(c) {
			t.Fatalf("a type-set interface must read as a constraint")
		}
	})

	t.Run("an interface embedding another interface is not a constraint", func(t *testing.T) {
		t.Parallel()
		i := &node.Interface{Name: "ReadCloser", Package: "x"}
		i.Embeds = []*node.Embed{{Type: namedRef("io", "Reader")}}
		if node.IsConstraint(i) {
			t.Fatalf("an embedding interface must not read as a constraint")
		}
	})

	t.Run("a nil interface is not a constraint", func(t *testing.T) {
		t.Parallel()
		if node.IsConstraint(nil) {
			t.Fatalf("nil must not read as a constraint")
		}
	})
}

// TestMethodSet_MalformedGraph pins the walk against the shapes a
// hand-built graph carries.
//
// Fixtures and partially-populated test graphs hold nil entries,
// and a panic here would surface as a framework fault reported
// against the caller's plugin rather than against the graph that
// caused it.
func TestMethodSet_MalformedGraph(t *testing.T) {
	t.Parallel()

	t.Run("skips a nil method entry", func(t *testing.T) {
		t.Parallel()
		i := &node.Interface{Name: "Store", Package: "x"}
		i.Methods = []*node.Method{{Name: "Get"}, nil, {Name: "Put"}}
		if got := names(node.MethodSet(i, nil)); !slices.Equal(got, []string{"Get", "Put"}) {
			t.Fatalf("Methods = %v, want [Get Put]", got)
		}
	})

	t.Run("skips a method carrying no name", func(t *testing.T) {
		t.Parallel()
		// An unnamed method cannot be called or asserted about, and
		// admitting one would let it match a caller's empty lookup.
		i := &node.Interface{Name: "Store", Package: "x"}
		i.Methods = []*node.Method{{Name: ""}, {Name: "Get"}}
		if got := names(node.MethodSet(i, nil)); !slices.Equal(got, []string{"Get"}) {
			t.Fatalf("Methods = %v, want [Get]", got)
		}
	})

	t.Run("skips a nil embed without reporting an issue", func(t *testing.T) {
		t.Parallel()
		i := ifaceOf("Store", "Put")
		i.Embeds = []*node.Embed{nil}
		got := node.MethodSet(i, nil)
		if !got.OK() {
			t.Fatalf("Issues = %+v, want none for a nil embed", got.Issues)
		}
	})

	t.Run("skips an embed carrying no type without reporting an issue", func(t *testing.T) {
		t.Parallel()
		// Distinct from an unresolved embed: there is no reference to
		// report, so naming it as a resolution failure would send a
		// reader looking for a package that was never named.
		i := ifaceOf("Store", "Put")
		i.Embeds = []*node.Embed{{}}
		got := node.MethodSet(i, nil)
		if !got.OK() {
			t.Fatalf("Issues = %+v, want none for a typeless embed", got.Issues)
		}
	})
}
