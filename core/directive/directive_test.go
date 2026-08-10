// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package directive_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/directive"
)

func TestDirective_Arg(t *testing.T) {
	t.Parallel()

	t.Run("returns the value at the given positional index", func(t *testing.T) {
		t.Parallel()
		d := &directive.Directive{Args: []string{"alpha", "beta", "gamma"}}
		assertEqualString(t, d.Arg(1), "beta")
	})

	t.Run("returns empty string for an index out of range", func(t *testing.T) {
		t.Parallel()
		d := &directive.Directive{Args: []string{"alpha"}}
		assertEqualString(t, d.Arg(5), "")
	})

	t.Run("returns empty string for a negative index", func(t *testing.T) {
		t.Parallel()
		d := &directive.Directive{Args: []string{"alpha"}}
		assertEqualString(t, d.Arg(-1), "")
	})
}

func TestDirective_HasKey(t *testing.T) {
	t.Parallel()

	t.Run("returns true when the key is present", func(t *testing.T) {
		t.Parallel()
		d := &directive.Directive{KV: map[string]string{"target": "Repo"}}
		if !d.HasKey("target") {
			t.Fatalf("HasKey should be true for present key")
		}
	})

	t.Run("returns false when the key is absent", func(t *testing.T) {
		t.Parallel()
		d := &directive.Directive{KV: map[string]string{}}
		if d.HasKey("absent") {
			t.Fatalf("HasKey should be false for absent key")
		}
	})
}

func TestDirective_Value(t *testing.T) {
	t.Parallel()

	t.Run("returns the value when present", func(t *testing.T) {
		t.Parallel()
		d := &directive.Directive{KV: map[string]string{"target": "Repo"}}
		assertEqualString(t, d.Value("target"), "Repo")
	})

	t.Run("returns empty string when absent", func(t *testing.T) {
		t.Parallel()
		d := &directive.Directive{KV: map[string]string{}}
		assertEqualString(t, d.Value("absent"), "")
	})
}

func TestHasPositive(t *testing.T) {
	t.Parallel()

	t.Run("matches an unnegated entry with the given name", func(t *testing.T) {
		t.Parallel()
		list := []*directive.Directive{{Name: "repo"}}
		if !directive.HasPositive(list, "repo") {
			t.Fatalf("HasPositive should match the +gen:repo form")
		}
	})

	t.Run("does not match a negated entry with the same name", func(t *testing.T) {
		t.Parallel()
		list := []*directive.Directive{{Name: "repo", Negated: true}}
		if directive.HasPositive(list, "repo") {
			t.Fatalf("HasPositive should not match the -gen:repo form")
		}
	})

	t.Run("returns false on an empty list", func(t *testing.T) {
		t.Parallel()
		if directive.HasPositive(nil, "repo") {
			t.Fatalf("HasPositive should return false on a nil list")
		}
	})

	t.Run("matches when a positive entry coexists with a negative", func(t *testing.T) {
		t.Parallel()
		list := []*directive.Directive{
			{Name: "repo", Negated: true},
			{Name: "repo"},
		}
		if !directive.HasPositive(list, "repo") {
			t.Fatalf("HasPositive should match when at least one positive entry exists")
		}
	})
}

func TestHasNegated(t *testing.T) {
	t.Parallel()

	t.Run("matches a negated entry with the given name", func(t *testing.T) {
		t.Parallel()
		list := []*directive.Directive{{Name: "repo", Negated: true}}
		if !directive.HasNegated(list, "repo") {
			t.Fatalf("HasNegated should match the -gen:repo form")
		}
	})

	t.Run("does not match an unnegated entry with the same name", func(t *testing.T) {
		t.Parallel()
		list := []*directive.Directive{{Name: "repo"}}
		if directive.HasNegated(list, "repo") {
			t.Fatalf("HasNegated should not match the +gen:repo form")
		}
	})

	t.Run("returns false on an empty list", func(t *testing.T) {
		t.Parallel()
		if directive.HasNegated(nil, "repo") {
			t.Fatalf("HasNegated should return false on a nil list")
		}
	})
}

// TestLast pins the last-wins lookup a repeatable value directive
// needs, against the first-wins rule a flag uses.
func TestLast(t *testing.T) {
	t.Parallel()

	list := []*directive.Directive{
		{Name: "default", KV: map[string]string{"limit": "10"}},
		{Name: "other"},
		nil,
		{Name: "default", KV: map[string]string{"limit": "50"}},
	}

	t.Run("returns the final entry of that name", func(t *testing.T) {
		t.Parallel()
		// An author writing the directive twice has said the second
		// thing; reading the first emits a value the source contradicts
		// two lines further down, with no diagnostic.
		got := directive.Last(list, "default")
		if got == nil || got.Value("limit") != "50" {
			t.Fatalf("Last = %+v, want the limit=50 entry", got)
		}
	})

	t.Run("a single entry is both first and last", func(t *testing.T) {
		t.Parallel()
		if got := directive.Last(list, "other"); got == nil || got.Name != "other" {
			t.Fatalf("Last = %+v, want the sole entry", got)
		}
	})

	t.Run("an absent name and an empty list report nothing", func(t *testing.T) {
		t.Parallel()
		if got := directive.Last(list, "absent"); got != nil {
			t.Errorf("Last = %+v, want nil", got)
		}
		if got := directive.Last(nil, "default"); got != nil {
			t.Errorf("Last over an empty list = %+v, want nil", got)
		}
	})
}
