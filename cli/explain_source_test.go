// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/cli"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// explainSourceMetaKey is the string meta key the source-anchored
// meta selectors below read. Declared at package scope so the
// global registry registers it once regardless of -count.
var explainSourceMetaKey = meta.NewKey("cli.explain.source.shape", meta.StringParser)

// namedRef, sliceRef and ptrRef build the [node.TypeRef] shapes the
// fixtures below need. Declared here so the fixture bodies read as
// the signature they model rather than as nested struct literals.
func namedRef(pkg, name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Package: pkg, Name: name}
}

func sliceRef(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: elem}
}

func ptrRef(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: elem}
}

// sourceFixture is the source-side graph every test in this file
// anchors against, plus the emit-side counterparts wired back to it
// by Origin.
//
// The two sides have to be built together: `explain`'s whole job is
// to walk from a source declaration to what the pipeline made of
// it, so a fixture carrying only one side exercises the lookup but
// never the reporting.
type sourceFixture struct {
	pkg      *node.Package
	user     *node.Struct
	save     *node.Method
	email    *node.Field
	colour   *node.Enum
	red      *node.EnumVariant
	emitPkg  *emit.Package
	mock     *emit.Struct
	mockSave *emit.Method
}

// newSourceFixture builds the paired source and emit graphs. The
// emit side models what a mock generator produces: a `UserMock`
// struct whose Origin is the source struct, carrying a `Save`
// method whose Origin is the source method, with a prebody slot
// contribution from a separate weaver plugin so the slot reporters
// have two distinct attributions to render.
func newSourceFixture(t *testing.T) *sourceFixture {
	t.Helper()

	f := &sourceFixture{}

	f.save = &node.Method{
		Name: "Save",
		BaseNode: node.BaseNode{
			SourcePos: position.Pos{File: "users/user.go", Line: 12},
		},
		Params: []*node.Param{
			{Name: "ctx", Type: namedRef("context", "Context")},
			{Type: sliceRef(namedRef("", "byte"))},
		},
		Returns: []*node.Return{
			{Name: "n", Type: namedRef("", "int")},
			{Type: namedRef("", "error")},
		},
	}
	f.email = &node.Field{
		Name: "Email",
		Tag:  `json:"email"`,
		Type: ptrRef(namedRef("", "string")),
	}
	f.user = &node.Struct{
		Name: "User", Package: "users",
		BaseNode: node.BaseNode{
			SourcePos: position.Pos{File: "users/user.go", Line: 8},
			DirectiveList: []*directive.Directive{
				{Name: "mock", Raw: "mock pkg=usersmock"},
				{Name: "validate", Negated: true},
			},
		},
		Methods: []*node.Method{f.save},
		Fields:  []*node.Field{f.email},
	}
	f.save.Owner = f.user
	f.email.Owner = f.user
	explainSourceMetaKey.Set(f.user.EnsureMeta(), "aggregate-root", "shapes")
	explainSourceMetaKey.Set(f.save.EnsureMeta(), "persister", "shapes")

	f.red = &node.EnumVariant{Name: "Red", Value: "1"}
	f.colour = &node.Enum{
		Name: "Colour", Package: "users",
		Variants: []*node.EnumVariant{f.red},
	}
	f.red.Owner = f.colour

	f.pkg = &node.Package{
		Name: "users", Path: "users",
		Structs: []*node.Struct{f.user},
		Enums:   []*node.Enum{f.colour},
	}

	target := emit.Target{Dir: "users", Filename: "user_mock.go", Package: "users"}
	f.mockSave = &emit.Method{
		BaseEmit: emit.BaseEmit{OriginNode: f.save},
		Name:     "Save",
	}
	if err := f.mockSave.Prebody().Append(
		emit.NewExprStmt(emit.NewIdent("trace")),
		emit.Provenance{SetBy: "tracegen", ID: "trace.entry"},
	); err != nil {
		t.Fatalf("seed prebody: %v", err)
	}
	f.mock = &emit.Struct{
		BaseEmit: emit.BaseEmit{OriginNode: f.user, SetByName: "mockgen"},
		Name:     "UserMock", Package: "users", Target: target,
		Methods: []*emit.Method{f.mockSave},
	}
	f.mockSave.Owner = f.mock
	if err := f.mock.FieldsSlot().Append(
		&emit.Field{Name: "calls", Type: emit.Builtin("int")},
		emit.Provenance{SetBy: "countergen"},
	); err != nil {
		t.Fatalf("seed fields slot: %v", err)
	}

	f.emitPkg = &emit.Package{
		Name: "users", Path: "users",
		Structs: []*emit.Struct{f.mock},
	}
	return f
}

