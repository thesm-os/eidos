// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
)

// TestFrameworkChecks_EveryCheckNamesAKnownViolation pins that every
// contract [plugintest.RunSuite] enforces has a fixture capable of
// defeating it.
//
// This is the cheap half of the obligation: it proves a Violation
// constant exists for each check, not that the constant does
// anything. On its own it is satisfiable by adding a name — see
// TestBrokenPlugin_DefeatsTheCheckItNames for the half that measures.
// Both must hold; weakening either reverts the pairing to decoration.
func TestFrameworkChecks_EveryCheckNamesAKnownViolation(t *testing.T) {
	t.Parallel()

	known := plugintest.Violations()
	for _, c := range plugintest.FrameworkChecks() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			if c.Violation == "" {
				t.Fatalf("check %q names no Violation; add one to broken.go and pair it in "+
					"frameworkChecks, or the check has nothing proving it can fail", c.Name)
			}
			if !slices.Contains(known, c.Violation) {
				t.Fatalf("check %q names Violation %q, which Violations() does not list",
					c.Name, c.Violation)
			}
		})
	}
}

// TestBrokenPlugin_DefeatsTheCheckItNames is the measuring half: each
// fixture must actually make its own check fail.
//
// A check that cannot be made to fail is decoration, and the failure
// mode is silent — it passes against every plugin including broken
// ones, and nothing about a green run distinguishes "the contract
// holds" from "the check never fires". Two of this repo's shipped
// defects had exactly that shape.
func TestBrokenPlugin_DefeatsTheCheckItNames(t *testing.T) {
	t.Parallel()

	for _, c := range plugintest.FrameworkChecks() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeT()
			captureFatal(func() { c.Fn(fake, plugintest.BrokenPlugin(c.Violation)) })

			if !fake.failed {
				t.Fatalf("check %q passed against BrokenPlugin(%q), which exists to defeat it; "+
					"either the fixture no longer violates the contract or the check no longer "+
					"enforces it", c.Name, c.Violation)
			}
		})
	}
}

// TestBrokenPlugin_BreaksNothingElse pins the "and nothing else" half
// of each fixture's contract.
//
// A fixture that violates several contracts at once makes a failing
// suite ambiguous about which assertion did the work, so a regression
// in one check hides behind another fixture's noise. Keeping each
// violation surgical is what lets the test above attribute a failure
// to a specific check.
func TestBrokenPlugin_BreaksNothingElse(t *testing.T) {
	t.Parallel()

	checks := plugintest.FrameworkChecks()
	for _, subject := range checks {
		t.Run(subject.Name, func(t *testing.T) {
			t.Parallel()

			p := plugintest.BrokenPlugin(subject.Violation)
			for _, other := range checks {
				if other.Name == subject.Name {
					continue
				}
				fake := newFakeT()
				captureFatal(func() { other.Fn(fake, p) })
				if fake.failed {
					t.Errorf("BrokenPlugin(%q) also failed unrelated check %q:\n%s",
						subject.Violation, other.Name, joinFake(fake))
				}
			}
		})
	}
}

// TestViolations_EveryViolationDefeatsSomeCheck catches the orphan
// case the per-check tests cannot: a Violation that no check reacts
// to.
//
// Several violations deliberately target one check — the four Output
// shape rules all defeat the Outputs-shape assertion — so the pairing
// is many-to-one and this walks the fixture set rather than the check
// table. A fixture nothing catches is a contract the suite claims to
// cover and does not.
func TestViolations_EveryViolationDefeatsSomeCheck(t *testing.T) {
	t.Parallel()

	checks := plugintest.FrameworkChecks()
	for _, v := range plugintest.Violations() {
		t.Run(string(v), func(t *testing.T) {
			t.Parallel()

			p := plugintest.BrokenPlugin(v)
			for _, c := range checks {
				fake := newFakeT()
				captureFatal(func() { c.Fn(fake, p) })
				if fake.failed {
					return
				}
			}
			t.Fatalf("BrokenPlugin(%q) passed every framework check; the violation it models "+
				"is not enforced by any of them", v)
		})
	}
}

// TestBrokenPlugin_UnknownViolationIsWellFormed pins that a typo in a
// caller's Violation surfaces as an unexpectedly-passing suite rather
// than a nil dereference inside the fixture.
func TestBrokenPlugin_UnknownViolationIsWellFormed(t *testing.T) {
	t.Parallel()

	p := plugintest.BrokenPlugin(plugintest.Violation("no-such-violation"))
	for _, c := range plugintest.FrameworkChecks() {
		fake := newFakeT()
		captureFatal(func() { c.Fn(fake, p) })
		if fake.failed {
			t.Errorf("an unrecognised Violation produced a plugin failing %q:\n%s",
				c.Name, joinFake(fake))
		}
	}
}

