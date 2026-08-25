// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"errors"
	"fmt"
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/eidostest/internal/nodefixture"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// annotatorTagKey is the meta key the test annotators stamp.
// Registered via [meta.EnsureKey] so re-running the test binary
// (and the package's own internal duplicate test fixtures) does
// not trip [meta.NewKey]'s duplicate-registration panic.
var annotatorTagKey = meta.EnsureKey(
	"plugintest.test.annotator.tag",
	func(raw string) (string, error) { return raw, nil },
)

// TestRunAnnotatorSuite_PassesForWellFormedAnnotator pins the
// happy path of [plugintest.RunAnnotatorSuite]: an annotator
// that stamps a stable value on every struct passes every
// contract — does not panic, leaves node counts unchanged, and
// produces identical meta state on a second pass.
func TestRunAnnotatorSuite_PassesForWellFormedAnnotator(t *testing.T) {
	t.Parallel()
	plugintest.RunAnnotatorSuite(
		t,
		&taggingAnnotator{name: "tagger"},
		[]plugintest.AnnotatorFixture{
			{
				Name: "package with one struct",
				BuildStore: func(t *testing.T) *store.Store {
					t.Helper()
					return nodefixture.Store("User")
				},
			},
			{
				Name: "package with three structs",
				BuildStore: func(t *testing.T) *store.Store {
					t.Helper()
					return nodefixture.Store("User", "Order", "Invoice")
				},
			},
		},
	)
}

// TestRunAnnotatorSuite_RejectsPanickingAnnotator covers the
// empty-store contract: an annotator that panics in Annotate
// fails the suite.
func TestRunAnnotatorSuite_RejectsPanickingAnnotator(t *testing.T) {
	t.Parallel()
	a := &panickingAnnotator{name: "panicky"}
	fake := newFakeT()
	if !captureFatal(func() {
		plugintest.AssertAnnotateEmptyStoreDoesNotPanic(fake, a)
	}) {
		// The panic is recovered into a t.Errorf, not a Fatalf;
		// so we don't capture via captureFatal. Verify Errorf
		// instead.
		assertFakeMentions(t, fake, "Annotate panicked on an empty store")
		return
	}
	assertFakeMentions(t, fake, "Annotate panicked on an empty store")
}

// TestRunAnnotatorSuite_RejectsNonIdempotentAnnotator pins the
// idempotency contract: an annotator that stamps a different
// value on each call fails on the second-pass comparison.
func TestRunAnnotatorSuite_RejectsNonIdempotentAnnotator(t *testing.T) {
	t.Parallel()
	s := nodefixture.Store("User")
	a := &flappingAnnotator{name: "flap"}
	fake := newFakeT()
	plugintest.AssertAnnotateIsIdempotent(fake, a, s)
	assertFakeMentions(t, fake, "annotator is not idempotent")
}

// TestRunAnnotatorSuite_RejectsAnnotatorThatAddsNodes pins the
// frozen-store contract: an annotator that mutates the
// source-side store fails the node-count check.
func TestRunAnnotatorSuite_RejectsAnnotatorThatAddsNodes(t *testing.T) {
	t.Parallel()
	s := nodefixture.Store("User")
	a := &nodeAddingAnnotator{name: "adder"}
	fake := newFakeT()
	plugintest.AssertAnnotateLeavesNodeCountUnchanged(fake, a, s)
	assertFakeMentions(t, fake, "Annotate changed indexed node counts")
}

// TestRunAnnotatorSuite_RejectsAnnotatorReturningError covers
// the return-error path: an annotator returning a non-nil
// error fails the per-fixture "does not panic" probe (which
// also covers clean returns through the error-or-panic
// recovery wrapper).
func TestRunAnnotatorSuite_RejectsAnnotatorReturningError(t *testing.T) {
	t.Parallel()
	s := nodefixture.Store("User")
	a := &erroringAnnotator{name: "errr", err: errors.New("boom")}
	fake := newFakeT()
	plugintest.AssertAnnotateDoesNotPanic(fake, a, s)
	assertFakeMentions(t, fake, "boom")
}

// TestRunAnnotatorSuite_FailsOnDuplicateFixtureName pins the
// fixture-name uniqueness contract: two fixtures sharing a Name
// would produce identical subtest paths.
func TestRunAnnotatorSuite_FailsOnDuplicateFixtureName(t *testing.T) {
	t.Parallel()
	fixtures := []plugintest.AnnotatorFixture{
		{Name: "dup", BuildStore: func(t *testing.T) *store.Store {
			t.Helper()
			return nodefixture.Store()
		}},
		{Name: "dup", BuildStore: func(t *testing.T) *store.Store {
			t.Helper()
			return nodefixture.Store()
		}},
	}
	fake := newFakeT()
	captureFatal(func() {
		plugintest.AssertAnnotatorFixtureNamesUnique(fake, fixtures)
	})
	assertFakeMentions(t, fake, "duplicate fixture Name")
}

// taggingAnnotator stamps a stable value (the struct name)
// under [annotatorTagKey] on every struct it sees. Idempotent
// by construction: re-stamping with the same value overwrites
// in place at the same authority.
type taggingAnnotator struct{ name string }

// Name returns the configured identifier.
func (a *taggingAnnotator) Name() string { return a.name }

// Annotate stamps the tag key on every struct in the store.
func (a *taggingAnnotator) Annotate(ctx *plugin.AnnotatorContext) error {
	for _, s := range ctx.Store.Nodes().Structs().Items() {
		annotatorTagKey.Set(s.EnsureMeta(), "tag:"+s.Name, a.name)
	}
	return nil
}

