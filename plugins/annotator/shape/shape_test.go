// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape_test

import (
	"reflect"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// frontendMarker is the key the framework's frontends stamp on
// every produced package — the shape plugin reads it to pick the
// per-language detector. Declared in tests via [sdk.EnsureKey] so
// the same canonical key is returned regardless of init order.
//
//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = sdk.EnsureKey("frontend", sdk.StringParser)

// TestPlugin_Contract covers the contract surface every framework
// component relies on at registration / build time: stable name,
// role implementation, deterministic priority, well-formed
// directive schema, and graceful behaviour on an empty store.
func TestPlugin_Contract(t *testing.T) {
	t.Parallel()

	t.Run("Name is the package-exported constant and stable across calls", func(t *testing.T) {
		t.Parallel()
		p := shape.New()
		if got, want := p.Name(), shape.PluginName; got != want {
			t.Fatalf("Name() = %q, want %q", got, want)
		}
		first := p.Name()
		second := p.Name()
		if first != second {
			t.Fatalf("Name() flapped: first=%q, second=%q", first, second)
		}
	})

	t.Run("satisfies sdk.Annotator", func(t *testing.T) {
		t.Parallel()
		var _ sdk.Annotator = shape.New()
	})

	t.Run("satisfies sdk.DirectiveProvider", func(t *testing.T) {
		t.Parallel()
		var _ sdk.DirectiveProvider = shape.New()
	})

	t.Run("Priority is the shape-detection bucket", func(t *testing.T) {
		t.Parallel()
		if got, want := shape.New().Priority(), sdk.AnnotatorShape; got != want {
			t.Fatalf("Priority() = %v, want %v", got, want)
		}
	})

	t.Run("Directives declares the +gen:shape and +gen:contract schemas", func(t *testing.T) {
		t.Parallel()
		schemas := shape.New().Directives()
		names := make(map[sdk.DirectiveName]bool, len(schemas))
		for _, s := range schemas {
			names[s.Name] = true
		}
		if !names[shape.DirectiveName] {
			t.Fatalf("missing %q schema; got names = %v", shape.DirectiveName, names)
		}
		if !names[shape.ContractDirectiveName] {
			t.Fatalf("missing %q schema; got names = %v", shape.ContractDirectiveName, names)
		}
	})

	t.Run("the annotator registers no template helpers", func(t *testing.T) {
		t.Parallel()
		// It ships no templates, so it has nothing to register for.
		// An earlier form handed every plugin its own prefixed copy of
		// the whole Go vocabulary, which meant this annotator carried
		// a hundred helpers no template of its own could call — and
		// two plugins reaching for one name registered it twice.
		// The backend owns that vocabulary now, in one copy.
		provider, ok := any(shape.New()).(sdk.TemplateProvider)
		if !ok {
			t.Fatalf("shape.New() no longer implements sdk.TemplateProvider")
		}
		if funcs := provider.TemplateFuncs(langGolang); len(funcs) != 0 {
			t.Errorf("TemplateFuncs(%q) = %v, want none", langGolang, funcs)
		}
		if _, ok := provider.Templates(langGolang); ok {
			t.Errorf("Templates(%q) reported a tree; the annotator ships none", langGolang)
		}
	})

	t.Run("Annotate on empty store does not error or panic", func(t *testing.T) {
		t.Parallel()
		ctx := newAnnotatorContext(t, store.New())
		if err := shape.New().Annotate(ctx); err != nil {
			t.Fatalf("Annotate(empty): %v", err)
		}
	})

	t.Run("Annotate is idempotent: a second pass leaves meta unchanged", func(t *testing.T) {
		t.Parallel()
		p := shape.New().Detectors(testReaderDetector())
		ctx := newAnnotatorContext(t, buildStoreWithReader(t))

		if err := p.Annotate(ctx); err != nil {
			t.Fatalf("first Annotate: %v", err)
		}
		first := snapshotShapeMeta(ctx.Store)

		if err := p.Annotate(ctx); err != nil {
			t.Fatalf("second Annotate: %v", err)
		}
		second := snapshotShapeMeta(ctx.Store)

		if !reflect.DeepEqual(first, second) {
			t.Fatalf("Annotate not idempotent:\n first  = %v\n second = %v", first, second)
		}
	})
}

// TestPlugin_DirectiveOverride pins the user-supplied-stamp-wins
// contract: a `+gen:shape` directive on a callable pre-stamps the
// meta before any detector runs, and per-key modifiers reach the
// matching shape.* meta keys verbatim.
func TestPlugin_DirectiveOverride(t *testing.T) {
	t.Parallel()

	t.Run("positional argument carries the shape name", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Custom", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					{Name: shape.DirectiveName, Args: []string{"lifecycle"}},
				},
			},
		}
		runAnnotate(t, shape.New(), pkgWithFunction(fn))
		assertShape(t, fn.Meta(), "lifecycle")
	})

	t.Run("kind= key carries the shape name when no positional", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Custom", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					{
						Name: shape.DirectiveName,
						KV:   map[string]string{"kind": "mutator"},
					},
				},
			},
		}
		runAnnotate(t, shape.New(), pkgWithFunction(fn))
		assertShape(t, fn.Meta(), "mutator")
	})

	t.Run("key= and value= modifiers populate KeyType / ValueType", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Custom", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					{
						Name: shape.DirectiveName,
						Args: []string{"reader"},
						KV: map[string]string{
							"key":   "string",
							"value": "Article",
						},
					},
				},
			},
		}
		runAnnotate(t, shape.New(), pkgWithFunction(fn))
		assertMeta(t, fn.Meta(), shape.MetaKeyType, "string")
		assertMeta(t, fn.Meta(), shape.MetaValueType, "Article")
	})

	t.Run("negated directive is ignored", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Custom", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					{Name: shape.DirectiveName, Args: []string{"lifecycle"}, Negated: true},
				},
			},
		}
		runAnnotate(t, shape.New(), pkgWithFunction(fn))
		if shape.IsStamped(fn.Meta()) {
			t.Fatalf("negated directive should not stamp; got shape=%q", shape.Get(fn.Meta()))
		}
	})

	t.Run("directive override wins over a detector match", func(t *testing.T) {
		t.Parallel()
		// Reader detector would normally stamp "reader"; the
		// directive forces "lifecycle" and the detector must skip
		// via the already-stamped guard.
		fn := readerFunc("Find")
		fn.DirectiveList = []*sdk.Directive{
			{Name: shape.DirectiveName, Args: []string{"lifecycle"}},
		}
		runAnnotate(t, shape.New().Detectors(testReaderDetector()), pkgWithFunction(fn))
		assertShape(t, fn.Meta(), "lifecycle")
	})
}

