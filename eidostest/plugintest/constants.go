// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

// Package-private sentinel strings used in failure-output
// projections. Centralised so the goconst linter does not flag
// repeated literal occurrences across the role-specific suite
// files.
const (
	// unnamedSentinel is the placeholder used when a node's
	// owner / identity reflectively exposes neither QName nor
	// Name. Surfaces in projection output so unnamed entries
	// remain greppable.
	unnamedSentinel = "<unnamed>"

	// unownedSentinel is the placeholder used when a node's
	// owner back-pointer is nil. Surfaces in projection output
	// so unowned entries remain greppable.
	unownedSentinel = "<unowned>"

	// emptyTargetSentinel is the placeholder used for empty
	// [emit.Target] fields (Dir / Filename / Package) so a
	// missing routing slot stays visible in failure output.
	emptyTargetSentinel = "<empty>"
)

// Fixture literals shared by [NewFixturePlugin] and the [BrokenPlugin]
// fixtures. They are named rather than repeated inline because several
// fixtures must agree on them: an Outputs-shape fixture that models
// "two outputs under one tag" only models it if both entries spell the
// tag identically, and a drifted literal would turn a deliberate
// violation into a well-formed plugin that the suite then passes. The
// meta-tests would catch that, but as a confusing "fixture no longer
// defeats its check" rather than as the typo it is.
const (
	// fixtureTag is the non-empty Tag every multi-output fixture
	// declares. Its value is arbitrary; what matters is that the
	// fixtures modelling tag collisions reuse the same one.
	fixtureTag = "test"

	// fixtureSuffixA and fixtureSuffixB are two distinct, well-formed
	// Output suffixes. The Outputs-shape fixtures need a pair that
	// differs only where the violation under test says it differs, so
	// the suffix is never the thing making a fixture invalid.
	fixtureSuffixA = "_a.go"
	fixtureSuffixB = "_b.go"

	// fixtureCapability is the capability [NewFixturePlugin] provides
	// and the unstable-Provides fixture returns on one of its two
	// alternating answers — so that fixture differs from the
	// well-formed baseline in cardinality alone, not in labels.
	fixtureCapability = "cap.one"
)

// ConformanceLanguage is the backend language every capability lookup
// in this suite is driven with. It matches the value real backends
// return from [plugin.Backend.Language] and that plugins branch on in
// Outputs / Templates.
//
// It is exported because it is part of the conformance contract: a
// plugin keyed on any other spelling answers none of the suite's
// probes, and two of the framework checks previously drove
// {"go","rust","ts","py",""} — none of which is what a real plugin
// answers to. Both checks therefore iterated an empty Outputs slice
// and validated nothing for every plugin in the tree. Naming the
// value once removes the opportunity for those sets to diverge again.
const ConformanceLanguage = "golang"
