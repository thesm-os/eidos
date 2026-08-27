// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/priority"
	"go.thesmos.sh/eidos/sdk"
)

// TestTypeAliasesPreserveIdentity pins the contract that every
// re-exported type in [sdk] is a Go type alias — not a wrapper.
// Aliases preserve identity, so a value of type [plugin.Plugin]
// is also a value of type [sdk.Plugin] without conversion. The
// checks below assign across the alias boundary; if any alias
// accidentally becomes a wrapper, this test fails to compile
// rather than at runtime.
//
// The deliberately-redundant `var x plugin.X = sdkAlias`
// pattern is the only way to express the identity assertion at
// compile time. staticcheck ST1023 flags the redundancy; we
// suppress it because the redundancy is the test.
//
//nolint:staticcheck // intentional redundant typing — see docblock above
func TestTypeAliasesPreserveIdentity(t *testing.T) {
	t.Parallel()

	t.Run("role interfaces alias to plugin package", func(t *testing.T) {
		t.Parallel()
		var p1 sdk.Plugin
		var p2 plugin.Plugin = p1
		_ = p2
		var f1 sdk.Frontend
		var f2 plugin.Frontend = f1
		_ = f2
		var a1 sdk.Annotator
		var a2 plugin.Annotator = a1
		_ = a2
		var g1 sdk.Generator
		var g2 plugin.Generator = g1
		_ = g2
		var b1 sdk.Backend
		var b2 plugin.Backend = b1
		_ = b2
	})

	t.Run("the rules interfaces alias to plugin package", func(t *testing.T) {
		t.Parallel()
		// The three halves included: a helper taking only the type
		// questions states the narrower contract, and it can only do
		// that if naming that half needs no import past this package.
		var sr1 sdk.SourceRules
		var sr2 plugin.SourceRules = sr1
		_ = sr2
		var dr1 sdk.DeclarationRules
		var dr2 plugin.DeclarationRules = dr1
		_ = dr2
		var tr1 sdk.TypeRules
		var tr2 plugin.TypeRules = tr1
		_ = tr2
		var nr1 sdk.NamingRules
		var nr2 plugin.NamingRules = nr1
		_ = nr2
		// And the composition holds across the boundary: SourceRules
		// satisfying all three is what lets a caller pass the whole
		// where a half is asked for.
		var whole sdk.SourceRules
		var part sdk.TypeRules = whole
		_ = part
	})

	t.Run("contexts alias to plugin package", func(t *testing.T) {
		t.Parallel()
		var fc1 sdk.FrontendContext
		var fc2 plugin.FrontendContext = fc1
		_ = fc2
		var ac1 sdk.AnnotatorContext
		var ac2 plugin.AnnotatorContext = ac1
		_ = ac2
		var gc1 sdk.GeneratorContext
		var gc2 plugin.GeneratorContext = gc1
		_ = gc2
		var bc1 sdk.BackendContext
		var bc2 plugin.BackendContext = bc1
		_ = bc2
	})

	t.Run("capability interfaces alias to plugin package", func(t *testing.T) {
		t.Parallel()
		var c1 sdk.CapabilityProvider
		var c2 plugin.CapabilityProvider = c1
		_ = c2
		var d1 sdk.DirectiveProvider
		var d2 plugin.DirectiveProvider = d1
		_ = d2
		var o1 sdk.OptionsProvider
		var o2 plugin.OptionsProvider = o1
		_ = o2
		var v1 sdk.Versioned
		var v2 plugin.Versioned = v1
		_ = v2
	})

	t.Run("Priority aliases to priority package", func(t *testing.T) {
		t.Parallel()
		var p1 sdk.Priority = sdk.GeneratorFoundation
		var p2 priority.Priority = p1
		_ = p2
	})

	t.Run("directive types alias to directive package", func(t *testing.T) {
		t.Parallel()
		var n1 sdk.DirectiveName = "repo"
		var n2 directive.Name = n1
		_ = n2
		var s1 sdk.DirectiveSchema
		var s2 directive.Schema = s1
		_ = s2
		var d1 sdk.Directive
		var d2 directive.Directive = d1
		_ = d2
	})

	t.Run("Kind aliases to kind package", func(t *testing.T) {
		t.Parallel()
		var k1 sdk.Kind = "struct"
		var k2 kind.Kind = k1
		_ = k2
	})
}

