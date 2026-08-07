// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"slices"
	"strconv"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// field builds a named field of the given type.
func field(name string, t *node.TypeRef) *node.Field {
	return &node.Field{Name: name, Type: t}
}

// embed builds an embed of the named type in the given package.
func embed(pkg, name string, byPointer bool) *node.Embed {
	t := namedTypeRef(pkg, name)
	if byPointer {
		t = &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: t}
	}
	return &node.Embed{Type: t}
}

// names projects a promoted-field list's identifiers.
func names(fields []golang.PromotedField) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = f.Field.Name
	}
	return out
}

func TestEmbedIdent(t *testing.T) {
	t.Parallel()

	t.Run("names the embedded type", func(t *testing.T) {
		t.Parallel()
		name, byPtr := golang.EmbedIdent(embed("example.com/x", "Base", false))
		if name != "Base" || byPtr {
			t.Fatalf("EmbedIdent = %q, %v", name, byPtr)
		}
	})

	t.Run("a pointer embed carries its name on the pointee", func(t *testing.T) {
		t.Parallel()
		// Reading the reference's own name yields the empty string and
		// the whole field is silently dropped — the bug this prevents.
		name, byPtr := golang.EmbedIdent(embed("example.com/x", "Base", true))
		if name != "Base" || !byPtr {
			t.Fatalf("EmbedIdent = %q, %v; want Base, true", name, byPtr)
		}
	})

	t.Run("a generic embed drops its arguments", func(t *testing.T) {
		t.Parallel()
		// `Base[T]` embeds as the field `Base`.
		e := embed("example.com/x", "Base", false)
		e.Type.TypeArgs = []*node.TypeRef{builtinRef("int")}
		if name, _ := golang.EmbedIdent(e); name != "Base" {
			t.Fatalf("EmbedIdent = %q", name)
		}
	})

	t.Run("an embed with no type names nothing", func(t *testing.T) {
		t.Parallel()
		if name, _ := golang.EmbedIdent(&node.Embed{}); name != "" {
			t.Fatalf("EmbedIdent = %q", name)
		}
		if name, _ := golang.EmbedIdent(nil); name != "" {
			t.Fatalf("EmbedIdent(nil) = %q", name)
		}
	})

	t.Run("the target strips the pointer", func(t *testing.T) {
		t.Parallel()
		// The pointer is a fact about the embedding, not about the
		// type a caller resolves.
		got := golang.EmbedTarget(embed("example.com/x", "Base", true))
		if got == nil || got.Name != "Base" || got.IsPointer() {
			t.Fatalf("EmbedTarget = %v", got)
		}
	})
}

