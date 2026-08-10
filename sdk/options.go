// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import "go.thesmos.sh/eidos/core/opt"

// Holder is the canonical typed-options holder plugins embed
// so they satisfy [plugin.OptionsProvider] without
// duplicating the schema / SetOptions boilerplate. Construct
// via [BindOptions].
//
//	type Plugin struct {
//	    *sdk.Holder[Options]
//	    opts Options
//	}
//
//	func New() *Plugin {
//	    p := &Plugin{}
//	    p.Holder = sdk.BindOptions(&p.opts)
//	    return p
//	}
type Holder[T any] = opt.Holder[T]

// BindOptions returns a [*Holder] bound to target. The
// reflected schema is derived from *target at Bind time and
// cached on the returned holder so subsequent
// [Holder.OptionsSchema] / [Holder.SetOptions] calls reuse
// it.
func BindOptions[T any](target *T) *Holder[T] {
	return opt.Bind(target)
}

// Options is a decoded option set — the value
// [plugin.OptionsProvider.SetOptions] receives.
type Options = opt.Options

// Schema is the reflected field set [Holder.OptionsSchema]
// returns and [NewOptions] decodes against.
type Schema = opt.Schema

// NewOptions builds an option set from a schema and raw string
// values, for handing to SetOptions directly.
//
// [BindOptions] is the whole surface a plugin needs in production —
// the pipeline decodes and applies the caller's values at Build time.
// This is for a plugin's own tests: driving one specific option value
// through and asserting on what gets rendered is a different question
// from the one [plugintest.RunOptionsSuite] answers, which is whether
// the schema accepts valid keys and rejects unknown ones.
//
// Named NewOptions rather than New because this package already
// carries NewSink, NewStore, NewProvenance, NewDirective and
// NewExternal; a bare New among those reads as constructing the
// package's own subject.
//
//nolint:gochecknoglobals // alias re-export of a stable constructor.
var NewOptions = opt.New