// TestPriorityBucketsMatchUnderlying pins that the SDK's
// re-exported bucket constants resolve to exactly the same
// numeric values as the underlying priority package. A typo or
// stale re-export here would silently reorder a plugin's
// scheduling.
func TestPriorityBucketsMatchUnderlying(t *testing.T) {
	t.Parallel()
	pairs := []struct {
		name     string
		got      sdk.Priority
		expected priority.Priority
	}{
		{"AnnotatorShape", sdk.AnnotatorShape, priority.AnnotatorShape},
		{"AnnotatorRefinement", sdk.AnnotatorRefinement, priority.AnnotatorRefinement},
		{"AnnotatorValidation", sdk.AnnotatorValidation, priority.AnnotatorValidation},
		{"GeneratorFoundation", sdk.GeneratorFoundation, priority.GeneratorFoundation},
		{"GeneratorComposition", sdk.GeneratorComposition, priority.GeneratorComposition},
		{"GeneratorCrossCutting", sdk.GeneratorCrossCutting, priority.GeneratorCrossCutting},
		{"GeneratorFinalize", sdk.GeneratorFinalize, priority.GeneratorFinalize},
		{"DefaultPriority", sdk.DefaultPriority, priority.Default},
	}
	for _, pair := range pairs {
		if pair.got != pair.expected {
			t.Errorf("sdk.%s = %d, want %d (priority.%s)",
				pair.name, pair.got, pair.expected, pair.name)
		}
	}
}

// TestDirectiveBuilderProxiesUnderlying drives the SDK's
// NewDirective + builder-option helpers and confirms the
// resulting schema is structurally equivalent to one built
// directly through the underlying directive package. Both call
// chains must compile and produce equivalent schemas — anything
// else means the SDK's re-export drifted from the source.
func TestDirectiveBuilderProxiesUnderlying(t *testing.T) {
	t.Parallel()
	viaFacade := sdk.NewDirective("repo").
		On(sdk.Kind("struct")).
		Positional("variant", sdk.Required(), sdk.OneOf("default", "stub")).
		Describe("Repository directive").
		Build()
	viaUnderlying := directive.NewSchema("repo").
		On(kind.Kind("struct")).
		Positional("variant", directive.Required(), directive.OneOf("default", "stub")).
		Describe("Repository directive").
		Build()
	if viaFacade.Name != viaUnderlying.Name {
		t.Errorf("Schema.Name via façade = %q, via underlying = %q",
			viaFacade.Name, viaUnderlying.Name)
	}
	if len(viaFacade.AppliesTo) != len(viaUnderlying.AppliesTo) {
		t.Errorf("Schema.AppliesTo length differs: facade=%d underlying=%d",
			len(viaFacade.AppliesTo), len(viaUnderlying.AppliesTo))
	}
	if len(viaFacade.PositionalArgs) != len(viaUnderlying.PositionalArgs) {
		t.Errorf("Schema.PositionalArgs length differs: facade=%d underlying=%d",
			len(viaFacade.PositionalArgs), len(viaUnderlying.PositionalArgs))
	}
}

