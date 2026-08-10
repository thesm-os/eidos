// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/sdk"
)

// TestPositionAliasesPreserveIdentity pins the position types as
// aliases. A plugin reads a [Pos] off a node and passes it to a
// diagnostic method typed in terms of the underlying package; a
// wrapper would break that pass-through, which is the only thing
// the type is ever used for.
//
//nolint:staticcheck // intentional redundant typing — the redundancy is the test
func TestPositionAliasesPreserveIdentity(t *testing.T) {
	t.Parallel()

	t.Run("positions alias to the position package", func(t *testing.T) {
		t.Parallel()
		var p1 sdk.Pos
		var p2 position.Pos = p1
		_ = p2
		var r1 sdk.Range
		var r2 position.Range = r1
		_ = r2
	})
}

// TestPositionFactoriesProxyUnderlying drives each re-exported
// constructor. They share a return type, so a var bound to the
// wrong one type-checks; only the value distinguishes them, and a
// diagnostic pointing at the wrong line is worse than one pointing
// nowhere.
func TestPositionFactoriesProxyUnderlying(t *testing.T) {
	t.Parallel()

	t.Run("At records file, line and column", func(t *testing.T) {
		t.Parallel()
		got := sdk.At("repo.go", 12, 4)
		want := position.At("repo.go", 12, 4)
		if got != want {
			t.Fatalf("At = %+v, want %+v", got, want)
		}
	})

	t.Run("AtOffset adds the byte offset", func(t *testing.T) {
		t.Parallel()
		got := sdk.AtOffset("repo.go", 12, 4, 96)
		if got.Offset != 96 {
			t.Fatalf("AtOffset.Offset = %d, want 96", got.Offset)
		}
	})

	t.Run("Synthetic is distinguishable from the zero value", func(t *testing.T) {
		t.Parallel()
		// The distinction the constructor exists for: "generated,
		// so there is no source line" against "we lost the line".
		got := sdk.Synthetic("repogen")
		if !got.IsSynthetic() {
			t.Errorf("Synthetic(repogen) = %+v, want a synthetic position", got)
		}
		if got.IsZero() {
			t.Error("Synthetic produced the zero position")
		}
	})

	t.Run("NewRange rejects a cross-file span", func(t *testing.T) {
		t.Parallel()
		if _, err := sdk.NewRange(sdk.At("a.go", 1, 1), sdk.At("b.go", 2, 1)); err == nil {
			t.Error("NewRange accepted a span across two files")
		}
		if _, err := sdk.NewRange(sdk.At("a.go", 1, 1), sdk.At("a.go", 2, 1)); err != nil {
			t.Errorf("NewRange rejected a valid span: %v", err)
		}
	})
}