func TestFieldSet(t *testing.T) {
	t.Parallel()

	base := &node.Struct{
		Name: "Base", Package: "x",
		Fields: []*node.Field{field("ID", builtinRef("string")), field("created", builtinRef("int"))},
	}
	r := mapResolver{"x.Base": base}

	t.Run("promotes an embedded type's fields", func(t *testing.T) {
		t.Parallel()
		// A generator reading s.Fields reads what the source typed, not
		// what the struct has.
		s := &node.Struct{
			Name: "User", Package: "x",
			Fields: []*node.Field{field("Name", builtinRef("string"))},
			Embeds: []*node.Embed{embed("x", "Base", false)},
		}
		got, complete := golang.FieldSet(s, r)
		if !complete {
			t.Fatalf("FieldSet reported incomplete")
		}
		if !slices.Equal(names(got), []string{"Name", "Base", "ID", "created"}) {
			t.Fatalf("FieldSet = %v", names(got))
		}
	})

	t.Run("a declared field shadows a promoted one", func(t *testing.T) {
		t.Parallel()
		// Depth zero beats everything.
		s := &node.Struct{
			Name: "User", Package: "x",
			Fields: []*node.Field{field("ID", builtinRef("int"))},
			Embeds: []*node.Embed{embed("x", "Base", false)},
		}
		got, _ := golang.FieldSet(s, r)
		for _, f := range got {
			if f.Field.Name == "ID" && f.Depth != 0 {
				t.Fatalf("ID promoted at depth %d, want the declared one", f.Depth)
			}
		}
	})

	t.Run("two promotions at equal depth cancel", func(t *testing.T) {
		t.Parallel()
		// Go makes an ambiguous selector an error rather than a choice,
		// so neither is reachable and neither appears.
		other := &node.Struct{
			Name: "Other", Package: "x",
			Fields: []*node.Field{field("ID", builtinRef("int"))},
		}
		s := &node.Struct{
			Name:   "User",
			Embeds: []*node.Embed{embed("x", "Base", false), embed("x", "Other", false)},
		}
		got, _ := golang.FieldSet(s, mapResolver{"x.Base": base, "x.Other": other})
		if slices.Contains(names(got), "ID") {
			t.Fatalf("FieldSet = %v, want no ambiguous ID", names(got))
		}
	})

	t.Run("a shallower promotion wins over a deeper one", func(t *testing.T) {
		t.Parallel()
		deep := &node.Struct{
			Name: "Deep", Package: "x",
			Fields: []*node.Field{field("ID", builtinRef("bool"))},
		}
		mid := &node.Struct{
			Name: "Mid", Package: "x",
			Embeds: []*node.Embed{embed("x", "Deep", false)},
		}
		s := &node.Struct{
			Name:   "User",
			Embeds: []*node.Embed{embed("x", "Base", false), embed("x", "Mid", false)},
		}
		got, _ := golang.FieldSet(s, mapResolver{"x.Base": base, "x.Mid": mid, "x.Deep": deep})
		for _, f := range got {
			if f.Field.Name == "ID" && f.Depth != 1 {
				t.Fatalf("ID resolved at depth %d, want the shallower", f.Depth)
			}
		}
	})

	t.Run("records the path a literal has to write", func(t *testing.T) {
		t.Parallel()
		// Promotion makes `v.ID` legal, but a composite literal setting
		// the same field has to write `v.Base.ID`.
		s := &node.Struct{Name: "User", Embeds: []*node.Embed{embed("x", "Base", false)}}
		got, _ := golang.FieldSet(s, r)
		for _, f := range got {
			if f.Field.Name == "ID" && f.Selector() != "Base.ID" {
				t.Fatalf("Selector = %q, want Base.ID", f.Selector())
			}
		}
	})

	t.Run("records that a path crossed a pointer", func(t *testing.T) {
		t.Parallel()
		// An embedded pointer is nil until something allocates it, so a
		// setter writing through one panics unless it allocates first.
		s := &node.Struct{Name: "User", Embeds: []*node.Embed{embed("x", "Base", true)}}
		got, _ := golang.FieldSet(s, r)
		for _, f := range got {
			if f.Field.Name == "ID" && !f.ThroughPointer {
				t.Fatalf("ID must record that it is reached through a pointer")
			}
		}
	})

	t.Run("an unreachable embed makes the answer incomplete", func(t *testing.T) {
		t.Parallel()
		// The answer is smaller rather than wrong, and the caller must
		// not treat it as complete.
		s := &node.Struct{Name: "User", Embeds: []*node.Embed{embed("x", "Missing", false)}}
		got, complete := golang.FieldSet(s, mapResolver{})
		if complete {
			t.Fatalf("FieldSet reported complete with an unresolved embed")
		}
		if !slices.Equal(names(got), []string{"Missing"}) {
			t.Fatalf("FieldSet = %v, want the embed itself", names(got))
		}
	})

	t.Run("no resolver sees only the first level", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{
			Name:   "User",
			Fields: []*node.Field{field("Name", builtinRef("string"))},
			Embeds: []*node.Embed{embed("x", "Base", false)},
		}
		got, complete := golang.FieldSet(s, nil)
		if complete {
			t.Fatalf("FieldSet without a resolver must report incomplete")
		}
		if !slices.Equal(names(got), []string{"Name", "Base"}) {
			t.Fatalf("FieldSet = %v", names(got))
		}
	})

	t.Run("a cycle terminates", func(t *testing.T) {
		t.Parallel()
		// Illegal in Go by value and reachable through a pointer embed
		// or a malformed graph.
		loop := &node.Struct{Name: "Loop", Package: "x"}
		loop.Embeds = []*node.Embed{embed("x", "Loop", true)}
		got, _ := golang.FieldSet(loop, mapResolver{"x.Loop": loop})
		if len(got) == 0 {
			t.Fatalf("FieldSet on a cycle yielded nothing")
		}
	})

	t.Run("nil is an empty complete set", func(t *testing.T) {
		t.Parallel()
		got, complete := golang.FieldSet(nil, r)
		if len(got) != 0 || !complete {
			t.Fatalf("FieldSet(nil) = %v, %v", got, complete)
		}
	})
}

