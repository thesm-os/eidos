// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"testing"

	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// Test-only aliases that expose the package's internal
// assertion functions through the black-box `plugintest_test`
// package so the rejection-path tests can drive each check
// individually against a recording fake [testing.TB] — the
// top-level [RunSuite] / [RunAnnotatorSuite] / etc. entry
// points take `*testing.T`, which we cannot fabricate outside
// the testing harness.
var (
	// Framework-level assertions.
	AssertStableName                  = assertStableName
	AssertImplementsARole             = assertImplementsARole
	AssertCapabilityProviderStability = assertCapabilityProviderStability
	AssertDirectiveSchemaUniqueness   = assertDirectiveSchemaUniqueness
	AssertVersionedStability          = assertVersionedStability
	AssertEmitVersionedStability      = assertEmitVersionedStability
	AssertNodesOnlyStability          = assertNodesOnlyStability
	AssertFilenameProviderStability   = assertFilenameProviderStability
	AssertOutputsShape                = assertOutputsShape
	AssertTemplateProviderStability   = assertTemplateProviderStability
	AssertStableFuncMap               = assertStableFuncMap

	// Annotator-suite assertions.
	AssertAnnotateEmptyStoreDoesNotPanic   = assertAnnotateEmptyStoreDoesNotPanic
	AssertAnnotateDoesNotPanic             = assertAnnotateDoesNotPanic
	AssertAnnotateLeavesNodeCountUnchanged = assertAnnotateLeavesNodeCountUnchanged
	AssertAnnotateIsIdempotent             = assertAnnotateIsIdempotent
	AssertAnnotatorFixtureNamesUnique      = assertAnnotatorFixtureNamesUnique

	// Generator-suite assertions.
	AssertGenerateEmptyStoreDoesNotPanic     = assertGenerateEmptyStoreDoesNotPanic
	AssertGenerateDoesNotPanic               = assertGenerateDoesNotPanic
	AssertGenerateLeavesSourceNodesUnchanged = assertGenerateLeavesSourceNodesUnchanged
	AssertGenerateIsDeterministic            = assertGenerateIsDeterministic
	AssertGeneratorFixtureNamesUnique        = assertGeneratorFixtureNamesUnique
	AssertEmittedTagsAreDeclared             = assertEmittedTagsAreDeclared
	AssertOutputPackagesTolerateMissingTags  = assertOutputPackagesTolerateMissingTags

	// Backend-suite assertions.
	AssertRenderEmptyEmitDoesNotPanic = assertRenderEmptyEmitDoesNotPanic
	AssertRenderDoesNotPanic          = assertRenderDoesNotPanic
	AssertRenderCarriesNoErrors       = assertRenderCarriesNoErrors
	AssertRenderIsByteStable          = assertRenderIsByteStable
	AssertBackendFixtureNamesUnique   = assertBackendFixtureNamesUnique

	// Frontend-suite assertions.
	AssertLoadEmptyPatternDoesNotPanic = assertLoadEmptyPatternDoesNotPanic
	AssertLoadDoesNotPanic             = assertLoadDoesNotPanic
	AssertLoadPopulatesStore           = assertLoadPopulatesStore
	AssertLoadIsDeterministic          = assertLoadIsDeterministic
	AssertLoadIsFingerprintKeyed       = assertLoadIsFingerprintKeyed
	AssertFrontendFixtureNamesUnique   = assertFrontendFixtureNamesUnique

	// Options-suite assertions.
	AssertOptionsSchemaStability       = assertOptionsSchemaStability
	AssertOptionsFixtureCoversRequired = assertOptionsFixtureCoversRequired
	AssertSetOptionsAcceptsValid       = assertSetOptionsAcceptsValid
	AssertSetOptionsRejectsUnknown     = assertSetOptionsRejectsUnknown
)

// CheckPair exposes one row of the framework check table to the
// black-box meta-tests. The table itself is unexported because its
// shape is an implementation detail; the pairing it encodes is not,
// which is why the meta-tests assert against it.
type CheckPair struct {
	// Name is the subtest name RunSuite runs the check under.
	Name string
	// Violation names the fixture that must defeat Fn.
	Violation Violation
	// Fn is the assertion enforcing the contract.
	Fn func(testing.TB, plugin.Plugin)
}

// FrameworkChecks returns the table [RunSuite] walks, in execution
// order.
func FrameworkChecks() []CheckPair {
	cs := frameworkChecks()
	out := make([]CheckPair, len(cs))
	for i, c := range cs {
		out[i] = CheckPair{Name: c.name, Violation: c.violation, Fn: c.fn}
	}
	return out
}

// AssertNodesOnlyIsTruthful exposes the per-role NodesOnly check to
// the black-box meta-tests.
var AssertNodesOnlyIsTruthful = assertNodesOnlyIsTruthful

// EmptyStore returns a fresh store, so a meta-test can supply the two
// independent stores the truthfulness check compares.
func EmptyStore() *store.Store { return store.New() }