// run drives the explain command over the fixture with the supplied
// selector and returns the exit code plus both streams.
func (f *sourceFixture) run(t *testing.T, selector string) (int, string, string) {
	t.Helper()
	env, stdout, stderr := freshEnv(t, "eidos")
	cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
		Plugins: []plugin.Plugin{
			sourceFrontend{name: "fe", pkg: f.pkg},
			emittingGenerator{name: "mockgen", pkg: f.emitPkg},
			stubBackend{name: "be", lang: "stub"},
		},
		Selector: selector,
	}}
	code := cmd.Execute(t.Context(), env)
	return code, stdout.String(), stderr.String()
}

// TestExplainCommand_SourceMeta covers the `anchor#key` form
// resolved against the source graph. This is the selector a user
// reaches for to answer "what did the annotators decide about this
// declaration", so an unset key has to be distinguishable from a
// key set to an empty value — hence the ExitUserError rather than
// printing nothing and exiting zero.
func TestExplainCommand_SourceMeta(t *testing.T) {
	t.Parallel()

	t.Run("prints the value of a key set on a source entity", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User#cli.explain.source.shape")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "aggregate-root") {
			t.Fatalf("expected the meta value; got %q", stdout)
		}
	})

	t.Run("prints the value of a key set on a source member", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User.Save#cli.explain.source.shape")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "persister") {
			t.Fatalf("expected the member's meta value; got %q", stdout)
		}
	})

	t.Run("an unset key on a source entity exits ExitUserError", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := newSourceFixture(t).run(t, "users.User#never.set")
		if code != cli.ExitUserError {
			t.Fatalf("Execute = %d, want ExitUserError", code)
		}
		if !strings.Contains(stderr, "is unset on") {
			t.Fatalf("expected an unset-key diagnostic; got %q", stderr)
		}
	})
}

// TestExplainCommand_SourceSlot covers the `anchor:slot` form
// resolved against the source graph. Slots live on emit decls, so
// the reporter has to walk one hop forward through Origin — the
// three outcomes below are "found it", "nothing was generated from
// this source", and "something was, but it does not carry that
// slot", which are different problems for the user to act on.
func TestExplainCommand_SourceSlot(t *testing.T) {
	t.Parallel()

	t.Run("reports contributions on the emit counterpart of a source entity", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User:fields")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "countergen") {
			t.Fatalf("expected the contributing plugin; got %q", stdout)
		}
	})

	t.Run("a source entity with no emit counterpart exits ExitUserError", func(t *testing.T) {
		t.Parallel()
		// Colour is in the source graph but nothing was generated
		// from it, so there is no slot bag to inspect at all.
		code, _, stderr := newSourceFixture(t).run(t, "users.Colour:fields")
		if code != cli.ExitUserError {
			t.Fatalf("Execute = %d, want ExitUserError", code)
		}
		if !strings.Contains(stderr, "no emit-side counterpart") {
			t.Fatalf("expected a no-counterpart diagnostic; got %q", stderr)
		}
	})

	t.Run("a slot with no contributions exits ExitUserError", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := newSourceFixture(t).run(t, "users.User:methods")
		if code != cli.ExitUserError {
			t.Fatalf("Execute = %d, want ExitUserError", code)
		}
		if !strings.Contains(stderr, "no contributions") {
			t.Fatalf("expected a no-contributions diagnostic; got %q", stderr)
		}
	})
}

