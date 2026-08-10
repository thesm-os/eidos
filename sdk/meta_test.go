// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"strconv"
	"sync/atomic"
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/sdk"
)

// freshSeq numbers the key names [freshKeyName] hands out.
//
// The registry is global and process-wide, and NewKey panics rather
// than returning on a name already registered. The suite runs under
// `-count 3`, so a fixed literal registers three times in one process
// and the second run panics — a failure about the test's own naming,
// not about the code under test.
//
//nolint:gochecknoglobals // process-wide counter for a process-wide registry.
var freshSeq atomic.Uint64

// freshKeyName returns a key name no earlier call has registered, so a
// test asserting on NewKey's fresh-registration path gets one.
func freshKeyName(prefix string) string {
	return prefix + "." + strconv.FormatUint(freshSeq.Add(1), 10)
}

// TestMetaAliasesPreserveIdentity pins the metadata surface as
// aliases.
//
// Identity carries more weight here than elsewhere: a [Key]
// declared through the façade in one plugin and read through the
// underlying package in another must be the same key, or the
// annotator writes a fact the generator cannot see — and neither
// side reports anything, because a missing metadata value is
// indistinguishable from one that was never stamped.
//
//nolint:staticcheck // intentional redundant typing — the redundancy is the test
func TestMetaAliasesPreserveIdentity(t *testing.T) {
	t.Parallel()

	t.Run("bag and key alias to the meta package", func(t *testing.T) {
		t.Parallel()
		var b1 *sdk.Bag
		var b2 *meta.Bag = b1
		_ = b2
		var k1 sdk.Key[string]
		var k2 meta.Key[string] = k1
		_ = k2
		var p1 sdk.MetaParser[int]
		var p2 meta.Parser[int] = p1
		_ = p2
		var pv1 sdk.MetaProvenance
		var pv2 meta.Provenance = pv1
		_ = pv2
		var a1 sdk.MetaAuthority
		var a2 meta.Authority = a1
		_ = a2
	})
}

// TestAuthoritiesMatchUnderlying pins the authority ranks. These
// are iota constants and the ordering is the whole mechanism: an
// annotator writing one rank too high silently overrules the
// `+gen:` directive it was meant to obey, and the run still
// succeeds.
func TestAuthoritiesMatchUnderlying(t *testing.T) {
	t.Parallel()

	t.Run("each re-export equals its meta constant", func(t *testing.T) {
		t.Parallel()
		pairs := []struct {
			name string
			got  sdk.MetaAuthority
			want meta.Authority
		}{
			{"AuthorityPlugin", sdk.AuthorityPlugin, meta.AuthorityPlugin},
			{"AuthorityDirective", sdk.AuthorityDirective, meta.AuthorityDirective},
			{"AuthorityManual", sdk.AuthorityManual, meta.AuthorityManual},
		}
		for _, pair := range pairs {
			if pair.got != pair.want {
				t.Errorf("sdk.%s = %d, want %d", pair.name, pair.got, pair.want)
			}
		}
	})

	t.Run("the ranks stay strictly ascending", func(t *testing.T) {
		t.Parallel()
		ranked := []struct {
			name string
			auth sdk.MetaAuthority
		}{
			{"AuthorityPlugin", sdk.AuthorityPlugin},
			{"AuthorityDirective", sdk.AuthorityDirective},
			{"AuthorityManual", sdk.AuthorityManual},
		}
		for i := 1; i < len(ranked); i++ {
			if ranked[i-1].auth >= ranked[i].auth {
				t.Errorf("%s (%d) does not rank below %s (%d)",
					ranked[i-1].name, ranked[i-1].auth,
					ranked[i].name, ranked[i].auth)
			}
		}
	})
}

// TestKeyConstructorsShareTheGlobalRegistry proves the generic
// wrappers do more than type-check: they must register into the
// same process-wide table the underlying package uses, or a key
// declared through the façade would be invisible to the
// directive-override step and `+gen:` overrides on it would do
// nothing.
func TestKeyConstructorsShareTheGlobalRegistry(t *testing.T) {
	t.Parallel()

	t.Run("EnsureKey returns the underlying registration", func(t *testing.T) {
		t.Parallel()
		const name = "sdktest.shared.viaunderlying"
		declared := meta.EnsureKey(name, meta.StringParser)
		viaFacade := sdk.EnsureKey(name, sdk.StringParser)
		if viaFacade.Name() != declared.Name() {
			t.Fatalf("EnsureKey returned %q, want %q", viaFacade.Name(), declared.Name())
		}
	})

	t.Run("a facade-declared key round-trips a value", func(t *testing.T) {
		t.Parallel()
		key := sdk.EnsureKey("sdktest.shared.roundtrip", sdk.StringParser)
		bag := sdk.NewBag()
		key.Set(bag, "repository", "sdktest")
		got, ok := key.Get(bag)
		if !ok || got != "repository" {
			t.Fatalf("Get after Set = (%q, %v), want (repository, true)", got, ok)
		}
	})

	t.Run("NewKey and EnsureKey agree on a fresh name", func(t *testing.T) {
		t.Parallel()
		// Fresh per call rather than a literal: NewKey panics on a name
		// already in the process-wide registry, so a literal fails on the
		// second of the suite's three runs.
		name := freshKeyName("sdktest.fresh.newkey")
		declared := sdk.NewKey(name, sdk.StringParser)
		if got := sdk.EnsureKey(name, sdk.StringParser); got.Name() != declared.Name() {
			t.Fatalf("EnsureKey after NewKey = %q, want %q", got.Name(), declared.Name())
		}
	})
}

// TestParsersProxyUnderlying spot-checks each re-exported parser
// against the value shape it claims. A parser var bound to the
// wrong function type-checks — they share a signature per value
// type — and surfaces only as a directive override that silently
// parses to the wrong thing.
func TestParsersProxyUnderlying(t *testing.T) {
	t.Parallel()

	t.Run("StringParser takes the raw argument", func(t *testing.T) {
		t.Parallel()
		got, err := sdk.StringParser(" spaced ")
		if err != nil || got != " spaced " {
			t.Fatalf("StringParser = (%q, %v), want the input verbatim", got, err)
		}
	})

	t.Run("StringListParser splits on commas", func(t *testing.T) {
		t.Parallel()
		got, err := sdk.StringListParser("a,b")
		if err != nil || len(got) != 2 {
			t.Fatalf("StringListParser = (%v, %v), want two entries", got, err)
		}
	})

	t.Run("BoolParser reads a boolean", func(t *testing.T) {
		t.Parallel()
		got, err := sdk.BoolParser("true")
		if err != nil || !got {
			t.Fatalf("BoolParser(true) = (%v, %v), want (true, nil)", got, err)
		}
	})

	t.Run("IntParser reads a decimal integer", func(t *testing.T) {
		t.Parallel()
		got, err := sdk.IntParser("42")
		if err != nil || got != 42 {
			t.Fatalf("IntParser(42) = (%d, %v), want (42, nil)", got, err)
		}
	})

	t.Run("NodeRefParser accepts a qualified name", func(t *testing.T) {
		t.Parallel()
		got, err := sdk.NodeRefParser("pkg.Type")
		if err != nil || got != "pkg.Type" {
			t.Fatalf("NodeRefParser = (%q, %v), want (pkg.Type, nil)", got, err)
		}
	})
}
