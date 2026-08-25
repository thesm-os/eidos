// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package closer

import (
	"slices"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this package stamps.
const Name = "closer"

// Priority places this detector directly above [poisonaccessor].
//
// The two accept the same signature, so the order is the whole
// discrimination: this one takes the names Go reserves for release and
// leaves every other bare-error nullary to fall through. Below
// [readerwithbool] at 840, which needs a second result and cannot
// collide.
const Priority = 835

// Names are the method spellings this detector claims.
//
// Close first because it is not a convention but an interface:
// [io.Closer] is in the standard library and a nullary error-only
// Close is its entire contract. The rest are what a release method is
// called when Close is taken, already used for something else, or
// reads wrong for the subject.
//
//nolint:gochecknoglobals // intentionally exported as the recognised set
var Names = []string{"Close", "Shutdown", "Stop", "Disconnect", "Terminate"}

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: Priority,
		Detect: map[string]shape.DetectFunc{
			golang.Language: detectGolang,
		},
	}
}

// detectGolang accepts a nullary bare-error callable whose name is one
// of [Names].
func detectGolang(n sdk.Node) (shape.Match, bool) {
	if !slices.Contains(Names, callableName(n)) {
		return shape.Match{}, false
	}
	params, returns := golang.Callable(n)
	if len(params) != 0 || len(returns) != 1 {
		return shape.Match{}, false
	}
	if !golang.HasError(returns) {
		return shape.Match{}, false
	}
	return shape.Match{}, true
}

// callableName returns the declared name of a function or method, and
// the empty string for anything else.
//
// Local rather than a shape helper: this is the only detector that
// discriminates on a name, and exporting the accessor would invite a
// second one to do it without the argument this package carries for
// why it is allowed here.
func callableName(n sdk.Node) string {
	switch x := n.(type) {
	case *sdk.Function:
		return x.Name
	case *sdk.Method:
		return x.Name
	}
	return ""
}