func TestPromotedAndExportedFields(t *testing.T) {
	t.Parallel()

	base := &node.Struct{
		Name: "Base", Package: "x",
		Fields: []*node.Field{field("ID", builtinRef("string")), field("created", builtinRef("int"))},
	}
	r := mapResolver{"x.Base": base}
	s := &node.Struct{
		Name: "User", Package: "x",
		Fields: []*node.Field{field("Name", builtinRef("string")), field("secret", builtinRef("string"))},
		Embeds: []*node.Embed{embed("x", "Base", false)},
	}

	t.Run("promoted excludes what the struct declared", func(t *testing.T) {
		t.Parallel()
		// A builder offering a setter per declared field and one whole
		// setter per embedded value needs to tell them apart.
		got, _ := golang.PromotedFields(s, r)
		if slices.Contains(names(got), "Name") {
			t.Fatalf("PromotedFields = %v, want no declared field", names(got))
		}
		if !slices.Contains(names(got), "ID") {
			t.Fatalf("PromotedFields = %v, want the promoted ID", names(got))
		}
	})

	t.Run("exported excludes what another package cannot name", func(t *testing.T) {
		t.Parallel()
		// A generator routed elsewhere that emitted a setter for an
		// unexported field produces a file that does not compile.
		got, _ := golang.ExportedFieldSet(s, r)
		for _, n := range names(got) {
			if n == "secret" || n == "created" {
				t.Fatalf("ExportedFieldSet = %v, want no unexported field", names(got))
			}
		}
	})
}

func TestPromotedMethods(t *testing.T) {
	t.Parallel()

	reader := &node.Interface{
		Name: "Reader", Package: "io",
		Methods: []*node.Method{{Name: "Read"}},
	}
	r := mapResolver{"io.Reader": reader}

	t.Run("an embedded interface contributes its methods", func(t *testing.T) {
		t.Parallel()
		// A struct embedding io.Reader has Read and declares nothing.
		s := &node.Struct{Name: "Wrapper", Embeds: []*node.Embed{embed("io", "Reader", false)}}
		got, complete := golang.PromotedMethods(s, r)
		if !complete || len(got) != 1 || got[0].Method.Name != "Read" {
			t.Fatalf("PromotedMethods = %+v, %v", got, complete)
		}
		if got[0].Through != "Reader" {
			t.Fatalf("Through = %q, want Reader", got[0].Through)
		}
	})

	t.Run("a declared method shadows the promoted one", func(t *testing.T) {
		t.Parallel()
		// Go resolves the selector to the shallower declaration.
		s := &node.Struct{
			Name:    "Wrapper",
			Methods: []*node.Method{{Name: "Read"}},
			Embeds:  []*node.Embed{embed("io", "Reader", false)},
		}
		got, _ := golang.PromotedMethods(s, r)
		if len(got) != 0 {
			t.Fatalf("PromotedMethods = %+v, want none", got)
		}
	})

	t.Run("records whether the embed was by pointer", func(t *testing.T) {
		t.Parallel()
		// Embedding *T promotes both receiver forms onto both; T
		// promotes only the value form onto the value.
		s := &node.Struct{Name: "Wrapper", Embeds: []*node.Embed{embed("io", "Reader", true)}}
		got, _ := golang.PromotedMethods(s, r)
		if len(got) != 1 || !got[0].ThroughPointer {
			t.Fatalf("PromotedMethods = %+v, want the pointer marker", got)
		}
	})

	t.Run("an unreachable embed makes the answer incomplete", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "Wrapper", Embeds: []*node.Embed{embed("io", "Missing", false)}}
		if _, complete := golang.PromotedMethods(s, mapResolver{}); complete {
			t.Fatalf("PromotedMethods reported complete with an unresolved embed")
		}
	})

	t.Run("nil is an empty complete set", func(t *testing.T) {
		t.Parallel()
		got, complete := golang.PromotedMethods(nil, r)
		if len(got) != 0 || !complete {
			t.Fatalf("PromotedMethods(nil) = %+v, %v", got, complete)
		}
	})
}