// TestPlugin_DetectorDispatch covers per-frontend detector
// dispatch: the registered DetectFunc fires for callables in
// matching-frontend packages and skips for any other.
func TestPlugin_DetectorDispatch(t *testing.T) {
	t.Parallel()

	t.Run("matching Go function gets stamped via detector", func(t *testing.T) {
		t.Parallel()
		fn := readerFunc("Get")
		runAnnotate(t, shape.New().Detectors(testReaderDetector()), pkgWithFunction(fn))
		assertShape(t, fn.Meta(), "reader")
	})

	t.Run("matching method on a struct gets stamped via detector", func(t *testing.T) {
		t.Parallel()
		m := readerMethod("Get")
		s := &sdk.Struct{
			Name: "Repo", Package: "x",
			Methods: []*sdk.Method{m},
		}
		runAnnotate(t, shape.New().Detectors(testReaderDetector()), pkgWithStruct(s))
		assertShape(t, m.Meta(), "reader")
	})

	t.Run("matching method on an interface gets stamped via detector", func(t *testing.T) {
		t.Parallel()
		m := readerMethod("Get")
		i := &sdk.Interface{
			Name: "Repo", Package: "x",
			Methods: []*sdk.Method{m},
		}
		runAnnotate(t, shape.New().Detectors(testReaderDetector()), pkgWithInterface(i))
		assertShape(t, m.Meta(), "reader")
	})

	t.Run("non-matching function is left unstamped", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{Name: "Anything", Package: "x"}
		runAnnotate(t, shape.New().Detectors(testReaderDetector()), pkgWithFunction(fn))
		if shape.IsStamped(fn.Meta()) {
			t.Fatalf("non-matching function unexpectedly stamped: shape=%q", shape.Get(fn.Meta()))
		}
	})

	t.Run("package without a frontend marker is skipped", func(t *testing.T) {
		t.Parallel()
		fn := readerFunc("Get")
		s := store.New()
		// Package added without stamping the frontend marker
		// — the detector map has no entry under "" so the
		// callable is permissively skipped.
		if err := s.Nodes().AddPackage(&sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{fn},
		}); err != nil {
			t.Fatalf("AddPackage: %v", err)
		}
		runAnnotateAgainst(t, shape.New().Detectors(testReaderDetector()), s)
		if shape.IsStamped(fn.Meta()) {
			t.Fatalf("callable stamped despite missing frontend marker: shape=%q", shape.Get(fn.Meta()))
		}
	})

	t.Run("package with a frontend the detector does not list is skipped", func(t *testing.T) {
		t.Parallel()
		// Detector supports only "golang"; the package is tagged
		// as "rust" so dispatch falls through.
		fn := readerFunc("Get")
		s := store.New()
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{fn},
		}
		if err := s.Nodes().AddPackage(pkg); err != nil {
			t.Fatalf("AddPackage: %v", err)
		}
		frontendMarker.Set(pkg.EnsureMeta(), "rust", "test")
		runAnnotateAgainst(t, shape.New().Detectors(testReaderDetector()), s)
		if shape.IsStamped(fn.Meta()) {
			t.Fatalf("callable stamped despite unsupported frontend: shape=%q", shape.Get(fn.Meta()))
		}
	})

	t.Run("first-detector-wins when two detectors both match", func(t *testing.T) {
		t.Parallel()
		first := shape.Detector{
			Name: "first",
			Detect: map[string]shape.DetectFunc{
				"golang": func(sdk.Node) (shape.Match, bool) {
					return shape.Match{}, true
				},
			},
		}
		second := shape.Detector{
			Name: "second",
			Detect: map[string]shape.DetectFunc{
				"golang": func(sdk.Node) (shape.Match, bool) {
					return shape.Match{}, true
				},
			},
		}
		fn := &sdk.Function{Name: "X", Package: "x"}
		runAnnotate(t, shape.New().Detectors(first, second), pkgWithFunction(fn))
		assertShape(t, fn.Meta(), "first")
	})

	t.Run("Match.Shape explicit value overrides the Detector.Name default", func(t *testing.T) {
		t.Parallel()
		det := shape.Detector{
			Name: "writer",
			Detect: map[string]shape.DetectFunc{
				"golang": func(sdk.Node) (shape.Match, bool) {
					return shape.Match{Shape: "appendWriter"}, true
				},
			},
		}
		fn := &sdk.Function{Name: "X", Package: "x"}
		runAnnotate(t, shape.New().Detectors(det), pkgWithFunction(fn))
		assertShape(t, fn.Meta(), "appendWriter")
	})

	t.Run("higher Detector.Priority wins regardless of registration order", func(t *testing.T) {
		t.Parallel()
		low := shape.Detector{
			Name: "low", Priority: 100,
			Detect: map[string]shape.DetectFunc{
				"golang": func(sdk.Node) (shape.Match, bool) {
					return shape.Match{}, true
				},
			},
		}
		high := shape.Detector{
			Name: "high", Priority: 900,
			Detect: map[string]shape.DetectFunc{
				"golang": func(sdk.Node) (shape.Match, bool) {
					return shape.Match{}, true
				},
			},
		}
		fn := &sdk.Function{Name: "X", Package: "x"}
		// Register low first, high second: priority sort puts
		// high in front, so the stamp is "high".
		runAnnotate(t, shape.New().Detectors(low, high), pkgWithFunction(fn))
		assertShape(t, fn.Meta(), "high")
	})

	t.Run("Match.StringStamps and ListStamps land alongside the universal triple", func(t *testing.T) {
		t.Parallel()
		extraKey := sdk.EnsureKey("shape.test.extra", sdk.StringParser)
		listKey := sdk.EnsureKey("shape.test.list", sdk.StringListParser)
		det := shape.Detector{
			Name: "test",
			Detect: map[string]shape.DetectFunc{
				"golang": func(sdk.Node) (shape.Match, bool) {
					return shape.Match{
						ValueType: "v",
						StringStamps: []shape.StringStamp{
							{Key: extraKey, Value: "extra-value"},
						},
						ListStamps: []shape.ListStamp{
							{Key: listKey, Value: []string{"a", "b"}},
						},
					}, true
				},
			},
		}
		fn := &sdk.Function{Name: "X", Package: "x"}
		runAnnotate(t, shape.New().Detectors(det), pkgWithFunction(fn))

		if got, _ := extraKey.Get(fn.Meta()); got != "extra-value" {
			t.Fatalf("extra string stamp = %q, want %q", got, "extra-value")
		}
		got, _ := listKey.Get(fn.Meta())
		if !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Fatalf("list stamp = %v, want [a b]", got)
		}
	})
}