// TestNodeKindsMatchUnderlying pins the source-kind re-exports to
// node's values, and — the property the prefix exists for — pins
// them as distinct from emit's identically-named constants.
//
// Every one of the eighteen source kinds shares a name with an emit
// kind carrying a different value ("struct" against "emit.struct").
// Re-exporting either set unprefixed would let an author reach for
// one and receive the other, and both halves fail silently: a slot
// constrained on a source kind accepts nothing an emit builder
// produces, and a directive scoped to an emit kind matches no source
// node, so the plugin never fires and nothing reports it.
func TestNodeKindsMatchUnderlying(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sdk  kind.Kind
		node kind.Kind
	}{
		{"Package", sdk.NodeKindPackage, node.KindPackage},
		{"File", sdk.NodeKindFile, node.KindFile},
		{"Import", sdk.NodeKindImport, node.KindImport},
		{"Struct", sdk.NodeKindStruct, node.KindStruct},
		{"Interface", sdk.NodeKindInterface, node.KindInterface},
		{"Method", sdk.NodeKindMethod, node.KindMethod},
		{"Field", sdk.NodeKindField, node.KindField},
		{"Function", sdk.NodeKindFunction, node.KindFunction},
		{"Param", sdk.NodeKindParam, node.KindParam},
		{"Return", sdk.NodeKindReturn, node.KindReturn},
		{"TypeParam", sdk.NodeKindTypeParam, node.KindTypeParam},
		{"TypeRef", sdk.NodeKindTypeRef, node.KindTypeRef},
		{"Alias", sdk.NodeKindAlias, node.KindAlias},
		{"Constant", sdk.NodeKindConstant, node.KindConstant},
		{"Variable", sdk.NodeKindVariable, node.KindVariable},
		{"Enum", sdk.NodeKindEnum, node.KindEnum},
		{"EnumVariant", sdk.NodeKindEnumVariant, node.KindEnumVariant},
		{"Embed", sdk.NodeKindEmbed, node.KindEmbed},
	}

	t.Run("each re-export equals its node constant", func(t *testing.T) {
		t.Parallel()
		for _, tc := range cases {
			if tc.sdk != tc.node {
				t.Errorf("NodeKind%s = %q, want %q", tc.name, tc.sdk, tc.node)
			}
		}
	})

	t.Run("the source set covers every node kind", func(t *testing.T) {
		t.Parallel()
		// Counted against node/kind.go itself rather than a literal,
		// because a literal would need updating by the same commit
		// that forgets the re-export. A kind added to node without a
		// matching entry here leaves a directive unable to scope
		// against it from sdk alone — the gap this set closed.
		declared := declaredKindsIn(t, "node")
		if len(cases) != declared {
			t.Fatalf("re-exported %d source kinds, node/kind.go declares %d",
				len(cases), declared)
		}
	})

	t.Run("no source kind collides with its emit namesake", func(t *testing.T) {
		t.Parallel()
		emitKinds := map[string]kind.Kind{
			"Package": emit.KindPackage, "File": emit.KindFile,
			"Import": emit.KindImport, "Struct": emit.KindStruct,
			"Interface": emit.KindInterface, "Method": emit.KindMethod,
			"Field": emit.KindField, "Function": emit.KindFunction,
			"Param": emit.KindParam, "Return": emit.KindReturn,
			"TypeParam": emit.KindTypeParam, "TypeRef": emit.KindTypeRef,
			"Alias": emit.KindAlias, "Constant": emit.KindConstant,
			"Variable": emit.KindVariable, "Enum": emit.KindEnum,
			"EnumVariant": emit.KindEnumVariant, "Embed": emit.KindEmbed,
		}
		for _, tc := range cases {
			ek, ok := emitKinds[tc.name]
			if !ok {
				continue
			}
			if tc.sdk == ek {
				t.Errorf("NodeKind%s and emit.Kind%s share the value %q; "+
					"the prefix stops being a distinction", tc.name, tc.name, ek)
			}
		}
	})
}

// TestEmitContractAliasesPreserveIdentity pins the emit-side
// contracts as aliases rather than wrappers, so a plugin's own type
// satisfies them without conversion and can assert so at compile
// time.
//
//nolint:staticcheck // intentional redundant typing — see TestTypeAliasesPreserveIdentity
func TestEmitContractAliasesPreserveIdentity(t *testing.T) {
	t.Parallel()

	t.Run("emit contracts alias to the emit package", func(t *testing.T) {
		t.Parallel()
		var o1 sdk.OutputPackageSetter
		var o2 emit.OutputPackageSetter = o1
		_ = o2
		var h1 sdk.SlotHost
		var h2 emit.SlotHost = h1
		_ = h2
		var s1 *sdk.Slot
		var s2 *emit.Slot = s1
		_ = s2
	})

	t.Run("NewSlot builds a usable slot", func(t *testing.T) {
		t.Parallel()
		// The re-export exists so a plugin can define a slot for
		// others to fill; a factory that returned nothing usable
		// would satisfy the alias test and still be useless.
		s := sdk.NewSlot("chain", "")
		if s == nil {
			t.Fatal("NewSlot returned nil")
		}
		if got := s.SlotName; got != "chain" {
			t.Fatalf("Slot.Name = %q, want chain", got)
		}
	})
}

// TestErrEmptyKeyIsDistinct pins the fourth parser sentinel as
// re-exported and as its own failure mode.
func TestErrEmptyKeyIsDistinct(t *testing.T) {
	t.Parallel()

	t.Run("aliases the directive sentinel", func(t *testing.T) {
		t.Parallel()
		if !errors.Is(sdk.ErrEmptyKey, directive.ErrEmptyKey) {
			t.Fatalf("sdk.ErrEmptyKey does not match directive.ErrEmptyKey")
		}
	})

	t.Run("is distinguishable from a malformed directive", func(t *testing.T) {
		t.Parallel()
		// Re-exporting three of four sentinels left a plugin unable
		// to tell "not a directive" from "a directive with a broken
		// pair" through sdk alone.
		if errors.Is(sdk.ErrEmptyKey, sdk.ErrMalformedDirective) {
			t.Fatalf("ErrEmptyKey must not match ErrMalformedDirective")
		}
	})
}

