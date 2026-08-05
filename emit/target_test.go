// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/emit"
)

func TestTarget_IsZero(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		t    emit.Target
		want bool
	}{
		{"zero value is zero", emit.Target{}, true},
		{"non-zero Dir is not zero", emit.Target{Dir: "x"}, false},
		{"non-zero Filename is not zero", emit.Target{Filename: "x.go"}, false},
		{"non-zero Package is not zero", emit.Target{Package: "x"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.t.IsZero() != tc.want {
				t.Fatalf("IsZero %+v = %v, want %v", tc.t, tc.t.IsZero(), tc.want)
			}
		})
	}
}

func TestTarget_JoinPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		t    emit.Target
		want string
	}{
		{"both populated", emit.Target{Dir: "internal/repo", Filename: "user_gen.go"}, "internal/repo/user_gen.go"},
		{"empty dir yields empty", emit.Target{Filename: "user.go"}, ""},
		{"empty filename yields empty", emit.Target{Dir: "internal/repo"}, ""},
		{"both empty yields empty", emit.Target{}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertEqualString(t, tc.t.JoinPath(), tc.want)
		})
	}
}

func TestTarget_IsComparable(t *testing.T) {
	t.Parallel()

	t.Run("equal Targets compare as map keys", func(t *testing.T) {
		t.Parallel()
		m := map[emit.Target]int{}
		k := emit.Target{Dir: "a", Filename: "b.go", Package: "b"}
		m[k] = 1
		m[emit.Target{Dir: "a", Filename: "b.go", Package: "b"}] = 2
		if len(m) != 1 {
			t.Fatalf("Target should be comparable as a map key; got %d entries", len(m))
		}
	})
}

// bareSetter is the minimum a plugin-defined emit kind needs to be
// an [emit.OutputPackageSetter]: [emit.BaseEmit], a Kind, and the
// one method. The assertion below is the executable form of the
// interface's claim that opting in is the whole ceremony — there is
// no registry to update and nothing for the framework to switch on.
type bareSetter struct {
	emit.BaseEmit
}

// Kind returns a plugin-defined kind.
func (*bareSetter) Kind() kind.Kind { return "test.baresetter" }

// SetOutputPackages satisfies the interface.
func (*bareSetter) SetOutputPackages(map[string]string) {}

var (
	_ emit.OutputPackageSetter = (*bareSetter)(nil)

	// The interface embeds emit.Node, so every implementor is a real
	// emit value the dispatch walk can reach. Losing the embed would
	// let a non-Node type satisfy it and never be visited.
	_ emit.Node = (*bareSetter)(nil)
)

// TestOutputPackageSetter_NilMapIsUsable pins that the documented
// "may see fewer tags than it declared" case bottoms out at zero:
// indexing the map an implementor receives is always safe, including
// when nothing routed, so implementors need no nil guard of their
// own.
func TestOutputPackageSetter_NilMapIsUsable(t *testing.T) {
	t.Parallel()

	var byTag map[string]string
	if got := byTag[""]; got != "" {
		t.Fatalf("byTag[\"\"] on a nil map = %q, want the zero value", got)
	}
}