// TestPlugin_DetectorCascade pins the cascade's skip semantics: a
// detector that does not claim a callable hands the turn to the
// next one instead of ending dispatch.
//
// Both subtests exist to kill a `continue` -> `break` mutant in
// [shape.Plugin]'s detector loop — one at the frontend-lookup skip,
// one at the declined-match skip. Either mutation makes the FIRST
// detector that passes on a callable abort the whole cascade, so
// every lower-priority detector goes dead. That is the defect this
// repo already shipped: the reader detector never won dispatch
// because a higher-priority sibling swallowed its inputs, and a
// green suite reported nothing for months (see
// detectors/reachability_test.go). A one-detector fixture cannot
// see it — the assertion needs two, where the first declines and
// the second must still be reached, which is why these do not fold
// into TestPlugin_DetectorDispatch's single-detector cases.
func TestPlugin_DetectorCascade(t *testing.T) {
	t.Parallel()

	t.Run("a detector that declines does not stop the ones below it", func(t *testing.T) {
		t.Parallel()
		declining := shape.Detector{
			Name: "declining", Priority: 900,
			Detect: map[string]shape.DetectFunc{
				"golang": func(sdk.Node) (shape.Match, bool) {
					return shape.Match{}, false
				},
			},
		}
		accepting := shape.Detector{
			Name: "accepting", Priority: 100,
			Detect: map[string]shape.DetectFunc{
				"golang": func(sdk.Node) (shape.Match, bool) {
					return shape.Match{ValueType: "Article"}, true
				},
			},
		}
		fn := &sdk.Function{Name: "X", Package: "x"}
		runAnnotate(t, shape.New().Detectors(declining, accepting), pkgWithFunction(fn))

		assertShape(t, fn.Meta(), "accepting")
		assertMeta(t, fn.Meta(), shape.MetaValueType, "Article")
		assertStampedBy(t, fn.Meta(), shape.MetaShape, shape.PluginName+".accepting")
	})

	t.Run("a detector with no function for this frontend does not stop the ones below it", func(t *testing.T) {
		t.Parallel()
		// The protobuf-only detector accepts everything it is
		// handed, so a stamp of "protobufOnly" would prove it was
		// invoked for a Go callable; absence of that stamp plus a
		// "golangOnly" one proves the cascade stepped over it.
		protobufOnly := shape.Detector{
			Name: "protobufOnly", Priority: 900,
			Detect: map[string]shape.DetectFunc{
				"protobuf": func(sdk.Node) (shape.Match, bool) {
					return shape.Match{}, true
				},
			},
		}
		golangOnly := shape.Detector{
			Name: "golangOnly", Priority: 100,
			Detect: map[string]shape.DetectFunc{
				"golang": func(sdk.Node) (shape.Match, bool) {
					return shape.Match{ValueType: "Article"}, true
				},
			},
		}
		fn := &sdk.Function{Name: "X", Package: "x"}
		runAnnotate(t, shape.New().Detectors(protobufOnly, golangOnly), pkgWithFunction(fn))

		assertShape(t, fn.Meta(), "golangOnly")
		assertMeta(t, fn.Meta(), shape.MetaValueType, "Article")
		assertStampedBy(t, fn.Meta(), shape.MetaShape, shape.PluginName+".golangOnly")
	})
}

