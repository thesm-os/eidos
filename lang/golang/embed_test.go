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

// reasons projects the classification of every unresolved embed, so a
// test asserts on why a walk stopped rather than only that it did.
func reasons(problems []golang.UnresolvedEmbed) []golang.ResolveProblem {
	out := make([]golang.ResolveProblem, len(problems))
	for i, p := range problems {
		out[i] = p.Reason
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
		got, problems := golang.FieldSet(s, r)
		if len(problems) != 0 {
			t.Fatalf("FieldSet problems = %v", reasons(problems))
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

	t.Run("an unreachable embed is reported by name and reason", func(t *testing.T) {
		t.Parallel()
		// The answer is smaller rather than wrong, and the caller must
		// not treat it as complete.
		s := &node.Struct{
			Name: "User", Package: "x",
			Embeds: []*node.Embed{embed("x", "Missing", false)},
		}
		got, problems := golang.FieldSet(s, mapResolver{})
		if len(problems) != 1 || problems[0].Reason != golang.NotLoaded {
			t.Fatalf("FieldSet problems = %+v, want one not-loaded", problems)
		}
		if problems[0].Written != "x.Missing" || problems[0].Host != "x.User" {
			t.Fatalf("problem = %+v, want it to name the embed and its embedder", problems[0])
		}
		if !slices.Equal(names(got), []string{"Missing"}) {
			t.Fatalf("FieldSet = %v, want the embed itself", names(got))
		}
	})

	t.Run("a generic embed is refused rather than substituted", func(t *testing.T) {
		t.Parallel()
		// Its fields are typed in the embedded declaration's type
		// parameters, so copying them across names identifiers that are
		// not in scope here.
		generic := &node.Struct{
			Name: "Box", Package: "x",
			Fields: []*node.Field{field("Value", namedTypeRef("", "T"))},
		}
		e := embed("x", "Box", false)
		e.Type.TypeArgs = []*node.TypeRef{builtinRef("int")}
		s := &node.Struct{Name: "User", Package: "x", Embeds: []*node.Embed{e}}

		got, problems := golang.FieldSet(s, mapResolver{"x.Box": generic})
		if !slices.Equal(reasons(problems), []golang.ResolveProblem{golang.GenericEmbed}) {
			t.Fatalf("FieldSet problems = %v, want generic", reasons(problems))
		}
		// The embedded field itself stays reachable by its own name.
		if !slices.Equal(names(got), []string{"Box"}) {
			t.Fatalf("FieldSet = %v, want only the embed itself", names(got))
		}
	})

	t.Run("an in-package embed resolves against the embedder's package", func(t *testing.T) {
		t.Parallel()
		// The frontend records `type S struct{ Base }` with an empty
		// package on the reference, because that is how the source
		// reads. A resolver keyed by qualified name has nothing to look
		// up until the gap is closed.
		s := &node.Struct{
			Name: "User", Package: "x",
			Embeds: []*node.Embed{embed("", "Base", false)},
		}
		got, problems := golang.FieldSet(s, r)
		if len(problems) != 0 {
			t.Fatalf("FieldSet problems = %v", reasons(problems))
		}
		if !slices.Contains(names(got), "ID") {
			t.Fatalf("FieldSet = %v, want the promoted ID", names(got))
		}
	})

	t.Run("no resolver sees only the first level", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{
			Name:   "User",
			Fields: []*node.Field{field("Name", builtinRef("string"))},
			Embeds: []*node.Embed{embed("x", "Base", false)},
		}
		got, problems := golang.FieldSet(s, nil)
		// Distinct from not-loaded: the same graph answers in full once
		// a resolver is passed, so the caller's remedy is different.
		if !slices.Equal(reasons(problems), []golang.ResolveProblem{golang.NoResolver}) {
			t.Fatalf("FieldSet problems = %v, want no-resolver", reasons(problems))
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
		got, problems := golang.FieldSet(nil, r)
		if len(got) != 0 || len(problems) != 0 {
			t.Fatalf("FieldSet(nil) = %v, %v", got, problems)
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
		got, problems := golang.PromotedMethods(s, r)
		if len(problems) != 0 || len(got) != 1 || got[0].Method.Name != "Read" {
			t.Fatalf("PromotedMethods = %+v, %v", got, problems)
		}
		if got[0].Through() != "Reader" || got[0].Selector() != "Reader.Read" {
			t.Fatalf("Through = %q, Selector = %q", got[0].Through(), got[0].Selector())
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

	t.Run("an unreachable embed is reported", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "Wrapper", Embeds: []*node.Embed{embed("io", "Missing", false)}}
		_, problems := golang.PromotedMethods(s, mapResolver{})
		if !slices.Equal(reasons(problems), []golang.ResolveProblem{golang.NotLoaded}) {
			t.Fatalf("PromotedMethods problems = %v, want not-loaded", reasons(problems))
		}
	})

	t.Run("an embedded interface contributes its flattened set", func(t *testing.T) {
		t.Parallel()
		// ReadCloser declares nothing of its own; a struct embedding it
		// still has Read and Close, so taking its Methods verbatim would
		// promote nothing at all.
		closer := &node.Interface{
			Name: "Closer", Package: "io", Methods: []*node.Method{{Name: "Close"}},
		}
		rc := &node.Interface{
			Name: "ReadCloser", Package: "io",
			Embeds: []*node.Embed{embed("io", "Reader", false), embed("io", "Closer", false)},
		}
		s := &node.Struct{Name: "W", Embeds: []*node.Embed{embed("io", "ReadCloser", false)}}

		got, problems := golang.PromotedMethods(s, mapResolver{
			"io.Reader": reader, "io.Closer": closer, "io.ReadCloser": rc,
		})
		if len(problems) != 0 {
			t.Fatalf("PromotedMethods problems = %v", reasons(problems))
		}
		if len(got) != 2 || got[0].Method.Name != "Read" || got[1].Method.Name != "Close" {
			t.Fatalf("PromotedMethods = %+v, want Read and Close", got)
		}
	})

	t.Run("a method reached through two embeds at equal depth cancels", func(t *testing.T) {
		t.Parallel()
		// Go makes the ambiguous selector an error rather than a choice,
		// so neither promotes and the caller must qualify explicitly.
		other := &node.Interface{
			Name: "Other", Package: "io", Methods: []*node.Method{{Name: "Read"}},
		}
		s := &node.Struct{
			Name:   "W",
			Embeds: []*node.Embed{embed("io", "Reader", false), embed("io", "Other", false)},
		}
		got, _ := golang.PromotedMethods(s, mapResolver{"io.Reader": reader, "io.Other": other})
		if len(got) != 0 {
			t.Fatalf("PromotedMethods = %+v, want no ambiguous Read", got)
		}
	})

	t.Run("a shallower method wins over a deeper one", func(t *testing.T) {
		t.Parallel()
		deep := &node.Struct{
			Name: "Deep", Package: "x", Methods: []*node.Method{{Name: "Read"}},
		}
		mid := &node.Struct{
			Name: "Mid", Package: "x", Embeds: []*node.Embed{embed("x", "Deep", false)},
		}
		s := &node.Struct{
			Name: "W", Package: "x",
			Embeds: []*node.Embed{embed("io", "Reader", false), embed("x", "Mid", false)},
		}
		got, _ := golang.PromotedMethods(s, mapResolver{
			"io.Reader": reader, "x.Mid": mid, "x.Deep": deep,
		})
		if len(got) != 1 || got[0].Depth != 1 || got[0].Through() != "Reader" {
			t.Fatalf("PromotedMethods = %+v, want the depth-1 Read", got)
		}
	})

	t.Run("nil is an empty complete set", func(t *testing.T) {
		t.Parallel()
		got, problems := golang.PromotedMethods(nil, r)
		if len(got) != 0 || len(problems) != 0 {
			t.Fatalf("PromotedMethods(nil) = %+v, %v", got, problems)
		}
	})
}

func TestPromotedMemberEdges(t *testing.T) {
	t.Parallel()

	t.Run("a zero promoted method has no first hop", func(t *testing.T) {
		t.Parallel()
		// Exported, so a caller can hold one; Through indexing a path it
		// never checked would panic on it.
		var p golang.PromotedMethod
		if p.Through() != "" {
			t.Fatalf("Through = %q, want empty", p.Through())
		}
	})

	t.Run("an embedded cycle terminates EmbedsType", func(t *testing.T) {
		t.Parallel()
		loop := &node.Struct{Name: "Loop", Package: "x"}
		loop.Embeds = []*node.Embed{embed("x", "Loop", true)}
		if golang.EmbedsType(loop, "x.Absent", mapResolver{"x.Loop": loop}) {
			t.Fatalf("EmbedsType found a type the cycle does not embed")
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
		got, problems := golang.FieldSet(s, mapResolver{})
		if len(got) != 0 || len(problems) != 0 {
			t.Fatalf("FieldSet = %v, %v", names(got), reasons(problems))
		}
	})

	t.Run("an embedded non-struct contributes no fields", func(t *testing.T) {
		t.Parallel()
		// An interface contributes methods; see PromotedMethods.
		iface := &node.Interface{Name: "Reader", Package: "io"}
		s := &node.Struct{Name: "W", Embeds: []*node.Embed{embed("io", "Reader", false)}}
		got, problems := golang.FieldSet(s, mapResolver{"io.Reader": iface})
		if len(problems) != 0 || !slices.Equal(names(got), []string{"Reader"}) {
			t.Fatalf("FieldSet = %v, %v", names(got), reasons(problems))
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
		_, problems := golang.FieldSet(r["x.L0"].(*node.Struct), r)
		if !slices.Equal(reasons(problems), []golang.ResolveProblem{golang.TooDeep}) {
			t.Fatalf("FieldSet problems = %v, want too-deep", reasons(problems))
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
		_, problems := golang.PromotedMethods(s, nil)
		if !slices.Equal(reasons(problems), []golang.ResolveProblem{golang.NoResolver}) {
			t.Fatalf("PromotedMethods problems = %v, want no-resolver", reasons(problems))
		}
	})

	t.Run("a deeper candidate found first still loses", func(t *testing.T) {
		t.Parallel()
		// Candidates arrive in embed order, so the shallower one is not
		// necessarily seen first and the rule cannot be "keep the first".
		deep := &node.Struct{
			Name: "Deep", Package: "x", Methods: []*node.Method{{Name: "Do"}},
		}
		mid := &node.Struct{
			Name: "Mid", Package: "x", Embeds: []*node.Embed{embed("x", "Deep", false)},
		}
		shallow := &node.Struct{
			Name: "Shallow", Package: "x", Methods: []*node.Method{{Name: "Do"}},
		}
		s := &node.Struct{
			Name: "W", Package: "x",
			Embeds: []*node.Embed{embed("x", "Mid", false), embed("x", "Shallow", false)},
		}
		got, _ := golang.PromotedMethods(s, mapResolver{
			"x.Mid": mid, "x.Deep": deep, "x.Shallow": shallow,
		})
		if len(got) != 1 || got[0].Depth != 1 || got[0].Through() != "Shallow" {
			t.Fatalf("PromotedMethods = %+v, want the depth-1 Do", got)
		}
	})

	t.Run("a nameless promoted method is skipped", func(t *testing.T) {
		t.Parallel()
		// A malformed graph can carry one, and a promoted member with
		// no name has no selector to generate.
		src := &node.Struct{
			Name: "Src", Package: "x",
			Methods: []*node.Method{nil, {Name: ""}, {Name: "Do"}},
		}
		s := &node.Struct{Name: "W", Package: "x", Embeds: []*node.Embed{embed("x", "Src", false)}}
		got, _ := golang.PromotedMethods(s, mapResolver{"x.Src": src})
		if len(got) != 1 || got[0].Method.Name != "Do" {
			t.Fatalf("PromotedMethods = %+v, want only Do", got)
		}
	})

	t.Run("a struct cycle terminates the method walk", func(t *testing.T) {
		t.Parallel()
		loop := &node.Struct{
			Name: "Loop", Package: "x", Methods: []*node.Method{{Name: "Go"}},
		}
		loop.Embeds = []*node.Embed{embed("x", "Loop", true)}
		got, _ := golang.PromotedMethods(loop, mapResolver{"x.Loop": loop})
		if len(got) != 0 {
			// Loop declares Go itself, so the promoted copy is shadowed;
			// what matters is that the walk returned at all.
			t.Fatalf("PromotedMethods = %+v", got)
		}
	})

	t.Run("a resolver yielding a typed nil is survived", func(t *testing.T) {
		t.Parallel()
		// A resolver is consumer-supplied. One returning a nil *Struct
		// as a non-nil node.Node passes the type assertion, and a walk
		// that dereferenced it would panic inside the framework.
		s := &node.Struct{Name: "W", Package: "x", Embeds: []*node.Embed{embed("x", "B", false)}}
		r := mapResolver{"x.B": (*node.Struct)(nil)}
		if got, _ := golang.PromotedMethods(s, r); len(got) != 0 {
			t.Fatalf("PromotedMethods = %+v, want none", got)
		}
		if got, _ := golang.FieldSet(s, r); len(names(got)) != 1 {
			t.Fatalf("FieldSet = %v, want only the embed itself", names(got))
		}
	})

	t.Run("a nameless embed is skipped by the method walk too", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "W", Embeds: []*node.Embed{{}}}
		got, problems := golang.PromotedMethods(s, mapResolver{})
		if len(got) != 0 || len(problems) != 0 {
			t.Fatalf("PromotedMethods = %+v, %v", got, reasons(problems))
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

// A struct embedding an interface takes that interface's whole set,
// including what the interface itself embedded. The walk is
// [node.MethodSet]; what is pinned here is the crossing — that its
// issues arrive in this package's vocabulary rather than being
// dropped, and that a type-set term is not one of them.
func TestPromotedMethodsThroughAnInterface(t *testing.T) {
	t.Parallel()

	reader := &node.Interface{
		Name: "Reader", Package: "io", Methods: []*node.Method{{Name: "Read"}},
	}
	closer := &node.Interface{
		Name: "Closer", Package: "io", Methods: []*node.Method{{Name: "Close"}},
	}
	rc := &node.Interface{
		Name: "ReadCloser", Package: "io",
		Embeds: []*node.Embed{embed("io", "Reader", false), embed("io", "Closer", false)},
	}

	t.Run("carries the embedded interface's flattened set", func(t *testing.T) {
		t.Parallel()
		// ReadCloser declares nothing of its own, so a walk reading
		// Methods alone would promote nothing at all.
		s := &node.Struct{Name: "W", Package: "x", Embeds: []*node.Embed{embed("io", "ReadCloser", false)}}
		got, problems := golang.PromotedMethods(s, mapResolver{
			"io.Reader": reader, "io.Closer": closer, "io.ReadCloser": rc,
		})
		if len(problems) != 0 {
			t.Fatalf("problems = %v", reasons(problems))
		}
		if len(got) != 2 || got[0].Method.Name != "Read" || got[1].Method.Name != "Close" {
			t.Fatalf("PromotedMethods = %+v, want Read and Close", got)
		}
	})

	t.Run("reports an embed the interface could not resolve", func(t *testing.T) {
		t.Parallel()
		// The model's walk answers in its own vocabulary; dropping the
		// issues here would emit a double short a method and say
		// nothing about why.
		broken := &node.Interface{
			Name: "Composed", Package: "io",
			Embeds: []*node.Embed{embed("io", "Absent", false)},
		}
		s := &node.Struct{Name: "W", Package: "x", Embeds: []*node.Embed{embed("io", "Composed", false)}}
		_, problems := golang.PromotedMethods(s, mapResolver{"io.Composed": broken})
		if !slices.Equal(reasons(problems), []golang.ResolveProblem{golang.NotLoaded}) {
			t.Fatalf("problems = %v, want not-loaded", reasons(problems))
		}
	})

	t.Run("a type-set term in the interface is not a problem", func(t *testing.T) {
		t.Parallel()
		// The defect the two walkers used to disagree about: a term
		// constraining a type set is not an embed that failed.
		termed := &node.Interface{
			Name: "Termed", Package: "io",
			Methods: []*node.Method{{Name: "Do"}},
			Embeds: []*node.Embed{{Type: &node.TypeRef{
				TypeKind: node.TypeRefSlice, Elem: builtinRef("byte"),
			}}},
		}
		s := &node.Struct{Name: "W", Package: "x", Embeds: []*node.Embed{embed("io", "Termed", false)}}
		got, problems := golang.PromotedMethods(s, mapResolver{"io.Termed": termed})
		if len(problems) != 0 {
			t.Fatalf("problems = %v, want none", reasons(problems))
		}
		if len(got) != 1 || got[0].Method.Name != "Do" {
			t.Fatalf("PromotedMethods = %+v, want Do", got)
		}
	})
}

func TestStructOf(t *testing.T) {
	t.Parallel()

	base := &node.Struct{Name: "Base", Package: "x", Fields: []*node.Field{field("ID", builtinRef("string"))}}
	iface := &node.Interface{Name: "Store", Package: "x"}
	r := mapResolver{"x.Base": base, "x.Store": iface}

	t.Run("resolves a named reference to its struct", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.StructOf(namedTypeRef("x", "Base"), r)
		if !ok || got != base {
			t.Fatalf("StructOf = %v, %v; want the Base declaration", got, ok)
		}
	})

	t.Run("a declaration that is not a struct is no struct", func(t *testing.T) {
		t.Parallel()
		// False rather than a wrong answer: an interface resolves fine
		// and has no members to aim at.
		if got, ok := golang.StructOf(namedTypeRef("x", "Store"), r); ok {
			t.Fatalf("StructOf(interface) = %v, true", got)
		}
	})

	t.Run("a type the run never loaded is no struct", func(t *testing.T) {
		t.Parallel()
		// The smaller answer. A run over one package cannot see a type
		// declared in another, and guessing from the name would make
		// the same source answer differently under a wider run.
		if _, ok := golang.StructOf(namedTypeRef("elsewhere", "Base"), r); ok {
			t.Fatal("StructOf resolved a type nothing loaded")
		}
	})

	t.Run("a pointer is not followed", func(t *testing.T) {
		t.Parallel()
		// `*T` and `T` differ where the caller is deciding what to
		// emit, so the strip is the caller's to ask for.
		ptr := &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: namedTypeRef("x", "Base")}
		if _, ok := golang.StructOf(ptr, r); ok {
			t.Fatal("StructOf followed a pointer")
		}
		if got, ok := golang.StructOf(golang.Deref(ptr), r); !ok || got != base {
			t.Fatalf("StructOf(Deref(ptr)) = %v, %v; want the Base declaration", got, ok)
		}
	})

	t.Run("no reference and no resolver report nothing", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.StructOf(nil, r); ok {
			t.Error("StructOf(nil) resolved something")
		}
		if _, ok := golang.StructOf(namedTypeRef("x", "Base"), nil); ok {
			t.Error("StructOf with no resolver resolved something")
		}
	})
}

func TestMemberField(t *testing.T) {
	t.Parallel()

	base := &node.Struct{
		Name: "Base", Package: "x",
		Fields: []*node.Field{field("Version", builtinRef("int")), field("secret", builtinRef("string"))},
	}
	user := &node.Struct{
		Name: "User", Package: "x",
		Fields: []*node.Field{field("Name", builtinRef("string")), field("age", builtinRef("int"))},
		Embeds: []*node.Embed{embed("x", "Base", false)},
	}
	r := mapResolver{"x.Base": base}

	t.Run("answers a declared member's type", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.MemberField(user, "Name", r)
		if !ok || got.Name != "string" {
			t.Fatalf("MemberField = %v, %v; want string", got, ok)
		}
	})

	t.Run("reaches a promoted member", func(t *testing.T) {
		t.Parallel()
		// Promotion is what makes the emitted selector legal: `u.Version`
		// compiles, so a lookup reading only what the source typed would
		// refuse to aim at a member the source can reach.
		got, ok := golang.MemberField(user, "Version", r)
		if !ok || got.Name != "int" {
			t.Fatalf("MemberField = %v, %v; want int", got, ok)
		}
	})

	t.Run("an unexported member is not reachable", func(t *testing.T) {
		t.Parallel()
		// Visible to the declaring package and to nothing else, so a
		// generated file routed elsewhere naming one does not compile.
		if _, ok := golang.MemberField(user, "age", r); ok {
			t.Error("MemberField answered for a declared unexported member")
		}
		if _, ok := golang.MemberField(user, "secret", r); ok {
			t.Error("MemberField answered for a promoted unexported member")
		}
	})

	t.Run("a name nothing carries is false", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.MemberField(user, "Nonesuch", r); ok {
			t.Error("MemberField answered for a member nobody declared")
		}
	})

	t.Run("a member behind an unresolvable embed is false", func(t *testing.T) {
		t.Parallel()
		// The limit the docblock states. This is indistinguishable from
		// "no such member" here, and ExportedFieldSet is what tells the
		// two apart — asserted together so the pair cannot drift.
		if _, ok := golang.MemberField(user, "Version", mapResolver{}); ok {
			t.Fatal("MemberField reached through an embed nothing resolved")
		}
		_, problems := golang.ExportedFieldSet(user, mapResolver{})
		if !slices.Contains(reasons(problems), golang.NotLoaded) {
			t.Fatalf("ExportedFieldSet problems = %v, want it to report the embed", reasons(problems))
		}
	})

	t.Run("no struct is no member", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.MemberField(nil, "Name", r); ok {
			t.Error("MemberField(nil) answered")
		}
	})
}
