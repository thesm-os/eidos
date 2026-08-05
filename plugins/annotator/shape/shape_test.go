// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape_test

import (
	"reflect"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/priority"
	"go.thesmos.sh/eidos/store"
)

// frontendMarker is the key the framework's frontends stamp on
// every produced package — the shape plugin reads it to pick the
// per-language detector. Declared in tests via [meta.EnsureKey] so
// the same canonical key is returned regardless of init order.
//
//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = meta.EnsureKey("frontend", meta.StringParser)

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

	t.Run("satisfies plugin.Annotator", func(t *testing.T) {
		t.Parallel()
		var _ plugin.Annotator = shape.New()
	})

	t.Run("satisfies plugin.DirectiveProvider", func(t *testing.T) {
		t.Parallel()
		var _ plugin.DirectiveProvider = shape.New()
	})

	t.Run("Priority is the shape-detection bucket", func(t *testing.T) {
		t.Parallel()
		if got, want := shape.New().Priority(), priority.AnnotatorShape; got != want {
			t.Fatalf("Priority() = %v, want %v", got, want)
		}
	})

	t.Run("Directives declares the +gen:shape and +gen:contract schemas", func(t *testing.T) {
		t.Parallel()
		schemas := shape.New().Directives()
		names := make(map[directive.Name]bool, len(schemas))
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
		fn := &node.Function{
			Name: "Custom", Package: "x",
			BaseNode: node.BaseNode{
				DirectiveList: []*directive.Directive{
					{Name: shape.DirectiveName, Args: []string{"lifecycle"}},
				},
			},
		}
		runAnnotate(t, shape.New(), pkgWithFunction(fn))
		assertShape(t, fn.Meta(), "lifecycle")
	})

	t.Run("kind= key carries the shape name when no positional", func(t *testing.T) {
		t.Parallel()
		fn := &node.Function{
			Name: "Custom", Package: "x",
			BaseNode: node.BaseNode{
				DirectiveList: []*directive.Directive{
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
		fn := &node.Function{
			Name: "Custom", Package: "x",
			BaseNode: node.BaseNode{
				DirectiveList: []*directive.Directive{
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
		fn := &node.Function{
			Name: "Custom", Package: "x",
			BaseNode: node.BaseNode{
				DirectiveList: []*directive.Directive{
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
		fn.DirectiveList = []*directive.Directive{
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
		s := &node.Struct{
			Name: "Repo", Package: "x",
			Methods: []*node.Method{m},
		}
		runAnnotate(t, shape.New().Detectors(testReaderDetector()), pkgWithStruct(s))
		assertShape(t, m.Meta(), "reader")
	})

	t.Run("matching method on an interface gets stamped via detector", func(t *testing.T) {
		t.Parallel()
		m := readerMethod("Get")
		i := &node.Interface{
			Name: "Repo", Package: "x",
			Methods: []*node.Method{m},
		}
		runAnnotate(t, shape.New().Detectors(testReaderDetector()), pkgWithInterface(i))
		assertShape(t, m.Meta(), "reader")
	})

	t.Run("non-matching function is left unstamped", func(t *testing.T) {
		t.Parallel()
		fn := &node.Function{Name: "Anything", Package: "x"}
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
		if err := s.Nodes().AddPackage(&node.Package{
			Name: "x", Path: "x",
			Functions: []*node.Function{fn},
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
		pkg := &node.Package{
			Name: "x", Path: "x",
			Functions: []*node.Function{fn},
		}
		if err := s.Nodes().AddPackage(pkg); err != nil {
			t.Fatalf("AddPackage: %v", err)
		}
		frontendMarker.Set(pkg.Meta(), "rust", "test")
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
				"golang": func(node.Node) (shape.Match, bool) {
					return shape.Match{}, true
				},
			},
		}
		second := shape.Detector{
			Name: "second",
			Detect: map[string]shape.DetectFunc{
				"golang": func(node.Node) (shape.Match, bool) {
					return shape.Match{}, true
				},
			},
		}
		fn := &node.Function{Name: "X", Package: "x"}
		runAnnotate(t, shape.New().Detectors(first, second), pkgWithFunction(fn))
		assertShape(t, fn.Meta(), "first")
	})

	t.Run("Match.Shape explicit value overrides the Detector.Name default", func(t *testing.T) {
		t.Parallel()
		det := shape.Detector{
			Name: "writer",
			Detect: map[string]shape.DetectFunc{
				"golang": func(node.Node) (shape.Match, bool) {
					return shape.Match{Shape: "appendWriter"}, true
				},
			},
		}
		fn := &node.Function{Name: "X", Package: "x"}
		runAnnotate(t, shape.New().Detectors(det), pkgWithFunction(fn))
		assertShape(t, fn.Meta(), "appendWriter")
	})

	t.Run("higher Detector.Priority wins regardless of registration order", func(t *testing.T) {
		t.Parallel()
		low := shape.Detector{
			Name: "low", Priority: 100,
			Detect: map[string]shape.DetectFunc{
				"golang": func(node.Node) (shape.Match, bool) {
					return shape.Match{}, true
				},
			},
		}
		high := shape.Detector{
			Name: "high", Priority: 900,
			Detect: map[string]shape.DetectFunc{
				"golang": func(node.Node) (shape.Match, bool) {
					return shape.Match{}, true
				},
			},
		}
		fn := &node.Function{Name: "X", Package: "x"}
		// Register low first, high second: priority sort puts
		// high in front, so the stamp is "high".
		runAnnotate(t, shape.New().Detectors(low, high), pkgWithFunction(fn))
		assertShape(t, fn.Meta(), "high")
	})

	t.Run("Match.StringStamps and ListStamps land alongside the universal triple", func(t *testing.T) {
		t.Parallel()
		extraKey := meta.EnsureKey("shape.test.extra", meta.StringParser)
		listKey := meta.EnsureKey("shape.test.list", meta.StringListParser)
		det := shape.Detector{
			Name: "test",
			Detect: map[string]shape.DetectFunc{
				"golang": func(node.Node) (shape.Match, bool) {
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
		fn := &node.Function{Name: "X", Package: "x"}
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

// runAnnotate wires the supplied package into a fresh store, runs
// the plugin's Annotate, and fails the test on error. Stamps the
// "golang" frontend marker on the package so the default test
// detectors dispatch.
func runAnnotate(t *testing.T, p *shape.Plugin, pkg *node.Package) {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.Meta(), "golang", "test")
	runAnnotateAgainst(t, p, s)
}

// runAnnotateAgainst runs p.Annotate against s with a fresh
// AnnotatorContext. Used directly by tests that want to control
// the package's frontend marker (or omit it entirely).
func runAnnotateAgainst(t *testing.T, p *shape.Plugin, s *store.Store) {
	t.Helper()
	if err := p.Annotate(newAnnotatorContext(t, s)); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
}

// newAnnotatorContext returns a context wrapping s with a fresh
// reader and an empty diagnostic sink.
func newAnnotatorContext(t *testing.T, s *store.Store) *plugin.AnnotatorContext {
	t.Helper()
	return &plugin.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
}

// pkgWithFunction returns a single-function test package named "x".
func pkgWithFunction(fn *node.Function) *node.Package {
	return &node.Package{
		Name: "x", Path: "x",
		Functions: []*node.Function{fn},
	}
}

// pkgWithStruct returns a single-struct test package named "x".
func pkgWithStruct(s *node.Struct) *node.Package {
	return &node.Package{
		Name: "x", Path: "x",
		Structs: []*node.Struct{s},
	}
}

// pkgWithInterface returns a single-interface test package named "x".
func pkgWithInterface(i *node.Interface) *node.Package {
	return &node.Package{
		Name: "x", Path: "x",
		Interfaces: []*node.Interface{i},
	}
}

// buildStoreWithReader builds a store containing a single function
// matching the reader signature so the idempotency test has a
// detector-stamped callable to compare across passes.
func buildStoreWithReader(t *testing.T) *store.Store {
	t.Helper()
	s := store.New()
	pkg := pkgWithFunction(readerFunc("Get"))
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.Meta(), "golang", "test")
	return s
}

// readerFunc returns a [node.Function] matching the canonical Go
// reader signature: `func(ctx context.Context, id string) (V, error)`.
func readerFunc(name string) *node.Function {
	return &node.Function{
		Name: name, Package: "x",
		Params: []*node.Param{
			{Name: "ctx", Type: &node.TypeRef{Name: "Context", Package: "context"}},
			{Name: "id", Type: &node.TypeRef{Name: "string"}},
		},
		Returns: node.AnonReturns(
			&node.TypeRef{Name: "Article", Package: "x"},
			&node.TypeRef{Name: "error"},
		),
	}
}

// readerMethod returns a [node.Method] matching the canonical Go
// reader signature, used to test method-bound dispatch.
func readerMethod(name string) *node.Method {
	fn := readerFunc(name)
	return &node.Method{
		Name:    fn.Name,
		Params:  fn.Params,
		Returns: fn.Returns,
	}
}

// testReaderDetector returns a Detector that recognises the reader
// pattern in Go callables. The body deliberately uses the
// package's lazy helpers so the helper API is exercised end-to-end
// through the dispatch path.
func testReaderDetector() shape.Detector {
	return shape.Detector{
		Name: "reader",
		Detect: map[string]shape.DetectFunc{
			"golang": func(n node.Node) (shape.Match, bool) {
				params, returns := shape.GoCallable(n)
				if !shape.GoHasContext(params) || !shape.GoHasError(returns) {
					return shape.Match{}, false
				}
				rest := shape.GoStripContext(params)
				vals := shape.GoStripError(returns)
				if len(rest) != 1 || len(vals) != 1 {
					return shape.Match{}, false
				}
				return shape.Match{
					KeyType:   shape.QName(rest[0].Type),
					ValueType: shape.QName(vals[0]),
				}, true
			},
		},
	}
}

// snapshotShapeMeta captures every shape stamp on every callable
// in s into a deterministic map keyed by "kind:qname:meta-name".
// Used by the idempotency test to compare meta state across runs.
func snapshotShapeMeta(s *store.Store) map[string]string {
	out := make(map[string]string)
	collect := func(bag *meta.Bag, key string) {
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
func assertShape(t *testing.T, bag *meta.Bag, want string) {
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
func assertMeta(t *testing.T, bag *meta.Bag, k meta.Key[string], want string) {
	t.Helper()
	got, ok := k.Get(bag)
	if !ok {
		t.Fatalf("%s unstamped; want %q", k.Name(), want)
	}
	if got != want {
		t.Fatalf("%s = %q, want %q", k.Name(), got, want)
	}
}

// TestShapeConformance runs the framework conformance suite over
// the shape annotator.
//
// The three shape plugins previously ran no conformance suite at
// all, which is how each came to declare Priority() without the rest
// of plugin.CapabilityProvider — satisfying nothing, so the pipeline
// ignored the declared ordering and ran all three in the default
// bucket, in registration order. The suite's completeness check
// catches that shape directly; wiring the plugins into the suite is
// what makes the check reach them.
func TestShapeConformance(t *testing.T) {
	t.Parallel()
	plugintest.RunSuite(t, shape.New())
}
