// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
)

// All three of this plugin's directives reject the negated form,
// for two different reasons that the schemas alone do not
// distinguish — hence this table.
//
// `contract` and `mixin` are declared-only: the attachment exists
// exactly where someone wrote it, so deleting the line is the
// suppression and a negated form has nothing to act on. That
// denial is permanent.
//
// `shape` is inferred whether or not anyone asked, so "do not
// classify this" is a real thing to want, and there is currently no
// way to say it. Its denial is a placeholder: the form previously
// parsed, was dropped by the override lookup, and then had a shape
// stamped by detection anyway — accepted, understood, and silently
// discarded. Denying is the reversible half; lifting it once
// suppression exists is additive to every consumer.
func TestDirectives_RejectNegatedForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		directive *directive.Directive
	}{
		{
			name: "shape: suppression is meaningful but unimplemented",
			directive: &directive.Directive{
				Name: shape.DirectiveName, Args: []string{"reader"}, Negated: true,
			},
		},
		{
			name: "shape: bare negated form",
			directive: &directive.Directive{
				Name: shape.DirectiveName, Negated: true,
			},
		},
		{
			name: "contract: membership exists only where declared",
			directive: &directive.Directive{
				Name: shape.ContractDirectiveName, Args: []string{"tx"},
				KV: map[string]string{"role": "begin"}, Negated: true,
			},
		},
		{
			name: "mixin: attachment exists only where declared",
			directive: &directive.Directive{
				Name: shape.MixinDirectiveName, Args: []string{"atomic"}, Negated: true,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if sink := validateAgainstSchema(t, tc.directive); !sink.HasErrors() {
				t.Fatalf("-gen:%s validated; a form that cannot act must not parse quietly",
					tc.directive.Name)
			}
		})
	}
}

// TestDirectives_AcceptPositiveForm guards the denial from
// over-reaching: rejecting `-gen:` must not disturb `+gen:`.
func TestDirectives_AcceptPositiveForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		directive *directive.Directive
	}{
		{
			name: "shape",
			directive: &directive.Directive{
				Name: shape.DirectiveName, Args: []string{"reader"},
			},
		},
		{
			name: "contract",
			directive: &directive.Directive{
				Name: shape.ContractDirectiveName, Args: []string{"tx"},
				KV: map[string]string{"role": "begin"},
			},
		},
		{
			name: "mixin",
			directive: &directive.Directive{
				Name: shape.MixinDirectiveName, Args: []string{"atomic"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if sink := validateAgainstSchema(t, tc.directive); sink.HasErrors() {
				t.Fatalf("+gen:%s should validate; got %+v", tc.directive.Name, sink.Diagnostics())
			}
		})
	}
}
