// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"maps"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
)

// TestDialectNamesReexported pins the façade against the dialect it
// re-exports.
//
// The re-export exists so a plugin replacing the dialect names the
// entries through this package rather than reaching past it into the
// language package. An entry added there and not here sends the next
// author back to the import this package exists to remove — and they
// will not notice, because reaching past it compiles.
func TestDialectNamesReexported(t *testing.T) {
	t.Parallel()

	reexported := []string{
		sdkgo.FuncAssertEqual, sdkgo.FuncAssertDeepEqual,
		sdkgo.FuncAssertNotEqual, sdkgo.FuncAssertTrue,
		sdkgo.FuncAssertFalse, sdkgo.FuncAssertNil,
		sdkgo.FuncAssertNotNil, sdkgo.FuncAssertLen,
		sdkgo.FuncAssertNoError, sdkgo.FuncAssertError,
		sdkgo.FuncNeedsDiffHelper,
	}

	t.Run("every dialect entry is named here", func(t *testing.T) {
		t.Parallel()
		want := slices.Sorted(maps.Keys(golang.AssertFuncMap()))
		got := slices.Sorted(slices.Values(reexported))
		if slices.Equal(got, want) {
			return
		}
		t.Errorf("the re-export has drifted from the dialect;\n"+
			"  in the dialect but not re-exported: %v\n"+
			"  re-exported but not in the dialect: %v",
			missing(want, got), missing(got, want))
	})

	t.Run("each name matches the dialect's own", func(t *testing.T) {
		t.Parallel()
		// A re-export bound to the wrong constant registers a
		// replacement under a name nothing calls, and the plugin's
		// own templates go on rendering the default.
		if sdkgo.FuncAssertEqual != golang.FuncAssertEqual {
			t.Errorf("FuncAssertEqual = %q, want %q",
				sdkgo.FuncAssertEqual, golang.FuncAssertEqual)
		}
		if sdkgo.FuncNeedsDiffHelper != golang.FuncNeedsDiffHelper {
			t.Errorf("FuncNeedsDiffHelper = %q, want %q",
				sdkgo.FuncNeedsDiffHelper, golang.FuncNeedsDiffHelper)
		}
	})
}

// missing returns the elements of a that b does not contain.
func missing(a, b []string) []string {
	var out []string
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}
