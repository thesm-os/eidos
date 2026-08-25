// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipelinetest_test

import (
	"fmt"
	"strings"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// Example runs the synthetic pipeline end to end: a [FromNodes]
// frontend injects a hand-built source package, a generator emits
// one decl per source struct, the pipeline's Layout phase resolves
// where that decl belongs, and a backend renders it into the
// harness's in-memory sink. The assertion chain then reads the
// captured file back.
//
// This is the middle layer of the three-layer surface the package
// docblock describes — enough pipeline to exercise routing and a
// real render pass, no source parsing. A plugin unit test that only
// needs a populated store stops at a language fixture; a test that needs
// a production frontend against real source graduates to
// [frontendtest].
//
// The backend here is a deliberately tiny local one rather than the
// framework's Go backend. eidostest's module requires only the
// framework core, and reaching for `backend/golang` would add a
// module edge back to a module that already depends on eidostest.
// Nothing in the example turns on which backend runs: the harness
// surface being documented is the frontend / generator / assertion
// chain, and a real test swaps in `golang.New()` at the same
// [Builder.WithBackend] call.
func Example() {
	// The source position is load-bearing: the default layout routes
	// generated output alongside the file its origin came from, so a
	// struct with no File has nowhere to land.
	origin := &node.Struct{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: "users/user.go"}},
		Name:     "User",
		Package:  "example.com/users",
	}
	src := &node.Package{
		Name:    "users",
		Path:    "example.com/users",
		Structs: []*node.Struct{origin},
	}

	p := pipelinetest.New(&exampleTB{}).
		WithFrontend(pipelinetest.FromNodes(src)).
		WithGenerator(&greeterGenerator{}).
		WithBackend(&listingBackend{}).
		Build().
		Run()

	p.AssertFileCount(1)
	rendered := p.AssertFile("user_greeter.go").
		Contains("type UserGreeter struct").
		Contains("Greeting string")

	fmt.Println("target:", rendered.Target().JoinPath())
	fmt.Print(rendered.String())

	// Output:
	// target: users/user_greeter.go
	// package users
	//
	// type UserGreeter struct {
	// 	Greeting string
	// }
}

// greeterGenerator emits one `<Name>Greeter` struct per source
// struct. It is the smallest generator that still exercises the
// parts of the pipeline the example is about: it anchors its output
// to a source node, so Layout has something to route against, and it
// declares a filename suffix, so the rendered basename is the
// plugin's choice rather than a framework default.
type greeterGenerator struct{}

// Name returns the plugin identifier the framework attributes the
// emitted decls to.
func (*greeterGenerator) Name() string { return "greetergen" }

// Outputs declares the one file the generator produces per origin.
// The language argument is the backend's [plugin.Backend.Language];
// returning nil for anything else is how a plugin declines to
// contribute to a target it does not know how to write.
func (*greeterGenerator) Outputs(lang string) []plugin.Output {
	if lang != exampleLanguage {
		return nil
	}
	return []plugin.Output{{Suffix: "_greeter.go"}}
}

// Generate emits one struct per source struct, anchored to its
// origin so the Layout phase resolves the target.
func (g *greeterGenerator) Generate(ctx *plugin.GeneratorContext) error {
	for _, src := range ctx.Reader.Structs().Slice() {
		pkg, err := builder.For(g.Name()).
			Anchor(src).
			Struct(src.Name+"Greeter", func(s *builder.StructBuilder) {
				s.Field("Greeting", emit.Builtin("string"), nil)
			}).
			Build()
		if err != nil {
			return fmt.Errorf("greetergen: build: %w", err)
		}
		if err := ctx.Store.Emit().AddPackage(pkg); err != nil {
			return fmt.Errorf("greetergen: AddPackage: %w", err)
		}
	}
	return nil
}

// exampleLanguage is the target-language identifier the example's
// backend declares and its generator matches on.
const exampleLanguage = "golang"

// listingBackend renders each emit struct as a bare type
// declaration at the [emit.Target] Layout resolved for it.
//
// It stands in for the framework's Go backend so the example stays
// inside eidostest's dependency footprint. The one property the
// example depends on is byte-determinism: emit buckets iterate in
// insertion order, so a fixed input renders the same bytes on every
// run and the `// Output:` block above is a real assertion rather
// than a hopeful one.
type listingBackend struct{}

// Name returns the backend's plugin identifier.
func (*listingBackend) Name() string { return "backend.listing" }

// Language returns the identifier plugins match on when deciding
// what to contribute.
func (*listingBackend) Language() string { return exampleLanguage }

// Render writes one file per emit struct at its resolved target.
func (*listingBackend) Render(ctx *plugin.BackendContext) error {
	for _, s := range ctx.Store.Emit().Structs().Items() {
		var body strings.Builder
		fmt.Fprintf(&body, "package %s\n\ntype %s struct {\n", s.Target.Package, s.Name)
		for _, f := range s.Fields {
			fmt.Fprintf(&body, "\t%s string\n", f.Name)
		}
		body.WriteString("}\n")
		if err := ctx.Sink.Write(s.Target, []byte(body.String())); err != nil {
			return fmt.Errorf("backend.listing: write %s: %w", s.Target.JoinPath(), err)
		}
	}
	return nil
}
