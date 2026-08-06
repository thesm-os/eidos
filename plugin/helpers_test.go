// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin_test

import (
	"errors"
	"io/fs"
	"testing"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/priority"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

// assertNoError fails the test if err is non-nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// stubKey is a package-level meta.Key used by stubAnnotator. Declared
// once so the global registry never sees re-registration on -count > 1.
var stubKey = meta.NewKey("plugin.stub.annotated", meta.BoolParser)

// stubFrontend implements [plugin.Frontend]. Load adds a fixed
// package containing one struct so downstream stubs have something
// to operate on.
type stubFrontend struct{ name string }

func (f *stubFrontend) Name() string { return f.name }
func (*stubFrontend) Load(ctx *plugin.FrontendContext) error {
	return ctx.Store.Nodes().AddPackage(&node.Package{
		Name: "x", Path: "x",
		Structs: []*node.Struct{{Name: "User", Package: "x"}},
	})
}

// stubAnnotator implements [plugin.Annotator]. Annotate stamps
// stubKey on every struct in the source store.
type stubAnnotator struct{ name string }

func (a *stubAnnotator) Name() string { return a.name }
func (a *stubAnnotator) Annotate(ctx *plugin.AnnotatorContext) error {
	ctx.Reader.Structs().Each(func(s *node.Struct) {
		stubKey.Set(s.EnsureMeta(), true, a.name)
	})
	return nil
}

// stubGenerator implements [plugin.Generator]. Generate produces an
// emit.Package mirroring every annotated struct with a Target so
// the backend has something to render.
type stubGenerator struct{ name string }

func (g *stubGenerator) Name() string { return g.name }
func (*stubGenerator) Generate(ctx *plugin.GeneratorContext) error {
	target := emit.Target{Dir: "out", Filename: "x_gen.go", Package: "x"}
	pkg := &emit.Package{Name: "x", Path: "x", Dir: "out"}
	ctx.Reader.Structs().
		Where(store.WithMetaKey[*node.Struct](stubKey)).
		Each(func(s *node.Struct) {
			pkg.Structs = append(pkg.Structs, &emit.Struct{
				Name: s.Name, Package: "x", Target: target,
			})
		})
	return ctx.Store.Emit().AddPackage(pkg)
}

// stubBackend implements [plugin.Backend]. Render writes one stub
// payload per emit.Struct through the sink, keyed by the struct's
// target.
type stubBackend struct{ name string }

func (b *stubBackend) Name() string   { return b.name }
func (*stubBackend) Language() string { return "stub" }
func (*stubBackend) Render(ctx *plugin.BackendContext) error {
	var firstErr error
	ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
		if err := ctx.Sink.Write(s.Target, []byte("stub:"+s.Name)); err != nil && firstErr == nil {
			firstErr = err
		}
	})
	return firstErr
}

// memSink implements [sink.Sink] with an in-memory map keyed by the
// stringified target path. Suitable for tests.
type memSink struct {
	files map[string][]byte
}

func newMemSink() *memSink { return &memSink{files: map[string][]byte{}} }

func (m *memSink) Write(t emit.Target, body []byte) error {
	m.files[t.JoinPath()] = body
	return nil
}

// failingSink returns a fixed error from every Write call — used by
// tests that exercise backend error propagation.
type failingSink struct{ err error }

func (f *failingSink) Write(emit.Target, []byte) error { return f.err }

// stubCapability is a stub implementing [plugin.CapabilityProvider]
// alongside [plugin.Plugin].
type stubCapability struct {
	name     string
	priority priority.Priority
	provides []string
	requires []string
}

func (c *stubCapability) Name() string                { return c.name }
func (c *stubCapability) Priority() priority.Priority { return c.priority }
func (c *stubCapability) Provides() []string          { return c.provides }
func (c *stubCapability) Requires() []string          { return c.requires }

// stubTemplate is a stub implementing [plugin.TemplateProvider]
// alongside [plugin.Plugin]. Templates returns the supplied fs.FS;
// TemplateFuncs / TemplateOverrides return the supplied funcmaps.
type stubTemplate struct {
	name      string
	tmplFS    fs.FS
	funcs     template.FuncMap
	overrides template.FuncMap
}

func (s *stubTemplate) Name() string                                { return s.name }
func (s *stubTemplate) Templates(_ string) (fs.FS, bool)            { return s.tmplFS, s.tmplFS != nil }
func (s *stubTemplate) TemplateFuncs(_ string) template.FuncMap     { return s.funcs }
func (s *stubTemplate) TemplateOverrides(_ string) template.FuncMap { return s.overrides }

// stubOptionsConfig is the typed options struct stubOptionsProvider
// declares via opt.Reflect.
type stubOptionsConfig struct {
	Output string `eidos:"output,required"`
}

// stubOptionsProvider is a stub implementing [plugin.OptionsProvider]
// alongside [plugin.Plugin].
type stubOptionsProvider struct {
	name string
	opts stubOptionsConfig
}

func (s *stubOptionsProvider) Name() string                   { return s.name }
func (*stubOptionsProvider) OptionsSchema() opt.Schema        { return opt.Reflect(stubOptionsConfig{}) }
func (s *stubOptionsProvider) SetOptions(o opt.Options) error { return o.Decode(&s.opts) }

// errSinkBoom is the sentinel returned by [failingSink].
var errSinkBoom = errors.New("plugin: sink boom (test)")

// makeStubReader returns a fresh Reader bound to a freshly-loaded
// stub frontend. Convenience for tests that want to skip the manual
// "construct frontend, call Load" boilerplate.
func makeStubReader(t *testing.T) (*store.Store, *store.Reader) {
	t.Helper()
	s := store.New()
	d := diag.New()
	fe := &stubFrontend{name: "stub-fe"}
	assertNoError(t, fe.Load(&plugin.FrontendContext{
		Store:   s,
		Diag:    d,
		Pattern: "input",
	}))
	return s, store.NewReader(s)
}

// _ asserts that [sink.Sink] is satisfied by both stub sinks at
// compile time. Failure here surfaces as a build-time error, not a
// runtime test failure, which is the appropriate granularity for an
// interface-contract assertion.
var (
	_ sink.Sink = (*memSink)(nil)
	_ sink.Sink = (*failingSink)(nil)
)