// TestLyingNodesOnlyGenerator_IsCaught proves the NodesOnly
// truthfulness check can fail, against a generator built to defeat it.
//
// The check exists to convert a latent data race into a test failure:
// the pipeline dispatches a whole generator bucket concurrently when
// every member declares NodesOnly, so a member that reads the emit
// graph races a sibling writing it. A check for that which cannot
// itself be made to fail would be worse than none, because it would
// license the concurrency it was meant to guard.
func TestLyingNodesOnlyGenerator_IsCaught(t *testing.T) {
	t.Parallel()

	fake := newFakeT()
	captureFatal(func() {
		plugintest.AssertNodesOnlyIsTruthful(fake,
			plugintest.LyingNodesOnlyGenerator(),
			plugintest.EmptyStore(), plugintest.EmptyStore())
	})

	if !fake.failed {
		t.Fatal("a generator declaring NodesOnly while reading the emit graph passed the " +
			"truthfulness check")
	}
	assertFakeMentions(t, fake, "NodesOnly reports true but Generate read")
}

// TestFixturePlugin_PassesNodesOnlyTruthfulness pins the other side:
// the well-formed fixture declares NodesOnly and honours it, so the
// check must stay quiet. Without this the test above is satisfiable
// by an assertion that fires for everything.
func TestFixturePlugin_PassesNodesOnlyTruthfulness(t *testing.T) {
	t.Parallel()

	fake := newFakeT()
	captureFatal(func() {
		plugintest.AssertNodesOnlyIsTruthful(fake,
			plugintest.NewFixturePlugin(),
			plugintest.EmptyStore(), plugintest.EmptyStore())
	})

	if fake.failed {
		t.Fatalf("the well-formed fixture failed the truthfulness check:\n%s", joinFake(fake))
	}
}

// TestErroringPlugins_AreCaught proves each role's no-Error-severity
// check can fail, against a plugin built to defeat it.
//
// Every one of these scored green before the checks existed: a plugin
// reporting an Error on every input cleared 7/7 generator, 4/4
// annotator and 3/3 frontend subtests, while the same shape handed to
// the backend suite failed. A check for that which cannot be made to
// fail would restore exactly that state.
func TestErroringPlugins_AreCaught(t *testing.T) {
	t.Parallel()

	t.Run("a generator that reports an Error diagnostic on every input fails its suite", func(t *testing.T) {
		t.Parallel()

		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertGenerateCarriesNoErrors(fake, plugintest.ErroringGenerator(),
				plugintest.GeneratorFixture{Name: "one struct"},
				storefixture.New().Struct("User", nil).Build())
		})

		if !fake.failed {
			t.Fatal("a generator reporting an Error-severity diagnostic on a fixture it claims to " +
				"handle passed the diagnostic check")
		}
		assertFakeMentions(t, fake, plugintest.ErroringDiagnosticMessage)
	})

	t.Run("an annotator that reports an Error diagnostic on every input fails its suite", func(t *testing.T) {
		t.Parallel()

		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertAnnotateCarriesNoErrors(fake, plugintest.ErroringAnnotator(),
				plugintest.AnnotatorFixture{Name: "one struct"},
				storefixture.New().Struct("User", nil).Build())
		})

		if !fake.failed {
			t.Fatal("an annotator reporting an Error-severity diagnostic on a fixture it claims to " +
				"handle passed the diagnostic check")
		}
		assertFakeMentions(t, fake, plugintest.ErroringDiagnosticMessage)
	})

	t.Run("a frontend that reports an Error diagnostic on every input fails its suite", func(t *testing.T) {
		t.Parallel()

		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertLoadCarriesNoErrors(fake, plugintest.ErroringFrontend(),
				plugintest.FrontendFixture{Name: "one package", Pattern: "./...", ExpectsEmpty: true})
		})

		if !fake.failed {
			t.Fatal("a frontend reporting an Error-severity diagnostic on a fixture it claims to " +
				"handle passed the diagnostic check")
		}
		assertFakeMentions(t, fake, plugintest.ErroringDiagnosticMessage)
	})

	t.Run("a generator that errors only on the empty store fails the empty-store probe", func(t *testing.T) {
		t.Parallel()

		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertGenerateEmptyStoreCarriesNoErrors(fake, &emptyInputComplainer{name: "complainer"})
		})

		if !fake.failed {
			t.Fatal("a generator that reports an Error only when there is nothing to generate from " +
				"passed the empty-store probe; every project whose patterns expand to nothing exits " +
				"non-zero against it")
		}
		assertFakeMentions(t, fake, "an empty store")
	})

	t.Run("an annotator that errors only on the empty store fails the empty-store probe", func(t *testing.T) {
		t.Parallel()

		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertAnnotateEmptyStoreCarriesNoErrors(fake, &emptyInputComplainer{name: "complainer"})
		})

		if !fake.failed {
			t.Fatal("an annotator that reports an Error only when there is nothing to stamp passed " +
				"the empty-store probe")
		}
		assertFakeMentions(t, fake, "an empty store")
	})

	t.Run("a frontend emitting a diagnostic on an empty pattern still passes", func(t *testing.T) {
		t.Parallel()

		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertLoadEmptyPatternDoesNotPanic(fake, plugintest.ErroringFrontend(),
				plugintest.FrontendFixture{Name: "one package", Pattern: "./..."})
		})

		if fake.failed {
			t.Fatalf("the empty-pattern probe read its sink and failed a frontend for rejecting an "+
				"empty pattern, which is the conforming behaviour and what frontend/golang does:\n%s",
				joinFake(fake))
		}
	})
}

