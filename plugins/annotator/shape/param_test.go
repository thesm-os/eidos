// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape_test

import (
	"reflect"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
)

// TestParamsForRole pins the predicate both the stamping pass and the
// resolver route through. The two must agree, so the rule has one
// implementation and this is its guard.
func TestParamsForRole(t *testing.T) {
	t.Parallel()

	unscoped := shape.Param{Key: "sentinel", Kind: shape.KindVar}
	openNext := shape.Param{Key: "next", Kind: shape.KindMember, Role: "open"}
	readerAt := shape.Param{Key: "at", Kind: shape.KindOpaque, Role: "next"}
	all := []shape.Param{unscoped, openNext, readerAt}

	t.Run("an unscoped param applies to every role", func(t *testing.T) {
		t.Parallel()
		for _, role := range []string{"open", "next", "close"} {
			got := shape.ParamsForRole([]shape.Param{unscoped}, role)
			if !reflect.DeepEqual(got, []shape.Param{unscoped}) {
				t.Errorf("role %q: got %v, want the unscoped param", role, got)
			}
		}
	})

	t.Run("a scoped param applies only under its own role", func(t *testing.T) {
		t.Parallel()
		got := shape.ParamsForRole(all, "open")
		if !reflect.DeepEqual(got, []shape.Param{unscoped, openNext}) {
			t.Fatalf("got %v, want the unscoped param and open's own", got)
		}
	})

	t.Run("another role's params are excluded", func(t *testing.T) {
		t.Parallel()
		// The whole point: `next` must not be a param key when the
		// host declares role=next, because there it is a partner
		// reference resolved through the host's own scope.
		got := shape.ParamsForRole(all, "next")
		if !reflect.DeepEqual(got, []shape.Param{unscoped, readerAt}) {
			t.Fatalf("got %v, want the unscoped param and next's own", got)
		}
	})

	t.Run("declaration order is preserved", func(t *testing.T) {
		t.Parallel()
		// Diagnostics list accepted keys in the order the author
		// wrote them, and resolution visits them in the same order so
		// two runs report the same failure first.
		got := shape.ParamKeys(shape.ParamsForRole(all, "open"))
		if !reflect.DeepEqual(got, []string{"sentinel", "next"}) {
			t.Fatalf("got %v, want [sentinel next]", got)
		}
	})

	t.Run("an empty role matches only the unscoped params", func(t *testing.T) {
		t.Parallel()
		// A directive with no role stamps nothing role-specific, so
		// nothing role-scoped can apply to it.
		got := shape.ParamsForRole(all, "")
		if !reflect.DeepEqual(got, []shape.Param{unscoped}) {
			t.Fatalf("got %v, want the unscoped param alone", got)
		}
	})

	t.Run("no params yields nil", func(t *testing.T) {
		t.Parallel()
		if got := shape.ParamsForRole(nil, "open"); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})
}