// declaredKindsIn counts the Kind constants pkg/kind.go declares,
// read from the file so the count cannot drift from the source of
// truth the way a hand-maintained literal would.
//
// Shared by the source-kind and emit-kind coverage checks: both
// sets have to stay complete, and a per-set copy of this parse
// would be one more thing to forget.
func declaredKindsIn(t *testing.T, pkg string) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join("..", pkg, "kind.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse %s/kind.go: %v", pkg, err)
	}
	n := 0
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if strings.HasPrefix(name.Name, "Kind") {
					n++
				}
			}
		}
	}
	return n
}

// A façade that re-exports a type and withholds the constructor that
// makes it is a façade a caller leaves. These three were the last of
// those: `Sink`, `Store` and `StoreReader` were all reachable while
// `diag.New`, `store.New` and `store.NewReader` were not, so a test
// assembling a phase context by hand imported past the façade for one
// call apiece.
func TestConstructorsMatchTheirTypes(t *testing.T) {
	t.Parallel()

	t.Run("makes a diagnostic sink", func(t *testing.T) {
		t.Parallel()
		//nolint:staticcheck // the redundant type is the identity assertion.
		var got *sdk.Sink = sdk.NewSink()
		if got == nil {
			t.Fatal("NewSink returned nil")
		}
	})

	t.Run("makes a store and a reader over it", func(t *testing.T) {
		t.Parallel()
		// The pairing is the point: a plugin's test needs both to build
		// a context, and the reader is also the Resolver `lang/golang`
		// asks for.
		//nolint:staticcheck // the redundant type is the identity assertion.
		var s *sdk.Store = sdk.NewStore()
		if s == nil {
			t.Fatal("NewStore returned nil")
		}
		//nolint:staticcheck // the redundant type is the identity assertion.
		var r *sdk.StoreReader = sdk.NewStoreReader(s)
		if r == nil {
			t.Fatal("NewStoreReader returned nil")
		}
	})

	t.Run("assembles a phase context without leaving the facade", func(t *testing.T) {
		t.Parallel()
		// The whole claim, spelled as the thing it enables. Every type
		// and every constructor below comes from this package.
		s := sdk.NewStore()
		ctx := &sdk.GeneratorContext{
			Store:  s,
			Reader: sdk.NewStoreReader(s),
			Diag:   sdk.NewSink(),
		}
		if ctx.Reader == nil || ctx.Diag == nil {
			t.Fatalf("context = %+v", ctx)
		}
	})
}

// TestLastReadsTheFinalDirective pins the accessor issue #8 needed, and
// the property that makes it distinct from the node's own method.
//
// A node's Directive method returns the *first* match and answers "is
// this declared". A repeatable value-carrying directive needs the
// opposite rule: an author writing the directive twice has said the
// second value, and a generator reading the first emits a value the
// source contradicts two lines below — with no diagnostic, because
// both directives are well-formed.
func TestLastReadsTheFinalDirective(t *testing.T) {
	t.Parallel()

	list := []*directive.Directive{
		{Name: "default", KV: map[string]string{"limit": "10"}},
		{Name: "other"},
		{Name: "default", KV: map[string]string{"limit": "50"}},
	}

	t.Run("returns the final match, not the first", func(t *testing.T) {
		t.Parallel()
		got := sdk.Last(list, "default")
		if got == nil {
			t.Fatal("Last returned nil for a directive that is present")
		}
		if got.KV["limit"] != "50" {
			t.Fatalf("limit = %q, want 50 — Last must not read the first match", got.KV["limit"])
		}
	})

	t.Run("disagrees with the node's first-wins accessor", func(t *testing.T) {
		t.Parallel()
		// The two rules must be distinguishable through the façade, or
		// a plugin whose schema promises last-wins has no way to honour
		// it. This asserts they genuinely differ on the same input.
		n := &sdk.BaseNode{DirectiveList: list}
		first, last := n.Directive("default"), sdk.Last(list, "default")
		if first.KV["limit"] == last.KV["limit"] {
			t.Fatalf("first-wins and last-wins agree (%q); the fixture no longer "+
				"exercises the distinction", first.KV["limit"])
		}
	})

	t.Run("returns nil when no directive matches", func(t *testing.T) {
		t.Parallel()
		if got := sdk.Last(list, "absent"); got != nil {
			t.Fatalf("Last(absent) = %+v, want nil", got)
		}
	})
}