func TestEmbedsType(t *testing.T) {
	t.Parallel()

	base := &node.Struct{Name: "Base", Package: "x"}
	mid := &node.Struct{Name: "Mid", Package: "x", Embeds: []*node.Embed{embed("x", "Base", false)}}
	r := mapResolver{"x.Base": base, "x.Mid": mid}

	t.Run("finds a direct embed", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "User", Embeds: []*node.Embed{embed("x", "Base", false)}}
		if !golang.EmbedsType(s, "x.Base", nil) {
			t.Fatalf("EmbedsType = false for a direct embed")
		}
	})

	t.Run("finds a transitive embed through the resolver", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "User", Embeds: []*node.Embed{embed("x", "Mid", false)}}
		if !golang.EmbedsType(s, "x.Base", r) {
			t.Fatalf("EmbedsType = false for a transitive embed")
		}
	})

	t.Run("without a resolver it sees only the first level", func(t *testing.T) {
		t.Parallel()
		// The honest partial answer.
		s := &node.Struct{Name: "User", Embeds: []*node.Embed{embed("x", "Mid", false)}}
		if golang.EmbedsType(s, "x.Base", nil) {
			t.Fatalf("EmbedsType claimed a transitive embed with no resolver")
		}
	})

	t.Run("a struct embedding nothing embeds nothing", func(t *testing.T) {
		t.Parallel()
		if golang.EmbedsType(&node.Struct{Name: "User"}, "x.Base", r) {
			t.Fatalf("EmbedsType = true for a struct with no embeds")
		}
		if golang.EmbedsType(nil, "x.Base", r) {
			t.Fatalf("EmbedsType(nil) = true")
		}
	})
}

