// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package deleter

import (
	"slices"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this package stamps.
const Name = "deleter"

// Priority places this detector directly above [writer] at 500.
//
// The two accept the same signature — one non-context parameter, a
// bare error out — so the order is the whole discrimination, and
// anything below writer would never win a dispatch. Nothing else
// occupies the band: the next detector up takes two or more results.
const Priority = 550

// Names are the method spellings this detector claims.
//
// Delete first because it is the spelling Go itself uses: it is the
// builtin for a map entry and the method on [sync.Map]. Remove is the
// standard library's other word for it ([os.Remove], [io/fs]), and
// the rest are what a removal is called when the subject is a cache
// or a store.
//
// Deliberately short. A name admitted here cannot be withdrawn
// without breaking whoever relied on the classification, while adding
// one later costs nothing — so a word that also means something else
// stays out. `Drop` is the case in point: it removes a table, and it
// also drops a packet, a connection, or a privilege.
//
//nolint:gochecknoglobals // intentionally exported as the recognised set
var Names = []string{"Delete", "Remove", "Del", "Evict", "Purge"}

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: Priority,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang accepts a callable named for removal taking one
// non-context parameter and returning only an error.
//
// The parameter records as KeyType, which is the second reason this
// detector exists. [writer] records its own as ValueType, so a delete
// falling through to it labelled the key it addresses as the value it
// stores — a stamp a consumer reads to decide what to hand the call.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	if !slices.Contains(Names, callableName(n)) {
		return shape.Match{}, false
	}
	params, returns := shape.GoCallable(n)
	if !shape.GoHasError(returns) {
		return shape.Match{}, false
	}
	keys := shape.GoStripContext(params)
	results := shape.GoStripError(returns)
	if len(keys) != 1 || len(results) != 0 {
		return shape.Match{}, false
	}
	return shape.Match{KeyType: shape.QName(keys[0].Type)}, true
}

// callableName returns the declared name of a function or method, and
// the empty string for anything else.
//
// Local rather than a shape helper, for the reason [closer] gives:
// exporting the accessor would invite a third name-gated detector
// without the argument each of these two carries for why the gate is
// allowed. Here it is that a delete and a write are the same
// signature, and only the word separates them.
func callableName(n sdk.Node) string {
	switch x := n.(type) {
	case *sdk.Function:
		return x.Name
	case *sdk.Method:
		return x.Name
	}
	return ""
}