// TestExplainCommand_SourceMemberSlot covers `owner.member:slot`.
// Cross-cutting weavers attach to emit methods rather than to the
// source method, so this is the only selector that reaches a
// prebody contribution from the source side — the question "who
// injected code into my method" has no other answer in the tool.
func TestExplainCommand_SourceMemberSlot(t *testing.T) {
	t.Parallel()

	t.Run("reports contributions on the emit counterpart of a source member", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User.Save:prebody")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "tracegen") {
			t.Fatalf("expected the weaver attribution; got %q", stdout)
		}
	})

	t.Run("reports the contribution's provenance id", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User.Save:prebody")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "trace.entry") {
			t.Fatalf("expected the anchor id so later plugins can target it; got %q", stdout)
		}
	})

	t.Run("a member with no emit counterpart exits ExitUserError", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := newSourceFixture(t).run(t, "users.User.Email:prebody")
		if code != cli.ExitUserError {
			t.Fatalf("Execute = %d, want ExitUserError", code)
		}
		if !strings.Contains(stderr, "no emit-side counterpart") {
			t.Fatalf("expected a no-counterpart diagnostic; got %q", stderr)
		}
	})

	t.Run("an empty slot on the counterpart exits ExitUserError", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := newSourceFixture(t).run(t, "users.User.Save:postbody")
		if code != cli.ExitUserError {
			t.Fatalf("Execute = %d, want ExitUserError", code)
		}
		if !strings.Contains(stderr, "no contributions on any emit-side counterpart") {
			t.Fatalf("expected a no-contributions diagnostic; got %q", stderr)
		}
	})
}

// TestExplainCommand_SourceMemberShape covers the per-member report
// body: the kind-specific shape line and the type rendering behind
// it. `explain` is the tool a user consults when generated output
// does not match what they expected the model to hold, so the
// rendered signature has to reflect the model rather than the
// source text — a dropped return name or a flattened pointer would
// send them looking in the wrong place.
func TestExplainCommand_SourceMemberShape(t *testing.T) {
	t.Parallel()

	t.Run("a method renders its full signature", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User.Save")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "Save(ctx context.Context, arg1 []byte) (n int, error)") {
			t.Fatalf("expected the rendered signature; got %q", stdout)
		}
	})

	t.Run("an anonymous parameter renders a positional name", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User.Save")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "arg1 []byte") {
			t.Fatalf("an unnamed param needs a positional stand-in; got %q", stdout)
		}
	})

	t.Run("a field renders its type and struct tag", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User.Email")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "*string") {
			t.Fatalf("expected the pointer type rendering; got %q", stdout)
		}
		if !strings.Contains(stdout, `json:"email"`) {
			t.Fatalf("expected the struct tag; got %q", stdout)
		}
	})

	t.Run("an enum variant renders its value", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.Colour.Red")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "Value:") || !strings.Contains(stdout, "1") {
			t.Fatalf("expected the variant value line; got %q", stdout)
		}
	})

	t.Run("a member with an emit counterpart lists it under Outputs", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User.Save")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "Save on UserMock (method)") {
			t.Fatalf("expected the emit member line naming its owner; got %q", stdout)
		}
	})

	t.Run("a member inherits its owner's plugin attribution", func(t *testing.T) {
		t.Parallel()
		// The emit method carries no SetBy of its own — it is
		// rendered inline by the host struct's template — so the
		// report has to fall back to the host's attribution rather
		// than showing the member as unattributed.
		code, stdout, _ := newSourceFixture(t).run(t, "users.User.Save")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "mockgen") {
			t.Fatalf("expected the host's attribution; got %q", stdout)
		}
	})

	t.Run("a member with a slotted counterpart lists the slot summary", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User.Save")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "Slots:") || !strings.Contains(stdout, "contribution(s)") {
			t.Fatalf("expected a slot summary line; got %q", stdout)
		}
	})

	t.Run("a member with no emit counterpart says so explicitly", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User.Email")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "no plugin emitted output for this member") {
			t.Fatalf("silence would read as a bug in explain; got %q", stdout)
		}
	})
}

// TestExplainCommand_SourceDirectives covers the directive
// rendering shared by the entity and member reports. A directive is
// rendered back into the form the user typed, so a negated
// directive must not read as a positive one — that inversion would
// send someone hunting for why an opt-out was ignored.
func TestExplainCommand_SourceDirectives(t *testing.T) {
	t.Parallel()

	t.Run("a positive directive renders with its arguments", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "+gen:mock pkg=usersmock") {
			t.Fatalf("expected the directive with its args; got %q", stdout)
		}
	})

	t.Run("a negated directive renders with a minus prefix", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "-gen:validate") {
			t.Fatalf("a negated directive must not read as positive; got %q", stdout)
		}
	})

	t.Run("a source entity lists its emit outputs grouped by plugin", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.User")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "mockgen:") {
			t.Fatalf("expected a per-plugin group header; got %q", stdout)
		}
	})

	t.Run("a source entity with no emit output says so explicitly", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := newSourceFixture(t).run(t, "users.Colour")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "no plugin emitted output for this source") {
			t.Fatalf("expected the explicit no-output line; got %q", stdout)
		}
	})
}