// TestNewOptionsBuildsASetSetOptionsAccepts pins the constructor issue
// #9 needed: a plugin's own test driving one option value through
// without reaching past the façade.
func TestNewOptionsBuildsASetSetOptionsAccepts(t *testing.T) {
	t.Parallel()

	type pluginOptions struct {
		Suffix string `eidos:"suffix,default=_gen.go"`
	}

	t.Run("a value set through the façade reaches the bound target", func(t *testing.T) {
		t.Parallel()
		var opts pluginOptions
		h := sdk.BindOptions(&opts)
		if err := h.SetOptions(sdk.NewOptions(h.OptionsSchema(), map[string]string{
			"suffix": "_stub.go",
		})); err != nil {
			t.Fatalf("SetOptions: %v", err)
		}
		if opts.Suffix != "_stub.go" {
			t.Fatalf("Suffix = %q, want _stub.go", opts.Suffix)
		}
	})

	t.Run("an unknown key is rejected", func(t *testing.T) {
		t.Parallel()
		var opts pluginOptions
		h := sdk.BindOptions(&opts)
		if err := h.SetOptions(sdk.NewOptions(h.OptionsSchema(), map[string]string{
			"nope": "x",
		})); err == nil {
			t.Fatal("SetOptions accepted a key the schema does not declare")
		}
	})
}

// TestExprKindsMatchUnderlying pins the ExprKind re-export as
// complete: every variant emit declares has an alias here, at the
// same value.
//
// The count is read from emit/expr.go the way the kind-coverage
// checks read kind.go, so a variant added there fails this test
// rather than sending the next consumer past the façade for the
// missing name.
func TestExprKindsMatchUnderlying(t *testing.T) {
	t.Parallel()

	aliased := map[emit.ExprKind]string{
		sdk.ExprLiteral: "ExprLiteral", sdk.ExprIdent: "ExprIdent",
		sdk.ExprField: "ExprField", sdk.ExprIndex: "ExprIndex",
		sdk.ExprIndexList: "ExprIndexList", sdk.ExprSlice: "ExprSlice",
		sdk.ExprCall: "ExprCall", sdk.ExprMethodCall: "ExprMethodCall",
		sdk.ExprAddr: "ExprAddr", sdk.ExprDeref: "ExprDeref",
		sdk.ExprParen: "ExprParen", sdk.ExprUnary: "ExprUnary",
		sdk.ExprBinary: "ExprBinary", sdk.ExprTypeAssert: "ExprTypeAssert",
		sdk.ExprComposite: "ExprComposite", sdk.ExprCompositeKeyed: "ExprCompositeKeyed",
		sdk.ExprFuncLit: "ExprFuncLit", sdk.ExprMake: "ExprMake",
		sdk.ExprNew: "ExprNew", sdk.ExprRaw: "ExprRaw",
		sdk.ExprExternal: "ExprExternal",
	}

	t.Run("every declared variant has an alias", func(t *testing.T) {
		t.Parallel()
		// The map above collapses on a value collision, so its length
		// equalling the declared count also proves each alias is bound
		// to a distinct underlying constant — a copy-paste alias bound
		// to its neighbour shrinks the map.
		if want := declaredExprKindsIn(t); len(aliased) != want {
			t.Fatalf("sdk aliases %d ExprKind variants, emit declares %d", len(aliased), want)
		}
	})

	t.Run("the alias preserves identity", func(t *testing.T) {
		t.Parallel()
		//nolint:staticcheck // intentional redundant typing — the identity is the test
		var k emit.ExprKind = sdk.ExprComposite
		_ = k
	})
}

// declaredExprKindsIn counts the ExprKind variants emit/expr.go
// declares, on the terms [declaredKindsIn] reads kind.go.
func declaredExprKindsIn(t *testing.T) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join("..", "emit", "expr.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse emit/expr.go: %v", err)
	}
	n := 0
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if strings.HasPrefix(name.Name, "Expr") {
					n++
				}
			}
		}
	}
	return n
}
