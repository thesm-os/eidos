// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape

import "go.thesmos.sh/eidos/sdk"

// The shape contract is three meta keys, all registered as typed
// singletons so consumers read them through one canonical key per
// concept regardless of which shape package stamped them.
//
// Keys go through [sdk.EnsureKey] so multiple shape sub-packages
// (each importing this package) register the same key without an
// init-order coupling: whichever package initialises first claims
// the singleton; later registrations return the same key.
var (
	// MetaShape carries the canonical shape name (`"reader"`,
	// `"writer"`, …) on a callable's meta bag. Owned by the shape
	// sub-package that recognised the pattern; the value vocabulary
	// is the union of every registered shape.
	MetaShape = sdk.EnsureKey(
		"shape",
		sdk.StringParser,
	) //nolint:gochecknoglobals // registry-singleton key

	// MetaKeyType carries the qualified type of the key parameter
	// (the K in `func(ctx, K) (V, error)` or its language-native
	// equivalent). Absent when the shape carries no key, e.g.,
	// Lifecycle or Pure.
	MetaKeyType = sdk.EnsureKey(
		"shape.key_type",
		sdk.StringParser,
	) //nolint:gochecknoglobals // registry-singleton key

	// MetaValueType carries the qualified type of the value/output
	// (the V in `func(ctx, K) (V, error)` or its language-native
	// equivalent). Absent when the shape carries no value, e.g.,
	// Mutator or Lifecycle.
	MetaValueType = sdk.EnsureKey(
		"shape.value_type",
		sdk.StringParser,
	) //nolint:gochecknoglobals // registry-singleton key
)

// Get returns the canonical shape name stamped on bag, or empty
// when no shape is stamped. Consumers switch on the result for
// type-safe dispatch:
//
//	switch shape.Get(method.Meta()) {
//	case reader.Name:
//	    …
//	case writer.Name:
//	    …
//	}
//
// Compare against the per-shape package's exported `Name` constant
// rather than the literal string so renames surface as compile
// errors.
func Get(bag *sdk.Bag) string {
	if bag == nil {
		return ""
	}
	got, _ := MetaShape.Get(bag)
	return got
}

// IsStamped reports whether bag already carries a shape stamp.
// Detectors and the umbrella plugin use this to honour the
// "first stamp wins" rule — user directives and earlier-registered
// detectors take precedence over later detector passes.
func IsStamped(bag *sdk.Bag) bool {
	return Get(bag) != ""
}