// TestExplainCommand_EmitMeta covers the `anchor#key` form falling
// through to the emit graph — the debug-side path for synthetic
// decls that carry no source anchor at all.
func TestExplainCommand_EmitMeta(t *testing.T) {
	t.Parallel()

	// emitOnly builds a store whose emit graph holds one entity with
	// no source counterpart, so the anchor resolves only emit-side.
	emitOnly := func(t *testing.T, selector string) (int, string, string) {
		t.Helper()
		env, stdout, stderr := freshEnv(t, "eidos")
		host := &emit.Struct{
			Name: "Synthetic", Package: "gen",
			Target: emit.Target{Dir: "gen", Filename: "synthetic.go", Package: "gen"},
		}
		explainSourceMetaKey.Set(host.EnsureMeta(), "synthesised", "gen")
		cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				emittingGenerator{name: "gen", pkg: &emit.Package{
					Name: "gen", Path: "gen",
					Structs: []*emit.Struct{host},
				}},
				stubBackend{name: "be", lang: "stub"},
			},
			Selector: selector,
		}}
		return cmd.Execute(t.Context(), env), stdout.String(), stderr.String()
	}

	t.Run("prints the value of a key set on an emit entity", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := emitOnly(t, "gen.Synthetic#cli.explain.source.shape")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "synthesised") {
			t.Fatalf("expected the meta value; got %q", stdout)
		}
	})

	t.Run("an unset key on an emit entity exits ExitUserError", func(t *testing.T) {
		t.Parallel()
		code, _, stderr := emitOnly(t, "gen.Synthetic#never.set")
		if code != cli.ExitUserError {
			t.Fatalf("Execute = %d, want ExitUserError", code)
		}
		if !strings.Contains(stderr, "is unset on") {
			t.Fatalf("expected an unset-key diagnostic; got %q", stderr)
		}
	})
}

// TestExplainCommand_RenderTypeRef covers the type-projection the
// per-member report prints. `explain` reports what the model holds,
// so every composite kind the node graph can carry needs a
// one-line spelling — a kind that falls through to a bare name
// would show `Attrs` where the user expects `map[string]int`, and
// they would conclude the frontend dropped the type.
func TestExplainCommand_RenderTypeRef(t *testing.T) {
	t.Parallel()

	// fieldTyped builds a fixture whose User struct carries one
	// extra field of the supplied type, then explains that field.
	fieldTyped := func(t *testing.T, name string, typ *node.TypeRef) string {
		t.Helper()
		f := newSourceFixture(t)
		extra := &node.Field{Name: name, Type: typ, Owner: f.user}
		f.user.Fields = append(f.user.Fields, extra)
		code, stdout, stderr := f.run(t, "users.User."+name)
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK (stderr %q)", code, stderr)
		}
		return stdout
	}

	t.Run("a map renders key and value types", func(t *testing.T) {
		t.Parallel()
		out := fieldTyped(t, "Attrs", &node.TypeRef{
			TypeKind: node.TypeRefMap,
			MapKey:   namedRef("", "string"),
			MapValue: namedRef("", "int"),
		})
		if !strings.Contains(out, "map[string]int") {
			t.Fatalf("expected the map spelling; got %q", out)
		}
	})

	t.Run("an array renders its length", func(t *testing.T) {
		t.Parallel()
		out := fieldTyped(t, "Buf", &node.TypeRef{
			TypeKind: node.TypeRefArray,
			ArrayLen: 16,
			Elem:     namedRef("", "byte"),
		})
		if !strings.Contains(out, "[16]byte") {
			t.Fatalf("an array must not render as a slice; got %q", out)
		}
	})

	t.Run("a type parameter renders as its bare identifier", func(t *testing.T) {
		t.Parallel()
		out := fieldTyped(t, "Item", &node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: "T"})
		if !strings.Contains(out, "Type:") || !strings.Contains(out, "T") {
			t.Fatalf("expected the type-parameter identifier; got %q", out)
		}
	})

	t.Run("a kind with no one-line spelling falls back to its name", func(t *testing.T) {
		t.Parallel()
		out := fieldTyped(t, "Fn", &node.TypeRef{TypeKind: node.TypeRefFunc, Name: "handler"})
		if !strings.Contains(out, "handler") {
			t.Fatalf("expected the fallback name; got %q", out)
		}
	})
}

