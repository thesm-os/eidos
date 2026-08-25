// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package reader

import (
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this detector stamps. Consumers
// switching on [shape.Get] compare against this constant rather
// than the literal string so renames surface as compile errors.
const Name = "reader"

// Detector returns the [shape.Detector] this package contributes
// to the umbrella shape plugin. Register one instance per
// pipeline:
//
//	pipe.WithAnnotator(shape.New().Detectors(reader.Detector()))
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 420,
		Detect: map[string]shape.DetectFunc{
			golang.Language: detectGolang,
		},
	}
}

// detectGolang recognises the canonical Go reader signature:
// exactly one non-context input parameter, exactly one non-error
// return value, plus a trailing `error` return. The leading
// `context.Context` parameter is optional — both `(ctx, K)` and
// `(K)` forms detect.
//
// A parameter that cannot serve as a key is refused — see
// [keyable]. The stamp's whole content is that the parameter *is* a
// key; generators derive same-key-same-value, read-after-write and
// deterministic re-read from it, and a parameter with no equality
// makes all three vacuous rather than false.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := golang.Callable(n)
	if !golang.HasError(returns) {
		return shape.Match{}, false
	}
	keys := golang.StripContext(params)
	values := golang.StripErrorTypes(returns)
	if len(keys) != 1 || len(values) != 1 {
		return shape.Match{}, false
	}
	if !keyable(keys[0].Type) {
		return shape.Match{}, false
	}
	return shape.Match{
		KeyType:   golang.QName(keys[0].Type),
		ValueType: golang.QName(values[0]),
	}, true
}

// keyable reports whether t can carry the reader shape's key
// contract: two calls with equal keys must be able to mean
// something. Slices, maps and functions have no equality in Go at
// all, so "the same key" is not expressible for them. An inline
// interface is refused for the adjacent reason — a stream passed as
// `interface{ Read(...) }` re-reads to the zero value, so a
// same-key-same-value assertion passes without testing anything.
//
// Narrower than [golang.Keyable], which answers Go's own
// comparability question. This one adds the anonymous-interface
// refusal, which is a statement about what a *reader key* means
// rather than about what Go permits: an inline interface compares
// fine and still makes the assertions the stamp licenses vacuous.
// A named interface — `io.Reader` being the common case — is opaque
// to both, since the node model records a package and an identifier
// and nothing about what they resolve to.
//
// Refusing a valid key costs a missing stamp, which a `+gen:shape`
// directive restores; accepting an invalid one costs a wrong stamp
// that downstream tooling acts on with no signal anything is off.
func keyable(t *sdk.TypeRef) bool {
	if t == nil || t.IsAnonInterface() {
		return false
	}
	return golang.Keyable(t)
}
