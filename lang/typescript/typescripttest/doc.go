// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package typescripttest asserts on the TypeScript a generator
// produced.
//
// It is a set of composable helpers, not a conformance suite. What a
// generator *should* emit is that generator's business — there is no
// fixed battery to run, and the one universal claim ("whatever a
// TypeScript generator emits must type-check") still cannot be
// checked without the hand-written module the output imports from,
// which only the fixture author has. So nothing here runs itself;
// every assertion is one the test author reaches for. [plugintest] is
// the conformance half, and this is deliberately the other kind.
//
// # What it is for
//
// A generator's tests answer three questions, in order.
//
// **Is what I emitted valid TypeScript?** [Generated.AssertParses]
// and [Source.AssertParses]. Without these, every other assertion is
// a proxy: a substring check passes against a template that renders
// an unclosed brace or a stray comma, and those surface only once a
// parser runs.
//
// **Does it declare what I meant?** The [Source] assertions. They
// exist for their failure messages more than their subject matter —
// a missing substring says a substring is missing, while
// [Source.AssertMethod] can say the method exists with a different
// signature, which is what is actually true most of the time. They
// are also immune to the normaliser's blank-line handling, which a
// substring spanning two declarations is not.
//
// **Does it type-check?** [Generated.AssertTypeChecks] runs `tsc`
// over the generated output and the module it imports from, which is
// the only way a dropped `?`, a wrong return type or a missing import
// is ever caught — all three produce output that parses perfectly.
//
// # tsc is optional, and the asymmetry is deliberate
//
// [Generated.AssertParses] uses tree-sitter, which is linked into
// this package: it always runs and needs nothing installed.
// [Generated.AssertTypeChecks] shells out to `tsc` and skips with a
// message when it is not on PATH and no local install answers.
//
// That is not a soft assertion. Type-checking generated TypeScript
// needs the TypeScript compiler, and vendoring a Go reimplementation
// of it is not on the table — so the choice is between a check that
// skips where the toolchain is absent and no check at all. A CI job
// that wants it installs Node; one that does not still gets the
// parse.
//
// # Cost, and how to spend it
//
// [Generated.AssertTypeChecks] shells out: roughly one to three
// seconds. A generator with forty render subtests cannot afford one
// per subtest, and a suite that tries will be deleted rather than
// fixed.
//
// Build the project once per fixture and let the structural
// assertions carry the fine-grained work:
//
//	gen := typescripttest.Render(t, backendts.New(), pkg, myplugin.New()).
//	    WithSource(typescripttest.TSFile(fixture().TSSource()))
//
//	t.Run("emits TypeScript the consumer can build", func(t *testing.T) {
//		gen.AssertTypeChecks(t)   // once
//	})
//	t.Run("exposes the configuration surface", func(t *testing.T) {
//		gen.Primary(t).AssertProperty(t, "UserStub", "onGet", "() => User")
//	})
//
// A [Generated] caches its assembled project, so several toolchain
// assertions over one fixture pay the setup once. Configure it before
// sharing it — [Generated.WithSource] and its siblings — and its
// assertions are then safe to run from parallel subtests.
//
// [Render] is the whole path from a fixture package to the files it
// produced. It takes the backend rather than constructing one, which
// is what keeps this package out of the backend's module graph — see
// [Driver] for why that constraint is load-bearing. Reach for
// [Driver] when the run needs a builder option, or when the test
// asserts on diagnostics and wants the pipeline rather than the
// files; [Rendered] adopts a pipeline a test drove itself.
//
// # What Go has that this does not
//
// The package answers the same questions [golangtest] does, spelled
// in TypeScript's vocabulary: [Source.AssertProperty] for its
// AssertField, [Source.AssertExtends] and [Source.AssertImplements]
// for its AssertEmbeds, [Source.Suites] and [Source.Cases] for its
// test-function and subtest queries, [Generated.InProject] for its
// InModule.
//
// Three of Go's have no counterpart, and their absence is the
// language rather than an omission. There is no AssertPackage: a
// `.ts` file declares no package, and what a module offers is its
// exports — [Source.API] is the assertion that covers it. There is no
// AssertPointerReceiver: TypeScript methods bind `this` implicitly,
// so there is no receiver form to get wrong. And there is no
// AssertVets: `go vet` is a second tool catching what the compiler
// lets through, where TypeScript's equivalents are compiler options
// — `noUnusedLocals`, `noUnusedParameters`,
// `exactOptionalPropertyTypes` — which [Generated.AssertTypeChecks]
// turns on by default, so the check is folded into the type check
// rather than sitting beside it.
//
// # Reading a failure
//
// A toolchain failure names code the author did not write, at line
// numbers in a directory that no longer exists. Every one of them
// therefore prints the offending file with line numbers attached.
// That is a requirement rather than a nicety: without it these
// assertions are worse than the substring checks they replace.
package typescripttest
