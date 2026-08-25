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
//   - Unit: drive a single plugin's Annotate / Generate against a
//     [store.Store] the test populates itself, through whichever
//     fixture its target language provides. No pipeline.
//   - Synthetic pipeline: drive multiple phases with a fully wired
//     pipeline whose frontend is [FromNodes]. This package.
//   - Full pipeline: drive a production frontend against testdata
//     fixtures. Provided by [frontendtest].
//
// # Synthetic pipeline shape
//
//	p := pipelinetest.New(t).
//	    WithFrontend(pipelinetest.FromNodes(users)).
//	    WithGenerator(repogen.New()).
//	    WithBackend(backend.New()).
//	    Build().
//	    Run()
//
//	p.AssertFile("user_repo.go").
//	    Contains("type UserRepo struct").
//	    MatchesGolden("testdata/user_repo.golden.go")
//
// The rendered basename is the origin's basename plus the emitting
// plugin's declared filename suffix — `user.go` plus repogen's
// `_repo.go`. A plugin's suffix, not the harness, decides it.
//
// `users` is a [node.Package] — a struct named User carrying the
// `repo` directive. Where it came from is the caller's business:
// [FromNodes] takes nodes and asks nothing about the language that
// produced them, so a plugin targeting Go builds it through
// `lang/golang/golangtest/gofixture` and one targeting another
// language through that language's equivalent.
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
//
// # What makes a golden portable
//
// A golden holds whole rendered bytes, and rendered bytes open with
// a header envelope whose fields come from the run rather than from
// the plugin under test. Three of them vary by invocation, and each
// has an option that pins it:
//
//   - "Command:" — the pipeline's rendering of os.Args unless
//     pinned. Under `go test` that is the test binary's own flag
//     set, including the per-machine `-test.testlogfile` path and
//     `-update-golden` itself, so the run that writes a baseline and
//     the run that asserts against it can never agree. [New] pins it
//     to [DefaultCommand] for exactly this reason; [Builder.WithCommand]
//     overrides.
//   - "Source:" — each entity's origin path, rendered relative to
//     the source root. Unset, the source root is os.Getwd, which is
//     the test binary's package directory. Fixtures carrying
//     absolute origin paths pin it with [Builder.WithSourceRoot].
//   - The generated-file marker and provenance footer carry the
//     brand; [Builder.WithBrand] pins it.
//
// Everything else in the envelope — the plugin attribution list and
// the body hash — is derived from the run's inputs and is stable by
// construction.
package pipelinetest
