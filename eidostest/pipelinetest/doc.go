// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package pipelinetest wraps the production [pipeline.Pipeline] with
// ergonomics tuned for test code: a synthetic frontend that accepts
// pre-built [node.Package] values, an in-memory sink whose contents
// are exposed for per-file assertions, and golden-file diffing with
// a `-update-golden` flag.
//
// # Three-layer test surface
//
// The eidostest surface has three layers, each appropriate to a
// different scope of test:
//
//   - Unit: drive a single plugin's Annotate / Generate using a
//     [storefixture.Builder]-produced [store.Store]. No pipeline.
//   - Synthetic pipeline: drive multiple phases with a fully wired
//     pipeline whose frontend is [FromNodes]. This package.
//   - Full pipeline: drive a production frontend against testdata
//     fixtures. Provided by [frontendtest].
//
// # Synthetic pipeline shape
//
//	p := pipelinetest.New(t).
//	    WithFrontend(pipelinetest.FromNodes(storefixture.New().
//	        Struct("User", func(s *storefixture.StructBuilder) {
//	            s.Directive(storefixture.Directive("repo"))
//	            s.Field("ID", storefixture.Named("string"), nil)
//	        }).PackageNode())).
//	    WithGenerator(repogen.New()).
//	    WithBackend(backend.New()).
//	    Build().
//	    Run()
//
//	p.AssertFile("user_repo_gen.go").
//	    Contains("type UserRepo struct").
//	    MatchesGolden("testdata/user_repo.golden.go")
//
// # Failure semantics
//
// Build- and Run-time errors fail the test via [testing.TB.Fatalf].
// Assertion failures call [testing.TB.Errorf] so a single failing
// expectation does not stop subsequent assertions in the same chain
// from reporting. Tests that need stop-on-first-failure semantics
// chain a [testing.TB.FailNow] explicitly.
//
// # Update-golden flag
//
// The package registers a single `-update-golden` flag. Run the test
// binary with `-update-golden` to rewrite golden fixtures from the
// current run's output. The rewrite is atomic (temp + rename) so a
// failed test does not leave a partial golden on disk.
package pipelinetest