// TestPlugin_DirectiveScan pins the directive scan's skip
// semantics: a directive the shape scan does not own is stepped
// over, not read as the end of the list.
//
// Exists to kill a `continue` -> `break` mutant on the non-shape
// guard inside the plugin's `+gen:shape` directive scan. With
// `break`, a callable whose directive list opens with
// `+gen:contract` loses its explicit `+gen:shape` override and
// falls back to detector inference — the override is not dropped
// loudly, it is replaced by a plausible wrong shape. Directive
// order belongs to the author, so shape-second is ordinary source.
// The fixture therefore puts a foreign directive FIRST and
// registers a detector that infers a different shape, so the mutant
// surfaces as the wrong stamp rather than as a missing one.
func TestPlugin_DirectiveScan(t *testing.T) {
	t.Parallel()

	t.Run("a foreign directive ahead of +gen:shape does not hide the override", func(t *testing.T) {
		t.Parallel()
		fn := readerFunc("Find")
		fn.DirectiveList = []*sdk.Directive{
			{
				Name: shape.ContractDirectiveName,
				Args: []string{"persister"},
				KV:   map[string]string{"role": "reader"},
			},
			{Name: shape.DirectiveName, Args: []string{"lifecycle"}},
		}
		p := shape.New().
			Detectors(testReaderDetector()).
			Contracts(shape.Contract{Name: "persister", Roles: []string{"reader", "writer"}})
		runAnnotate(t, p, pkgWithFunction(fn))

		// The contract stamp proves the leading directive really is
		// the foreign one the scan had to step over — without it a
		// green result would only say the list was scanned, not that
		// anything preceded the override.
		assertMeta(t, fn.Meta(), shape.ContractRoleKey("persister"), "reader")
		assertShape(t, fn.Meta(), "lifecycle")
		assertStampedBy(t, fn.Meta(), shape.MetaShape, shape.PluginName+".directive")
	})

	// Closes the last `continue` -> `break` site in matchFromDirective,
	// which sat outside every agent's assignment in the cascade audit
	// and which no fixture in this package reached: nothing anywhere
	// constructed a nameless +gen:shape ahead of a named one.
	//
	// A nameless directive is legal to construct — the schema marks
	// the positional Required, so the parser rejects it, but plugin
	// code and other annotators build sdk.Directive values
	// directly, which is the same path the negation guards defend.
	// Under `break` the nameless entry aborts the scan, the explicit
	// override is lost, and detection quietly substitutes an inferred
	// shape. As with the foreign-directive case, the damage is a
	// plausible wrong answer rather than an absent one.
	t.Run("a nameless +gen:shape does not discard the named one after it", func(t *testing.T) {
		t.Parallel()
		fn := readerFunc("Find")
		fn.DirectiveList = []*sdk.Directive{
			{Name: shape.DirectiveName},
			{Name: shape.DirectiveName, Args: []string{"lifecycle"}},
		}
		p := shape.New().Detectors(testReaderDetector())
		runAnnotate(t, p, pkgWithFunction(fn))

		// "reader" is what the detector would infer if the override
		// were lost, so asserting the value — not merely that a stamp
		// exists — is what separates the mutant from the fix.
		assertShape(t, fn.Meta(), "lifecycle")
		assertStampedBy(t, fn.Meta(), shape.MetaShape, shape.PluginName+".directive")
	})
}

