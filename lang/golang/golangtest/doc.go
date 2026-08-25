// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package golangtest asserts on the Go a generator produced.
//
// It is a set of composable helpers, not a conformance suite. What a
// generator *should* emit is that generator's business — there is no
// fixed battery to run, and the one universal claim ("whatever a Go
// generator emits must compile") still cannot be checked without the
// hand-written package the output references, which only the fixture
// author has. So nothing here runs itself; every assertion is one
// the test author reaches for. [plugintest] is the conformance half,
// and this is deliberately the other kind.
//
// # What it is for
//
// A generator's tests answer three questions, in order.
//
// **Is what I emitted valid Go?** [Generated.AssertCompiles],
// [Generated.AssertVets]. Without these, every other assertion is a
// proxy: a substring check passes against a template that renders an
// unused local, a redeclared name, or a field the template still
// references. Those surface only once a compiler runs.
//
// **Does it declare what I meant?** The [Source] assertions. They
// exist for their failure messages more than their subject matter —
// a missing substring says a substring is missing, while
// [Source.AssertMethod] can say the method exists with a different
// signature, which is what is actually true most of the time. They
// are also immune to gofmt's column alignment, which a substring
// spelling a struct field is not.
//
// **Does it behave?** [Generated.AssertSatisfies] compiles an
// interface-satisfaction assertion against the generated type, which
// catches a dropped variadic marker or a method lost through an embed
// — both of which compile perfectly and satisfy nothing.
// [Generated.AssertDoesNotSatisfy] states the same thing the other way
// round, which is the claim a shape detector is really making: not
// that the canonical shape passes but that every near miss fails.
// [Generated.AssertTestsPass] compiles and *runs* a generated
// `_test.go`, which is the only way a generator that emits test
// suites ever learns whether they pass.
//
// # Cost, and how to spend it
//
// Every toolchain assertion shells out to `go`: roughly one to three
// seconds each. A generator with forty render subtests cannot afford
// one per subtest, and a suite that tries will be deleted rather than
// fixed.
//
// Build the module once per fixture and let the structural
// assertions carry the fine-grained work:
//
//	gen := golangtest.Render(t, backendgolang.New(), pkg, myplugin.New()).
//	    WithSource(sourcePkg)
//
//	t.Run("emits Go the consumer can build", func(t *testing.T) {
//		gen.AssertCompiles(t)   // once
//	})
//	t.Run("exposes the configuration surface", func(t *testing.T) {
//		gen.Primary(t).AssertField(t, "StoreStub", "OnGet", "*StoreGetStub")
//	})
//
// A [Generated] caches its built module, so several toolchain
// assertions over one fixture pay the setup once. Configure it before
// sharing it — [Generated.WithSource] and its siblings — and its
// assertions are then safe to run from parallel subtests.
//
// [Render] is the whole path from a fixture package to the files it
// produced. It takes the backend rather than constructing one, which
// is what keeps this package out of backend/golang's module graph —
// see [Driver] for why that constraint is load-bearing. Reach for
// [Driver] when the run needs a builder option, or when the test
// asserts on diagnostics and wants the pipeline rather than the
// files; [Rendered] adopts a pipeline a test drove itself.
//
// # Reading a failure
//
// A toolchain failure names code the author did not write, at line
// numbers in a directory that no longer exists. Every one of them
// therefore prints the offending file with line numbers attached.
// That is a requirement rather than a nicety: without it these
// assertions are worse than the substring checks they replace.
package golangtest