// panickingAnnotator panics in Annotate. Used to verify the
// suite recovers and reports a contract failure.
type panickingAnnotator struct{ name string }

// Name returns the configured identifier.
func (a *panickingAnnotator) Name() string { return a.name }

// Annotate panics with a sentinel message.
func (*panickingAnnotator) Annotate(_ *plugin.AnnotatorContext) error {
	panic("plugintest test: panickingAnnotator panicking on purpose") //nolint:forbidigo
}

// flappingAnnotator stamps a different value on each call,
// driving the idempotency check's rejection path.
type flappingAnnotator struct {
	name  string
	count int
}

// Name returns the configured identifier.
func (a *flappingAnnotator) Name() string { return a.name }

// Annotate stamps a counter-derived value that differs on every
// invocation.
func (a *flappingAnnotator) Annotate(ctx *plugin.AnnotatorContext) error {
	a.count++
	for _, s := range ctx.Store.Nodes().Structs().Items() {
		annotatorTagKey.Set(s.EnsureMeta(), fmt.Sprintf("flap-%d", a.count), a.name)
	}
	return nil
}

// nodeAddingAnnotator mutates the source-side store by adding a
// synthetic struct on every Annotate call. Violates the
// frozen-store contract — drives the node-count check's
// rejection path.
type nodeAddingAnnotator struct {
	name  string
	count int
}

// Name returns the configured identifier.
func (a *nodeAddingAnnotator) Name() string { return a.name }

// Annotate adds a synthetic struct to the first package the
// store knows about.
func (a *nodeAddingAnnotator) Annotate(ctx *plugin.AnnotatorContext) error {
	a.count++
	pkgs := ctx.Store.Nodes().Packages().Items()
	if len(pkgs) == 0 {
		return nil
	}
	pkg := pkgs[0]
	synthetic := &node.Struct{
		Name:    fmt.Sprintf("Synthetic%d", a.count),
		Package: pkg.Path,
	}
	pkg.Structs = append(pkg.Structs, synthetic)
	// The bucket-level Add bypasses the package-rooted ingest
	// path the frontend would normally use; that's fine for this
	// test — we only need the indexed structs count to grow so
	// the suite's check has something to detect.
	if err := ctx.Store.Nodes().Structs().Add(synthetic.QName(), synthetic); err != nil {
		return fmt.Errorf("nodeAddingAnnotator: add synthetic struct: %w", err)
	}
	return nil
}

// erroringAnnotator returns a configured error from Annotate
// without panicking. Verifies the suite distinguishes the
// returned-error path from the recover-from-panic path while
// still failing the contract.
type erroringAnnotator struct {
	name string
	err  error
}

// Name returns the configured identifier.
func (a *erroringAnnotator) Name() string { return a.name }

// Annotate returns the configured error.
func (a *erroringAnnotator) Annotate(_ *plugin.AnnotatorContext) error {
	return fmt.Errorf("erroringAnnotator: %w", a.err)
}

// countingAnnotator stamps a value derived from a per-instance call
// counter rather than from the node it is handed.
//
// It defeats determinism and not idempotency, which is the whole
// point: run twice over one store its already-stamped guard makes the
// second pass a no-op and the meta state matches, so the single-store
// check reports green. Given two equivalent stores it stamps "1" on
// one and "2" on the other. The negative fixture lives here rather
// than in broken.go because the completeness meta-test walks the
// framework check table only, and this is a per-role contract.
type countingAnnotator struct {
	name  string
	calls int
}

// Name returns the configured identifier.
func (a *countingAnnotator) Name() string { return a.name }

// Annotate stamps the current call count on every struct.
func (a *countingAnnotator) Annotate(ctx *plugin.AnnotatorContext) error {
	a.calls++
	for _, s := range ctx.Store.Nodes().Structs().Items() {
		if b := s.Meta(); b != nil && countingAnnotatorKey.Has(b) {
			continue
		}
		countingAnnotatorKey.Set(s.EnsureMeta(), a.calls, a.name)
	}
	return nil
}

// countingAnnotatorKey is the bag key countingAnnotator stamps under.
var countingAnnotatorKey = meta.EnsureKey("plugintest.counting", meta.IntParser)

// TestRunAnnotatorSuite_RejectsCountDerivedStamp pins the annotator
// determinism contract: a stamp derived from anything other than the
// input differs across two equivalent stores.
func TestRunAnnotatorSuite_RejectsCountDerivedStamp(t *testing.T) {
	t.Parallel()

	t.Run("a count-derived stamp fails the two-store determinism check", func(t *testing.T) {
		t.Parallel()
		a := &countingAnnotator{name: "counting"}
		fake := newFakeT()
		plugintest.AssertAnnotateIsDeterministic(
			fake, a,
			nodefixture.Store("User"),
			nodefixture.Store("User"),
		)
		assertFakeMentions(t, fake, "annotator is not idempotent")
	})

	t.Run("the same annotator still clears the single-store idempotency check", func(t *testing.T) {
		t.Parallel()
		// Demonstrates why the new check earns its place: the defect
		// above is invisible to the check that already existed.
		a := &countingAnnotator{name: "counting"}
		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertAnnotateIsIdempotent(fake, a, nodefixture.Store("User"))
		})
		if fake.failed {
			t.Fatalf("idempotency was expected to pass a count-derived stamp: %v", fake.errs)
		}
	})
}