// runAnnotate wires the supplied package into a fresh store, runs
// the plugin's Annotate, and fails the test on error. Stamps the
// "golang" frontend marker on the package so the default test
// detectors dispatch.
func runAnnotate(t *testing.T, p *shape.Plugin, pkg *sdk.Package) {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")
	runAnnotateAgainst(t, p, s)
}

// runAnnotateAgainst runs p.Annotate against s with a fresh
// AnnotatorContext. Used directly by tests that want to control
// the package's frontend marker (or omit it entirely).
func runAnnotateAgainst(t *testing.T, p *shape.Plugin, s *sdk.Store) {
	t.Helper()
	if err := p.Annotate(newAnnotatorContext(t, s)); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
}

// runAnnotateDiags runs p over a package holding fn and returns every
// diagnostic raised, for tests asserting that an authoring mistake is
// reported rather than only that it stamped nothing.
func runAnnotateDiags(t *testing.T, p *shape.Plugin, pkg *sdk.Package) []sdk.Diag {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")
	sink := diag.New()
	ctx := &sdk.AnnotatorContext{Store: s, Reader: store.NewReader(s), Diag: sink}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	return sink.Diagnostics()
}

// newAnnotatorContext returns a context wrapping s with a fresh
// reader and an empty diagnostic sink.
func newAnnotatorContext(t *testing.T, s *sdk.Store) *sdk.AnnotatorContext {
	t.Helper()
	return &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
}

