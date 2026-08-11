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

// ReflectOptions derives a [Schema] from a struct value, reporting a
// malformed tag rather than panicking on it.
//
// [BindOptions] is the production path and treats a bad `eidos:` tag as
// an init-time programmer error: it panics, which is right for a plugin
// whose tags are correct and wrong for a test asserting that they are.
// A panic inside Bind takes down the suite before any subtest names the
// field at fault. This returns that failure as an error wrapping
// [ErrInvalidTag] or [ErrUnsupportedFieldType], so a plugin can pin its
// own options struct the way it pins the rest of its contract.
//
// Named for what it reflects rather than carrying the underlying
// Checked suffix, which distinguishes it from a panicking sibling this
// package deliberately does not re-export.
//
//nolint:gochecknoglobals // alias re-export of a stable function.
var ReflectOptions = opt.ReflectChecked

// Error sentinels surfaced when an options schema is derived and when
// values are decoded against it. Plugin code that wants to distinguish
// failure modes wraps them with [errors.Is].
var (
	// ErrInvalidTag is returned by [ReflectOptions] for a malformed
	// `eidos:` tag, or one naming a tag option the package does not
	// define. [BindOptions] panics on the same condition.
	ErrInvalidTag = opt.ErrInvalidTag

	// ErrUnsupportedFieldType is returned by [ReflectOptions] for an
	// options field whose Go type the decoder cannot parse.
	ErrUnsupportedFieldType = opt.ErrUnsupportedFieldType

	// ErrMissingRequired is returned by [Holder.SetOptions] when a
	// field the schema marks required was not supplied.
	ErrMissingRequired = opt.ErrMissingRequired

	// ErrUnknownField is returned by [Holder.SetOptions] for an input
	// key the schema does not declare. Strict by default, so a typo
	// fails at the config file that carries it rather than being
	// silently dropped.
	ErrUnknownField = opt.ErrUnknownField

	// ErrInvalidValue is returned by [Holder.SetOptions] for a value
	// that fails per-kind parsing, or one outside the field's OneOf
	// enumeration.
	ErrInvalidValue = opt.ErrInvalidValue

	// ErrInvalidDecodeTarget is returned by [Options.Decode] when the
	// destination passed in is not a pointer to a struct.
	ErrInvalidDecodeTarget = opt.ErrInvalidDecodeTarget
)
