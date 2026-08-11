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

	t.Run("an interface embedding a composite term is a constraint", func(t *testing.T) {
		t.Parallel()
		// A constraint has no method set to double; a generator
		// treating one as an interface emits a type asserting nothing.
		// A slice can never be an interface, so it is evidence whatever
		// language produced the graph.
		c := &node.Interface{Name: "Bytes", Package: "x"}
		c.Embeds = []*node.Embed{{Type: sliceRef(namedRef("", "byte"))}}
		if !node.IsConstraint(c) {
			t.Fatalf("a composite type-set term must read as a constraint")
		}
	})

	t.Run("a named term is not structural evidence either way", func(t *testing.T) {
		t.Parallel()
		// `interface{ int | int64 }` is a constraint and
		// `interface{ error }` is not, and the two are the same shape
		// here — a Named ref with no package. Answering from the shape
		// means picking one to be wrong about, and this used to pick
		// the second: every `interface{ error }` read as a constraint.
		//
		// A Go pipeline asks `lang/golang.IsConstraintInterface`, which
		// reads the stamp the frontend sets from type information. This
		// is the fallback for a graph no Go frontend produced.
		numeric := &node.Interface{Name: "Numeric", Package: "x"}
		numeric.Embeds = []*node.Embed{{Type: namedRef("", "int")}, {Type: namedRef("", "int64")}}
		if node.IsConstraint(numeric) {
			t.Fatalf("a named term must not be read as structural evidence")
		}

		errIface := &node.Interface{Name: "Wrapped", Package: "x"}
		errIface.Embeds = []*node.Embed{{Type: namedRef("", "error")}}
		if node.IsConstraint(errIface) {
			t.Fatalf("interface{ error } is an ordinary interface, not a constraint")
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

func TestMethodSet_Attribution(t *testing.T) {
	t.Parallel()

	t.Run("a declared method reports no contributing embed", func(t *testing.T) {
		t.Parallel()
		got := node.MethodSet(ifaceOf("Store", "Get"), nil)
		if from := got.From("Get"); from != nil {
			t.Fatalf("From(Get) = %v, want nil for a method the interface declares", from)
		}
	})

	t.Run("an embedded method reports the embed it arrived through", func(t *testing.T) {
		t.Parallel()
		// The attribution a generated field spends on saying where it
		// came from: a double that grows because an embedded interface
		// gained a method otherwise explains nothing.
		reader := ifaceOf("Reader", "Read")
		store := ifaceOf("Store", "Put")
		embed(store, "x", "Reader")

		got := node.MethodSet(store, resolverFor(reader))
		from := got.From("Read")
		if from == nil {
			t.Fatal("From(Read) = nil, want the embed that contributed it")
		}
		if from.Type == nil || from.Type.Name != "Reader" {
			t.Fatalf("From(Read) named %v, want the Reader embed", from.Type)
		}
	})

	t.Run("a transitively embedded method reports the top-level embed", func(t *testing.T) {
		t.Parallel()
		// A embeds B embeds C: C's method is attributed to A's embed of
		// B, because that is the one the interface in front of the
		// caller actually writes down. Naming C's embed would name
		// something the caller cannot see.
		inner := ifaceOf("Closer", "Close")
		mid := ifaceOf("ReadCloser", "Read")
		embed(mid, "x", "Closer")
		outer := ifaceOf("Store", "Put")
		embed(outer, "x", "ReadCloser")

		got := node.MethodSet(outer, resolverFor(inner, mid))
		from := got.From("Close")
		if from == nil || from.Type == nil {
			t.Fatalf("From(Close) = %v, want the top-level embed", from)
		}
		if from.Type.Name != "ReadCloser" {
			t.Fatalf("From(Close) named %q, want ReadCloser (the top-level embed)", from.Type.Name)
		}
	})

	t.Run("Entries lines up with Methods", func(t *testing.T) {
		t.Parallel()
		// The two are published side by side, so a caller indexing one
		// against the other must not be reading different sets.
		reader := ifaceOf("Reader", "Read")
		store := ifaceOf("Store", "Put")
		embed(store, "x", "Reader")

		got := node.MethodSet(store, resolverFor(reader))
		if len(got.Entries) != len(got.Methods) {
			t.Fatalf("Entries=%d Methods=%d, want equal", len(got.Entries), len(got.Methods))
		}
		for i, m := range got.Methods {
			if got.Entries[i].Method != m {
				t.Fatalf("Entries[%d].Method = %v, want Methods[%d] = %v",
					i, got.Entries[i].Method, i, m)
			}
		}
	})

	t.Run("an unresolved method is absent rather than unattributed", func(t *testing.T) {
		t.Parallel()
		// From answers nil for both "declared here" and "not in the set",
		// so the distinction is ByName's to make.
		got := node.MethodSet(ifaceOf("Store", "Get"), nil)
		if got.From("Missing") != nil || got.ByName("Missing") != nil {
			t.Fatal("a method the set does not have was reported")
		}
	})
}

// A term constraining an interface's type set is not an embed that
// failed to contribute methods — it is not an embed. Reporting one as
// an Issue sends a generator's diagnostic after a package the author
// never needed to load.
func TestMethodSet_TypeSetTerms(t *testing.T) {
	t.Parallel()

	// missing resolves nothing, standing in for a run narrower than
	// the graph it walks.
	missing := func(*node.TypeRef) (*node.Interface, bool) { return nil, false }

	t.Run("skips a composite term rather than reporting it", func(t *testing.T) {
		t.Parallel()
		// `[]byte` reached the resolver, missed, and came back as "not
		// loaded by this run" — under a name EmbedName renders empty,
		// so the diagnostic named nothing at all.
		i := &node.Interface{Name: "Termed", Package: "x", Embeds: []*node.Embed{
			{Type: sliceRef(namedRef("", "byte"))},
		}}
		if got := node.MethodSet(i, missing); len(got.Issues) != 0 {
			t.Fatalf("Issues = %+v, want none for a type-set term", got.Issues)
		}
	})

	t.Run("skips every composite variant", func(t *testing.T) {
		t.Parallel()
		// None of these can name an interface in any language this
		// model describes, so none of them is a resolver's business.
		for _, ref := range []*node.TypeRef{
			sliceRef(namedRef("", "byte")),
			{TypeKind: node.TypeRefMap, MapKey: namedRef("", "string"), MapValue: namedRef("", "int")},
			{TypeKind: node.TypeRefFunc},
			{TypeKind: node.TypeRefArray, Elem: namedRef("", "byte")},
			{TypeKind: node.TypeRefPointer, Elem: namedRef("x", "T")},
			{TypeKind: node.TypeRefAnonStruct},
			{TypeKind: node.TypeRefTypeParam, Name: "T"},
		} {
			i := &node.Interface{Name: "Termed", Package: "x", Embeds: []*node.Embed{{Type: ref}}}
			if got := node.MethodSet(i, missing); len(got.Issues) != 0 {
				t.Fatalf("%v reported %+v", ref.TypeKind, got.Issues)
			}
		}
	})

	t.Run("still reports a named embed the run did not load", func(t *testing.T) {
		t.Parallel()
		// The half the model cannot answer: an unloaded `MyReader` and
		// an unloaded type-set term are the same shape here, and
		// silence would hide a genuinely incomplete method set.
		i := &node.Interface{Name: "Composed", Package: "x", Embeds: []*node.Embed{
			{Type: namedRef("io", "Reader")},
		}}
		got := node.MethodSet(i, missing)
		if len(got.Issues) != 1 || got.Issues[0].Reason != node.ReasonUnresolved {
			t.Fatalf("Issues = %+v, want one unresolved", got.Issues)
		}
	})

	t.Run("does not consult the resolver for a composite term", func(t *testing.T) {
		t.Parallel()
		// The ordering is the fix: asking the resolver first is what
		// turned a shape question into a miss the switch could not tell
		// from a failed embed.
		var asked []node.TypeRefKind
		spy := func(t *node.TypeRef) (*node.Interface, bool) {
			asked = append(asked, t.TypeKind)
			return nil, false
		}
		i := &node.Interface{Name: "Termed", Package: "x", Embeds: []*node.Embed{
			{Type: sliceRef(namedRef("", "byte"))},
			{Type: namedRef("io", "Reader")},
		}}
		node.MethodSet(i, spy)
		if len(asked) != 1 || asked[0] != node.TypeRefNamed {
			t.Fatalf("resolver saw %v, want only the named embed", asked)
		}
	})
}

// TestMethodSet_ResolvedEmbed covers the projection an embed carries
// for a declaration the run did not load.
//
// The shape that motivated it — an interface embedding io.Closer
// beside a domain method — reported ReasonUnresolved and took the
// whole declaration out of scope, because a projection missing a
// method describes a type that does not satisfy what it claims to.
func TestMethodSet_ResolvedEmbed(t *testing.T) {
	t.Parallel()

	// foreign stands in for io.Closer: a declaration outside the run,
	// projected from the type-checker rather than loaded.
	foreign := func() *node.Interface {
		i := &node.Interface{Name: "Closer", Package: "io"}
		i.Methods = []*node.Method{{Name: "Close", Owner: i}}
		return i
	}

	t.Run("an unloaded embed completes from its projection", func(t *testing.T) {
		t.Parallel()
		host := ifaceOf("Stream", "Read")
		e := embed(host, "io", "Closer")
		e.Resolved = foreign()

		got := node.MethodSet(host, resolverFor())
		if len(got.Issues) != 0 {
			t.Fatalf("Issues = %v, want none", got.Issues)
		}
		if len(got.Methods) != 2 {
			t.Fatalf("Methods = %d, want Read and Close", len(got.Methods))
		}
	})

	t.Run("the projected method is attributed to the embed", func(t *testing.T) {
		t.Parallel()
		host := ifaceOf("Stream", "Read")
		e := embed(host, "io", "Closer")
		e.Resolved = foreign()

		got := node.MethodSet(host, resolverFor())
		for _, entry := range got.Entries {
			if entry.Method.Name == "Close" && entry.From != e {
				t.Errorf("Close attributed to %v, want the embed that carried it", entry.From)
			}
		}
	})

	t.Run("a loaded declaration still wins", func(t *testing.T) {
		t.Parallel()
		// The projection carries no directives, docs or positions, so a
		// run that loaded the real declaration must read that instead.
		loaded := ifaceOf("Closer", "Close")
		host := ifaceOf("Stream", "Read")
		e := embed(host, "io", "Closer")
		e.Resolved = foreign()

		got := node.MethodSet(host, resolverFor(loaded))
		for _, m := range got.Methods {
			if m.Name != "Close" {
				continue
			}
			// Identity, not equality: the two carry the same name and
			// only the loaded one carries what the projection cannot.
			if m != loaded.Methods[0] {
				t.Error("the projection was preferred over a loaded declaration")
			}
		}
	})

	t.Run("an embed with neither still reports unresolved", func(t *testing.T) {
		t.Parallel()
		// The existing diagnostic must stay reachable, or a consumer's
		// handling of it becomes dead code.
		host := ifaceOf("Stream", "Read")
		embed(host, "io", "Closer")

		got := node.MethodSet(host, resolverFor())
		if len(got.Issues) != 1 || got.Issues[0].Reason != node.ReasonUnresolved {
			t.Fatalf("Issues = %v, want one ReasonUnresolved", got.Issues)
		}
	})

	t.Run("the projected method keeps its declaring package", func(t *testing.T) {
		t.Parallel()
		// What the whole shape is for: a method's owner is how it
		// answers which package it came from, and the answer must not
		// depend on whether the run happened to load it.
		host := ifaceOf("Stream", "Read")
		e := embed(host, "io", "Closer")
		e.Resolved = foreign()

		got := node.MethodSet(host, resolverFor())
		for _, m := range got.Methods {
			if m.Name != "Close" {
				continue
			}
			owner, ok := m.Owner.(*node.Interface)
			if !ok || owner.Package != "io" {
				t.Errorf("Close owner = %v, want the io.Closer projection", m.Owner)
			}
		}
	})
}