// pkgWithFunction returns a single-function test package named "x".
func pkgWithFunction(fn *sdk.Function) *sdk.Package {
	return &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{fn},
	}
}

// pkgWithStruct returns a single-struct test package named "x".
func pkgWithStruct(s *sdk.Struct) *sdk.Package {
	return &sdk.Package{
		Name: "x", Path: "x",
		Structs: []*sdk.Struct{s},
	}
}

// pkgWithInterface returns a single-interface test package named "x".
func pkgWithInterface(i *sdk.Interface) *sdk.Package {
	return &sdk.Package{
		Name: "x", Path: "x",
		Interfaces: []*sdk.Interface{i},
	}
}

// buildStoreWithReader builds a store containing a single function
// matching the reader signature so the idempotency test has a
// detector-stamped callable to compare across passes.
func buildStoreWithReader(t *testing.T) *sdk.Store {
	t.Helper()
	s := store.New()
	pkg := pkgWithFunction(readerFunc("Get"))
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")
	return s
}

// readerFunc returns a [sdk.Function] matching the canonical Go
// reader signature: `func(ctx context.Context, id string) (V, error)`.
func readerFunc(name string) *sdk.Function {
	return &sdk.Function{
		Name: name, Package: "x",
		Params: []*sdk.Param{
			{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
			{Name: "id", Type: &sdk.TypeRef{Name: "string"}},
		},
		Returns: sdk.AnonReturns(
			&sdk.TypeRef{Name: "Article", Package: "x"},
			&sdk.TypeRef{Name: "error"},
		),
	}
}

// readerMethod returns a [sdk.Method] matching the canonical Go
// reader signature, used to test method-bound dispatch.
func readerMethod(name string) *sdk.Method {
	fn := readerFunc(name)
	return &sdk.Method{
		Name:    fn.Name,
		Params:  fn.Params,
		Returns: fn.Returns,
	}
}

// testReaderDetector returns a Detector that recognises the reader
// pattern in Go callables. The body deliberately composes the
// shared Go query vocabulary so the dispatch path is exercised the
// way a real detector writes it.
func testReaderDetector() shape.Detector {
	return shape.Detector{
		Name: "reader",
		Detect: map[string]shape.DetectFunc{
			golang.Language: func(n sdk.Node) (shape.Match, bool) {
				params, returns := golang.Callable(n)
				if !golang.HasContext(params) || !golang.HasError(returns) {
					return shape.Match{}, false
				}
				rest := golang.StripContext(params)
				vals := golang.StripErrorTypes(returns)
				if len(rest) != 1 || len(vals) != 1 {
					return shape.Match{}, false
				}
				return shape.Match{
					KeyType:   golang.QName(rest[0].Type),
					ValueType: golang.QName(vals[0]),
				}, true
			},
		},
	}
}

// snapshotShapeMeta captures every shape stamp on every callable
// in s into a deterministic map keyed by "kind:qname:meta-name".
// Used by the idempotency test to compare meta state across runs.
func snapshotShapeMeta(s *sdk.Store) map[string]string {
	out := make(map[string]string)
	collect := func(bag *sdk.Bag, key string) {
		if v, ok := shape.MetaShape.Get(bag); ok {
			out[key+":shape"] = v
		}
		if v, ok := shape.MetaKeyType.Get(bag); ok {
			out[key+":key"] = v
		}
		if v, ok := shape.MetaValueType.Get(bag); ok {
			out[key+":value"] = v
		}
	}
	for _, fn := range s.Nodes().Functions().Items() {
		collect(fn.Meta(), "fn:"+fn.QName())
	}
	for _, m := range s.Nodes().Methods().Items() {
		collect(m.Meta(), "method:"+m.Name)
	}
	return out
}

// assertShape fails the test when MetaShape on bag does not equal
// want. Used by the directive-override and detector-dispatch
// tests as the primary positive-path assertion.
func assertShape(t *testing.T, bag *sdk.Bag, want string) {
	t.Helper()
	got, ok := shape.MetaShape.Get(bag)
	if !ok {
		t.Fatalf("MetaShape unstamped; want %q", want)
	}
	if got != want {
		t.Fatalf("MetaShape = %q, want %q", got, want)
	}
}

// assertMeta fails the test when k on bag does not resolve to want.
// Generic helper used for KeyType / ValueType assertions.
func assertMeta(t *testing.T, bag *sdk.Bag, k sdk.Key[string], want string) {
	t.Helper()
	got, ok := k.Get(bag)
	if !ok {
		t.Fatalf("%s unstamped; want %q", k.Name(), want)
	}
	if got != want {
		t.Fatalf("%s = %q, want %q", k.Name(), got, want)
	}
}

// assertStampedBy fails when the winning provenance for k on bag
// names a setter other than want. The cascade tests need it because
// a shape name alone cannot say WHICH detector produced the stamp:
// two detectors can legitimately stamp the same name, and the
// attribution is the only per-detector fingerprint the meta bag
// carries.
func assertStampedBy(t *testing.T, bag *sdk.Bag, k sdk.Key[string], want string) {
	t.Helper()
	p, ok := bag.Winning(k.Name())
	if !ok {
		t.Fatalf("%s unstamped; want a stamp set by %q", k.Name(), want)
	}
	if p.SetBy != want {
		t.Fatalf("%s set by %q, want %q", k.Name(), p.SetBy, want)
	}
}

// langGolang is the language tag backend/golang queries a
// [sdk.TemplateProvider] with. Spelled once here because the
// funcmap contribution is only reachable under this exact key —
// a typo would silently probe a language nothing answers for and
// turn the assertion into a no-op.
const langGolang = "golang"

// TestShapeConformance runs the framework conformance suite over
// the shape annotator.
//
// The three shape plugins previously ran no conformance suite at
// all, which is how each came to declare Priority() without the rest
// of sdk.CapabilityProvider — satisfying nothing, so the pipeline
// ignored the declared ordering and ran all three in the default
// bucket, in registration order. The suite's completeness check
// catches that shape directly; wiring the plugins into the suite is
// what makes the check reach them.
func TestShapeConformance(t *testing.T) {
	t.Parallel()
	plugintest.RunSuite(t, shape.New())
}

// Every declaration that can carry a method is walked.
//
// Go attaches methods to any defined type, so the set is wider than
// the obvious two: a `type Weekday int` carries them on an alias, and
// the same declaration moves them onto an enum the moment a const
// block coalesces it. A walk covering only structs and interfaces
// leaves those callables unclassified with nothing to say they were
// skipped — which is invisible until a consumer asks the graph a
// question the missing stamp was meant to answer.
func TestAnnotateReachesEveryMethodCarrier(t *testing.T) {
	t.Parallel()

	// A nullary bare-error method, which the lifecycle-free reader
	// detector below classifies wherever it is declared.
	nullaryError := func(m *gofixture.MethodBuilder) {
		m.Return(gofixture.Named("error"))
	}

	fixture := gofixture.New().
		Struct("Store", func(s *gofixture.StructBuilder) {
			s.Method("OnStruct", nullaryError)
		}).
		Interface("Port", func(i *gofixture.InterfaceBuilder) {
			i.Method("OnInterface", nullaryError)
		}).
		Enum("Status", func(e *gofixture.EnumBuilder) {
			e.Underlying(gofixture.Named("int"))
			e.Variant("StatusActive", "0")
			e.Method("OnEnum", nullaryError)
		}).
		Alias("Weekday", func(a *gofixture.AliasBuilder) {
			a.Target(gofixture.Named("int"))
			a.Method("OnAlias", nullaryError)
		})

	s := fixture.Build()
	pkg := fixture.PackageNode()
	sdk.MetaFrontend.Set(pkg.EnsureMeta(), golang.Language, "test")

	p := shape.New().Detectors(shape.Detector{
		Name: "closer",
		Detect: map[string]shape.DetectFunc{
			golang.Language: func(n sdk.Node) (shape.Match, bool) {
				params, returns := golang.Callable(n)
				return shape.Match{}, len(params) == 0 && golang.HasError(returns)
			},
		},
	})
	ctx := &sdk.AnnotatorContext{Store: s, Reader: store.NewReader(s), Diag: diag.New()}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}

	for _, tc := range []struct {
		carrier string
		method  string
	}{
		{"struct", "OnStruct"},
		{"interface", "OnInterface"},
		{"enum", "OnEnum"},
		{"named type", "OnAlias"},
	} {
		t.Run("stamps a method on a "+tc.carrier, func(t *testing.T) {
			t.Parallel()
			m := methodNamed(t, pkg, tc.method)
			if got := shape.Get(m.Meta()); got != "closer" {
				t.Errorf("%s.Meta() shape = %q, want %q — the walk skipped this carrier",
					tc.method, got, "closer")
			}
		})
	}
}

// methodNamed finds a method by name across every carrier a package
// holds, so the lookup cannot itself encode the omission under test.
func methodNamed(t *testing.T, pkg *sdk.Package, name string) *sdk.Method {
	t.Helper()
	var all []*sdk.Method
	for _, s := range pkg.Structs {
		all = append(all, s.Methods...)
	}
	for _, i := range pkg.Interfaces {
		all = append(all, i.Methods...)
	}
	for _, e := range pkg.Enums {
		all = append(all, e.Methods...)
	}
	for _, a := range pkg.Aliases {
		all = append(all, a.Methods...)
	}
	for _, m := range all {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("fixture declares no method %q", name)
	return nil
}