func TestEmbedEdges(t *testing.T) {
	t.Parallel()

	t.Run("a pointer embed with no pointee names nothing", func(t *testing.T) {
		t.Parallel()
		e := &node.Embed{Type: &node.TypeRef{TypeKind: node.TypeRefPointer}}
		if name, _ := golang.EmbedIdent(e); name != "" {
			t.Fatalf("EmbedIdent = %q", name)
		}
		if got := golang.EmbedTarget(nil); got != nil {
			t.Fatalf("EmbedTarget(nil) = %v", got)
		}
	})

	t.Run("a declared field's selector is its own name", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "User", Fields: []*node.Field{field("ID", builtinRef("int"))}}
		got, _ := golang.FieldSet(s, nil)
		if got[0].Selector() != "ID" {
			t.Fatalf("Selector = %q, want ID", got[0].Selector())
		}
	})

	t.Run("a nameless field is skipped", func(t *testing.T) {
		t.Parallel()
		// A malformed graph can carry one; the model's own embeds are
		// recorded separately.
		s := &node.Struct{Name: "User", Fields: []*node.Field{nil, {Name: ""}}}
		if got, _ := golang.FieldSet(s, nil); len(got) != 0 {
			t.Fatalf("FieldSet = %v, want none", names(got))
		}
	})

	t.Run("an embed naming nothing is skipped", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "User", Embeds: []*node.Embed{{}}}
		got, complete := golang.FieldSet(s, mapResolver{})
		if len(got) != 0 || !complete {
			t.Fatalf("FieldSet = %v, %v", names(got), complete)
		}
	})

	t.Run("an embedded non-struct contributes no fields", func(t *testing.T) {
		t.Parallel()
		// An interface contributes methods; see PromotedMethods.
		iface := &node.Interface{Name: "Reader", Package: "io"}
		s := &node.Struct{Name: "W", Embeds: []*node.Embed{embed("io", "Reader", false)}}
		got, complete := golang.FieldSet(s, mapResolver{"io.Reader": iface})
		if !complete || !slices.Equal(names(got), []string{"Reader"}) {
			t.Fatalf("FieldSet = %v, %v", names(got), complete)
		}
	})

	t.Run("a depth beyond the budget is incomplete", func(t *testing.T) {
		t.Parallel()
		// A chain longer than any real declaration stops rather than
		// walking forever.
		r := mapResolver{}
		for i := range 12 {
			name := "L" + strconv.Itoa(i)
			next := "L" + strconv.Itoa(i+1)
			r["x."+name] = &node.Struct{
				Name: name, Package: "x",
				Embeds: []*node.Embed{embed("x", next, false)},
			}
		}
		if _, complete := golang.FieldSet(r["x.L0"].(*node.Struct), r); complete {
			t.Fatalf("a chain past the budget must report incomplete")
		}
	})

	t.Run("promoted methods come from every declaring kind", func(t *testing.T) {
		t.Parallel()
		// A type switch, and an arm left out silently promotes nothing.
		for name, decl := range map[string]node.Node{
			"Struct":    &node.Struct{Name: "B", Package: "x", Methods: []*node.Method{{Name: "M"}}},
			"Interface": &node.Interface{Name: "B", Package: "x", Methods: []*node.Method{{Name: "M"}}},
			"Enum":      &node.Enum{Name: "B", Package: "x", Methods: []*node.Method{{Name: "M"}}},
			"Alias":     &node.Alias{Name: "B", Package: "x", Methods: []*node.Method{{Name: "M"}}},
		} {
			s := &node.Struct{Name: "W", Embeds: []*node.Embed{embed("x", "B", false)}}
			got, _ := golang.PromotedMethods(s, mapResolver{"x.B": decl})
			if len(got) != 1 {
				t.Errorf("PromotedMethods through %s = %d, want 1", name, len(got))
			}
		}
	})

	t.Run("an embedded kind with no methods promotes none", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "W", Embeds: []*node.Embed{embed("x", "V", false)}}
		got, _ := golang.PromotedMethods(s, mapResolver{"x.V": &node.Variable{Name: "V", Package: "x"}})
		if len(got) != 0 {
			t.Fatalf("PromotedMethods = %+v, want none", got)
		}
	})

	t.Run("promoted methods without a resolver are incomplete", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "W", Embeds: []*node.Embed{embed("x", "B", false)}}
		if _, complete := golang.PromotedMethods(s, nil); complete {
			t.Fatalf("PromotedMethods without a resolver must report incomplete")
		}
	})

	t.Run("a nameless embed is skipped by the method walk too", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "W", Embeds: []*node.Embed{{}}}
		got, complete := golang.PromotedMethods(s, mapResolver{})
		if len(got) != 0 || !complete {
			t.Fatalf("PromotedMethods = %+v, %v", got, complete)
		}
	})

	t.Run("EmbedsType skips an embed naming nothing", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "W", Embeds: []*node.Embed{{}, embed("x", "Base", false)}}
		if !golang.EmbedsType(s, "x.Base", nil) {
			t.Fatalf("EmbedsType = false")
		}
	})

	t.Run("EmbedsType tolerates an unresolvable embed", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "W", Embeds: []*node.Embed{embed("x", "Missing", false)}}
		if golang.EmbedsType(s, "x.Base", mapResolver{}) {
			t.Fatalf("EmbedsType = true through an unresolvable embed")
		}
	})
}
