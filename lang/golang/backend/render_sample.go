// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"fmt"

	langgo "go.thesmos.sh/eidos/lang/golang"
)

// ErrEmptySample is returned by [renderState.renderSample] when the
// sample carries no value.
//
// An error rather than an empty string, at the consumer's own
// request: rendering nothing is how a missed dispatch arm emits
// `foo(, )` and fails at format.Source, three files and one plugin
// away from the sample that was empty. The error names the refusal,
// so the failure points at the derivation instead.
var ErrEmptySample = errors.New("backend: sample carries no value")

// renderSample renders a [langgo.Sample] whole — the dispatch every
// consumer previously hand-wrote in its templates:
//
//	Expr set    → renderExpr, imports registered per reference
//	no Ref      → the bare text
//	Composite   → renderType(Ref) + Text
//	otherwise   → renderType(Ref) + "(" + Text + ")"
//
// The rule for which arm applies is a property of [langgo.Sample],
// not of any consumer, and the demonstration that hand-copying it
// drifts arrived with the type's fourth arm: three copies downstream,
// two updated, one shipping `foo(, )`. A consumer writes
// `{{ renderSample .Sample }}` and the next arm is this function's
// problem.
//
// Callers gate on [langgo.Sample.OK] before rendering; a sample that
// carries nothing errors rather than rendering empty.
func (s *renderState) renderSample(sample langgo.Sample) (string, error) {
	if !sample.OK() {
		return "", fmt.Errorf("%w (refusal: %s); gate on OK() before rendering",
			ErrEmptySample, sample.Refusal)
	}
	if sample.Expr != nil {
		return s.renderExpr(sample.Expr)
	}
	if sample.Ref == nil {
		return sample.Text, nil
	}
	typeText, err := s.renderType(sample.Ref)
	if err != nil {
		return "", err
	}
	if sample.Composite {
		return typeText + sample.Text, nil
	}
	return typeText + "(" + sample.Text + ")", nil
}