// TestExplainCommand_IndirectAttribution covers the correlation
// fallback: an emit member whose own Origin is unset, matched to a
// source member by name plus its host's origin.
//
// This is the shape mock and weaver plugins actually produce — they
// stamp Origin on the generated struct but not on each method,
// because the methods are rendered inline by the host's template.
// Without the fallback, `explain` would report "no plugin emitted
// output for this member" for every method of every mock in the
// tree, which is the exact question the command exists to answer.
func TestExplainCommand_IndirectAttribution(t *testing.T) {
	t.Parallel()

	// indirect builds a fixture whose emit graph carries a port
	// interface and a second mock struct, neither stamping Origin on
	// its members.
	indirect := func(t *testing.T) *sourceFixture {
		t.Helper()
		f := newSourceFixture(t)
		port := &emit.Interface{
			BaseEmit: emit.BaseEmit{OriginNode: f.user, SetByName: "portgen"},
			Name:     "UserPort", Package: "users",
			Target: emit.Target{Dir: "users", Filename: "user_port.go", Package: "users"},
		}
		portSave := &emit.Method{Name: "Save", Owner: port}
		port.Methods = []*emit.Method{portSave}

		record := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: f.user, SetByName: "portgen"},
			Name:     "UserRecord", Package: "users",
			Target: emit.Target{Dir: "users", Filename: "user_record.go", Package: "users"},
		}
		recEmail := &emit.Field{Name: "Email", Type: emit.Builtin("string"), Owner: record}
		record.Fields = []*emit.Field{recEmail}

		f.emitPkg.Interfaces = append(f.emitPkg.Interfaces, port)
		f.emitPkg.Structs = append(f.emitPkg.Structs, record)
		return f
	}

	t.Run("a field is correlated through its host's origin", func(t *testing.T) {
		t.Parallel()
		// The source Email field has no directly-anchored emit
		// counterpart; only the name-plus-host-origin match reaches
		// UserRecord.Email.
		code, stdout, _ := indirect(t).run(t, "users.User.Email")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "Email on UserRecord (field)") {
			t.Fatalf("expected the correlated field line; got %q", stdout)
		}
	})

	t.Run("an interface host renders by name in the owner label", func(t *testing.T) {
		t.Parallel()
		f := indirect(t)
		// Drop the directly-anchored method so the interface's
		// inline method is the only candidate for Save.
		f.mock.Methods = nil
		f.mockSave.Owner = nil
		f.mockSave.OriginNode = nil
		code, stdout, _ := f.run(t, "users.User.Save")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "Save on UserPort (method)") {
			t.Fatalf("expected the interface owner label; got %q", stdout)
		}
	})

	t.Run("multiple contributions from one plugin are counted in the group header", func(t *testing.T) {
		t.Parallel()
		// portgen emits two decls from the same source struct, so the
		// group header has to distinguish "one output" from "several"
		// rather than silently listing them under a bare name.
		code, stdout, _ := indirect(t).run(t, "users.User")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "portgen: (2 contributions)") {
			t.Fatalf("expected a counted group header; got %q", stdout)
		}
	})
}