// TestPositionlessDiagnostics_AreCaught covers the positioned-
// diagnostic check in all three directions: a zero position is
// rejected, the fixture's waiver silences it, and a real position
// passes without one.
//
// The third subtest is what stops the first two being satisfiable by
// an assertion that fires for any diagnostic at all — which would
// make the waiver mandatory for every plugin that emits anything.
func TestPositionlessDiagnostics_AreCaught(t *testing.T) {
	t.Parallel()

	t.Run("a diagnostic carrying a zero Pos fails the check", func(t *testing.T) {
		t.Parallel()

		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertGenerateDiagnosticsArePositioned(fake, plugintest.ErroringGenerator(),
				plugintest.GeneratorFixture{Name: "one struct"},
				storefixture.New().Struct("User", nil).Build())
		})

		if !fake.failed {
			t.Fatal("a diagnostic carrying no source position passed the positioned-diagnostic check")
		}
		assertFakeMentions(t, fake, "no source position")
	})

	t.Run("a fixture allowing positionless diagnostics silences the check", func(t *testing.T) {
		t.Parallel()

		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertGenerateDiagnosticsArePositioned(fake, plugintest.ErroringGenerator(),
				plugintest.GeneratorFixture{Name: "one struct", AllowsPositionlessDiagnostics: true},
				storefixture.New().Struct("User", nil).Build())
		})

		if fake.failed {
			t.Fatalf("AllowsPositionlessDiagnostics did not waive the check:\n%s", joinFake(fake))
		}
	})

	t.Run("a positioned diagnostic passes without the waiver", func(t *testing.T) {
		t.Parallel()

		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertGenerateDiagnosticsArePositioned(fake, &positionedWarner{name: "warner"},
				plugintest.GeneratorFixture{Name: "one struct"},
				storefixture.New().Struct("User", nil).Build())
		})

		if fake.failed {
			t.Fatalf("a diagnostic carrying a real position failed the positioned-diagnostic "+
				"check:\n%s", joinFake(fake))
		}
	})
}

// TestFixturePlugin_PassesTheDiagnosticChecks pins the other side of
// both: the well-formed fixture emits nothing, so every diagnostic
// check must stay quiet. Without this the tests above are satisfiable
// by assertions that fire for everything.
func TestFixturePlugin_PassesTheDiagnosticChecks(t *testing.T) {
	t.Parallel()

	t.Run("the well-formed fixture surfaces no Error-severity diagnostics", func(t *testing.T) {
		t.Parallel()

		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertGenerateEmptyStoreCarriesNoErrors(fake, plugintest.NewFixturePlugin())
			plugintest.AssertGenerateCarriesNoErrors(fake, plugintest.NewFixturePlugin(),
				plugintest.GeneratorFixture{Name: "one struct"},
				storefixture.New().Struct("User", nil).Build())
		})

		if fake.failed {
			t.Fatalf("the well-formed fixture failed a diagnostic check:\n%s", joinFake(fake))
		}
	})

	t.Run("the well-formed fixture emits no positionless diagnostics", func(t *testing.T) {
		t.Parallel()

		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertGenerateDiagnosticsArePositioned(fake, plugintest.NewFixturePlugin(),
				plugintest.GeneratorFixture{Name: "one struct"},
				storefixture.New().Struct("User", nil).Build())
		})

		if fake.failed {
			t.Fatalf("the well-formed fixture failed the positioned-diagnostic check:\n%s", joinFake(fake))
		}
	})
}