// TestExplainCommand_EveryEmitKind covers the output enumeration
// across every emit bucket the collector walks, plus the emit-side
// entity report those decls resolve to.
//
// The collector is a fixed sequence of per-bucket scans rather than
// a generic walk, so a bucket omitted from it fails silently: the
// decl is generated, written to disk, and simply never mentioned
// when the user asks what a source declaration produced. A test
// covering only structs cannot distinguish that from a correct
// implementation.
func TestExplainCommand_EveryEmitKind(t *testing.T) {
	t.Parallel()

	const pkg = "orders"

	// everyKind builds a source struct plus one emit decl of every
	// routable kind anchored to it, and a file-level slot
	// contribution originating from the same source node.
	everyKind := func(t *testing.T) (*node.Package, *emit.Package) {
		t.Helper()
		order := &node.Struct{
			Name: "Order", Package: pkg,
			BaseNode: node.BaseNode{
				SourcePos:     position.Pos{File: "orders/order.go", Line: 4},
				DirectiveList: []*directive.Directive{{Name: "persist", Raw: "persist"}},
			},
		}
		explainSourceMetaKey.Set(order.EnsureMeta(), "entity", "shapes")

		target := emit.Target{Dir: pkg, Filename: "order_gen.go", Package: pkg}
		base := func() emit.BaseEmit {
			return emit.BaseEmit{OriginNode: order, SetByName: "ordergen"}
		}

		file := &emit.File{Name: "order_gen.go", Package: pkg, Dir: pkg}
		if err := file.Init().Append(
			&emit.Variable{BaseEmit: base(), Name: "registered", Package: pkg},
			emit.Provenance{SetBy: "registrygen"},
		); err != nil {
			t.Fatalf("seed file init slot: %v", err)
		}

		return &node.Package{
				Name: pkg, Path: pkg,
				Structs: []*node.Struct{order},
			}, &emit.Package{
				Name: pkg, Path: pkg,
				Files:      []*emit.File{file},
				Structs:    []*emit.Struct{{BaseEmit: base(), Name: "OrderMock", Package: pkg, Target: target}},
				Interfaces: []*emit.Interface{{BaseEmit: base(), Name: "OrderPort", Package: pkg, Target: target}},
				Functions:  []*emit.Function{{BaseEmit: base(), Name: "NewOrder", Package: pkg, Target: target}},
				Variables:  []*emit.Variable{{BaseEmit: base(), Name: "DefaultOrder", Package: pkg, Target: target}},
				Constants:  []*emit.Constant{{BaseEmit: base(), Name: "MaxOrders", Package: pkg, Target: target}},
				Enums:      []*emit.Enum{{BaseEmit: base(), Name: "OrderState", Package: pkg, Target: target}},
				Aliases:    []*emit.Alias{{BaseEmit: base(), Name: "OrderID", Package: pkg, File: target}},
			}
	}

	run := func(t *testing.T, selector string) (int, string, string) {
		t.Helper()
		src, out := everyKind(t)
		env, stdout, stderr := freshEnv(t, "eidos")
		cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
			Plugins: []plugin.Plugin{
				sourceFrontend{name: "fe", pkg: src},
				emittingGenerator{name: "ordergen", pkg: out},
				stubBackend{name: "be", lang: "stub"},
			},
			Selector: selector,
		}}
		return cmd.Execute(t.Context(), env), stdout.String(), stderr.String()
	}

	t.Run("every routable emit kind appears under Outputs", func(t *testing.T) {
		t.Parallel()
		code, stdout, stderr := run(t, "orders.Order")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK (stderr %q)", code, stderr)
		}
		for _, kind := range []string{
			"(struct)", "(interface)", "(function)",
			"(variable)", "(constant)", "(enum)", "(alias)",
		} {
			if !strings.Contains(stdout, kind) {
				t.Errorf("Outputs omitted a %s decl; got %q", kind, stdout)
			}
		}
	})

	t.Run("a file-level slot contribution appears under Outputs", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := run(t, "orders.Order")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "slot:init") {
			t.Fatalf("a decl landing in a file slot must still be attributed; got %q", stdout)
		}
	})

	t.Run("a file-slot contribution is attributed to its contributing plugin", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := run(t, "orders.Order")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "registrygen") {
			t.Fatalf("the slot's provenance names the contributor, not the host; got %q", stdout)
		}
	})

	t.Run("an emit-only function anchor resolves and reports its kind", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := run(t, "orders.NewOrder")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "emit.function") {
			t.Fatalf("expected the function kind line; got %q", stdout)
		}
	})

	t.Run("an emit-only alias anchor resolves through the last bucket", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := run(t, "orders.OrderID")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "emit.alias") {
			t.Fatalf("expected the alias kind line; got %q", stdout)
		}
	})

	t.Run("an emit entity reports its source origin position", func(t *testing.T) {
		t.Parallel()
		code, stdout, _ := run(t, "orders.OrderState")
		if code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout, "orders/order.go") {
			t.Fatalf("expected the origin position; got %q", stdout)
		}
	})
}
