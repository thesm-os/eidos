// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strconv"

	"go.thesmos.sh/eidos/core/naming"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// The projection: a source callable in the form a generator
// renders.
//
// Every Go generator that emits a method has to answer the same
// questions about it — what a body calls each parameter, which
// field of a recorded call each return maps to, whether the
// source's return names survive onto the generated signature — and
// four independent implementations of these answers exist across
// this repository and its consumers. They have already diverged:
// one numbers unnamed returns across every slot and another across
// the non-error slots only, so adding an `error` return to a source
// signature renumbers one generator's fields and not the other's.
// Both compile.
//
// # Why a third model
//
// [node] is the source model and [emit] the output model; [Sig] is
// neither, and that is a real cost. It earns the place by being
// derivable and total: every field is computed from the source, no
// field is optional, and [Sig.Source] is retained so a consumer
// needing a fact this does not carry reaches the declaration rather
// than asking for the field to be added. Per-generator facts —
// which mixin is attached, what shape a detector stamped — stay
// with the generator; they are not signature facts.
//
// # Both directions
//
// [SigOf] takes a source method and [SigOfEmit] one another
// generator produced. A generator consuming upstream emit output
// needs the same projection over a shape that carries no source
// node, which is why the reference mock generator grew a private
// intermediate to lower both onto.

// Param is one positional parameter in rendered form.
type Param struct {
	// Name is the identifier a generated body binds this parameter
	// to: the declared name where the source gave one, `arg<N>`
	// where it did not, adjusted for keywords and made unique
	// within the list.
	Name string

	// Declared is the name the source actually wrote, empty for an
	// anonymous or blank parameter. Carried apart from Name so a
	// consumer can tell a chosen identifier from an authored one —
	// a doc comment quoting the source should quote what was
	// written.
	Declared string

	// Type is the parameter's type in emit form.
	Type emit.Ref

	// Source is the parameter's declared type in source form, nil
	// when the projection came from the emit side. Retained because
	// every structural query in this package takes a
	// [node.TypeRef], and a consumer classifying a parameter needs
	// one.
	Source *node.TypeRef

	// Field is the exported identifier a recorded-call struct uses
	// for this parameter — the Pascal form of Name.
	Field string

	// Variadic reports whether the source declared `...T`.
	//
	// Type stays the element type, matching the model and what the
	// generated signature spells after the ellipsis. Everything
	// around it changes: a recorded field holds a slice, a
	// forwarding call needs `name...`, and a double that dropped
	// the marker takes one value where the interface takes many and
	// no longer satisfies it.
	Variadic bool
}

// Return is one return slot in rendered form.
type Return struct {
	// Name is the source's declared return name, empty for the
	// anonymous form. The blank identifier normalises to empty —
	// `_` cannot be used as a derived identifier, and leaving it in
	// would make every consumer special-case it.
	Name string

	// Type is the slot's type in emit form.
	Type emit.Ref

	// Source is the slot's declared type in source form, nil when
	// the projection came from the emit side.
	Source *node.TypeRef

	// Field is the exported identifier a recorded-call struct uses.
	// Always populated: a recorded call needs one field per return
	// whether or not the source named the slot.
	//
	// The fallback reads as a failure message rather than as an
	// index. The error slot is `Err`, because that is what it is; a
	// lone value slot is `Result`, since a number distinguishes it
	// from nothing; several value slots are `Result0`, `Result1`, …
	// numbered across the value slots only, so adding an error
	// return does not renumber the fields beside it.
	Field string

	// Local is the identifier a generated body binds this slot to
	// when capturing a call's result. The declared name where the
	// signature carries names, `r<N>` otherwise — prefixed with an
	// underscore on the rare occasion a parameter already holds
	// that identifier, since shadowing a parameter would capture
	// the wrong value.
	Local string

	// Error reports whether this slot carries the builtin error.
	//
	// Recorded per slot rather than derived by position, because a
	// signature returning `(error, string)` is unusual and legal,
	// and a positional rule binds the wrong slot without failing to
	// compile.
	Error bool
}

// Sig is a callable projected into the form a generator renders.
type Sig struct {
	// Name is the callable's identifier.
	Name string

	Params  []Param
	Returns []Return

	// TypeParams is the declaration's own type-parameter list, nil
	// for a non-generic callable.
	TypeParams []*node.TypeParam

	// Receiver is the method's receiver type, nil for a function
	// and for an interface method.
	Receiver *node.TypeRef

	// NamedReturns reports whether the generated signature may
	// carry the source's return names. All-or-nothing; see
	// [NamedReturnsUsable] for both reasons.
	NamedReturns bool

	// Source is the method this was projected from, nil when the
	// projection came from the emit side. The escape hatch: a
	// consumer needing a fact this type does not carry reads the
	// declaration rather than growing the projection.
	Source *node.Method

	// receiverIdent is the identifier the generated method binds
	// its receiver to, held so the collision guards can reason
	// about it.
	receiverIdent string
}

// SigOption tunes the identifiers a projection chooses.
type SigOption func(*sigOpts)

// sigOpts carries the resolved option set.
type sigOpts struct {
	receiver     string
	receiverType string
	paramPrefix  string
	localPrefix  string
}

// Default identifiers the projection uses when a caller supplies
// none.
//
// Exported because a generator emitting a receiver has to spell the
// same identifier the collision guard reserved, and a template that
// chose its own would silently invalidate the guard.
const (
	DefaultReceiverIdent = "s"
	DefaultParamPrefix   = "arg"
	DefaultLocalPrefix   = "r"
)

// WithReceiverIdent sets the receiver identifier the generated
// method binds, which the return-name collision guard reserves.
func WithReceiverIdent(name string) SigOption {
	return func(o *sigOpts) { o.receiver = name }
}

// WithReceiverFromType derives the receiver identifier from the type
// the method is emitted on, disambiguated against the parameters.
//
// The option [WithReceiverIdent] cannot express, because of an
// ordering the caller cannot resolve alone: the identifier is the type
// name's initial *made unique against the parameter identifiers*, and
// those are what [SigOf] is being asked to project. A generator
// holding only the type name would otherwise have to project the
// signature once for its identifiers, derive the receiver, and project
// it again — which is what the one generator doing this wrote, and
// what put a second copy of the rule in a plugin.
//
// Takes precedence over [WithReceiverIdent] when both are supplied:
// this one is derived from the emitted type and the other is a
// caller's literal, so honouring the literal would silently reinstate
// the shadowing this exists to prevent.
func WithReceiverFromType(typeName string) SigOption {
	return func(o *sigOpts) { o.receiverType = typeName }
}

// WithParamPrefix sets the stem for a parameter the source left
// anonymous — `arg0`, `arg1` by default.
func WithParamPrefix(prefix string) SigOption {
	return func(o *sigOpts) { o.paramPrefix = prefix }
}

// WithLocalPrefix sets the stem for the local a body captures an
// anonymous return into — `r0`, `r1` by default.
func WithLocalPrefix(prefix string) SigOption {
	return func(o *sigOpts) { o.localPrefix = prefix }
}

// resolveOpts folds the supplied options over the defaults.
func resolveOpts(opts []SigOption) sigOpts {
	o := sigOpts{
		receiver:    DefaultReceiverIdent,
		paramPrefix: DefaultParamPrefix,
		localPrefix: DefaultLocalPrefix,
	}
	for _, apply := range opts {
		apply(&o)
	}
	return o
}

// SigOf projects a source method into rendered form.
//
// Returns nil for a nil method, so a caller iterating a resolved
// method set that contains one skips rather than panics.
func SigOf(m *node.Method, opts ...SigOption) *Sig {
	if m == nil {
		return nil
	}
	o := resolveOpts(opts)
	s := &Sig{
		Name:          m.Name,
		TypeParams:    m.TypeParams,
		Receiver:      m.Receiver,
		Source:        m,
		receiverIdent: o.receiver,
	}
	s.Params = paramsFrom(m.Params, o)
	// After the parameters, because the derived form is the type's
	// initial made unique against exactly those identifiers.
	if o.receiverType != "" {
		s.receiverIdent = ReceiverIdent(o.receiverType, s.Idents()...)
	}
	s.NamedReturns = NamedReturnsUsable(m.Returns, s.Taken()...)
	s.Returns = returnsFrom(m.Returns, s.Params, s.NamedReturns, o)
	return s
}

// SigOfFunc projects a free function into rendered form.
//
// A function has no receiver, so nothing is reserved for one and a
// return may legitimately take the identifier a method's receiver
// would have claimed.
func SigOfFunc(f *node.Function, opts ...SigOption) *Sig {
	if f == nil {
		return nil
	}
	o := resolveOpts(opts)
	s := &Sig{
		Name:          f.Name,
		TypeParams:    f.TypeParams,
		Source:        nil,
		receiverIdent: "",
	}
	s.Params = paramsFrom(f.Params, o)
	s.NamedReturns = NamedReturnsUsable(f.Returns, s.Taken()...)
	s.Returns = returnsFrom(f.Returns, s.Params, s.NamedReturns, o)
	return s
}

// SigOfEmit projects a method another generator produced.
//
// The emit model records no return names — [emit.Return] carries
// one, but a generator that built the method chose it rather than
// an author writing it — so the projection is always anonymous and
// every field falls back to its positional form. Source stays nil:
// there is no declaration behind an emitted method.
func SigOfEmit(m *emit.Method, opts ...SigOption) *Sig {
	if m == nil {
		return nil
	}
	o := resolveOpts(opts)
	s := &Sig{Name: m.Name, receiverIdent: o.receiver}

	s.Params = make([]Param, 0, len(m.Params))
	idents := make([]string, 0, len(m.Params))
	for i, p := range m.Params {
		declared := ""
		if p != nil {
			declared = p.Name
		}
		name := UniqueIdent(identFor(declared, i, o.paramPrefix), idents...)
		idents = append(idents, name)
		out := Param{Name: name, Declared: declared, Field: naming.Pascal(name)}
		if p != nil {
			out.Type, out.Variadic = p.Type, p.Variadic
		}
		s.Params = append(s.Params, out)
	}

	s.Returns = make([]Return, 0, len(m.Returns))
	values := 0
	for _, r := range m.Returns {
		if r != nil && !isErrorRef(r.Type) {
			values++
		}
	}
	valueIdx, errIdx := 0, 0
	for i, r := range m.Returns {
		out := Return{Local: o.localPrefix + strconv.Itoa(i)}
		if r != nil {
			out.Type = r.Type
			out.Error = isErrorRef(r.Type)
		}
		out.Field = returnField("", out.Error, values, &valueIdx, &errIdx)
		s.Returns = append(s.Returns, out)
	}
	return s
}

// paramsFrom resolves the identifier, field name and type of every
// parameter.
func paramsFrom(params []*node.Param, o sigOpts) []Param {
	out := make([]Param, 0, len(params))
	idents := make([]string, 0, len(params))
	for i, p := range params {
		declared := ""
		if p != nil && !IsBlank(p.Name) {
			declared = p.Name
		}
		name := UniqueIdent(identFor(declared, i, o.paramPrefix), idents...)
		idents = append(idents, name)
		entry := Param{Name: name, Declared: declared, Field: naming.Pascal(name)}
		if p != nil {
			entry.Source, entry.Type, entry.Variadic = p.Type, FromNode(p.Type), p.Variadic
		}
		out = append(out, entry)
	}
	return out
}

// returnsFrom resolves the field name, capture local and error flag
// of every return slot.
//
// The value counter runs ahead of the loop because the lone-value
// case names its field `Result` rather than `Result0`, and that is
// only knowable once the whole list has been counted.
func returnsFrom(returns []*node.Return, params []Param, named bool, o sigOpts) []Return {
	values := 0
	for _, r := range returns {
		if r != nil && !IsError(r.Type) {
			values++
		}
	}
	taken := make(map[string]struct{}, len(params))
	for _, p := range params {
		taken[p.Name] = struct{}{}
	}

	out := make([]Return, 0, len(returns))
	valueIdx, errIdx := 0, 0
	for i, r := range returns {
		entry := Return{}
		if r != nil {
			entry.Name = r.Name
			entry.Source = r.Type
			entry.Type = FromNode(r.Type)
			entry.Error = IsError(r.Type)
		}
		entry.Field = returnField(entry.Name, entry.Error, values, &valueIdx, &errIdx)
		entry.Local = localFor(entry.Name, i, named, taken, o.localPrefix)
		out = append(out, entry)
	}
	return out
}

// identFor picks a parameter's identifier: the declared name made
// safe, or the positional fallback.
func identFor(declared string, index int, prefix string) string {
	if declared == "" {
		return prefix + strconv.Itoa(index)
	}
	return SafeIdent(declared)
}

// localFor picks the identifier a body captures a return into.
//
// Named results are already declared by the signature, so the local
// is the declared name. An anonymous result needs a fresh one,
// which is positional — and prefixed with an underscore when a
// parameter already holds that identifier, since shadowing a
// parameter would capture the wrong value.
func localFor(name string, index int, named bool, taken map[string]struct{}, prefix string) string {
	if named {
		return name
	}
	local := prefix + strconv.Itoa(index)
	if _, clash := taken[local]; clash {
		return "_" + local
	}
	return local
}

// returnField picks the exported field name for one return slot.
//
// The counters advance only for the class they name, which is what
// keeps value numbering independent of whether the signature
// returns an error.
func returnField(declared string, isErr bool, values int, valueIdx, errIdx *int) string {
	if declared != "" {
		return naming.Pascal(declared)
	}
	if isErr {
		// A second error return is legal and vanishingly rare; index
		// it rather than emitting a duplicate field name, which would
		// not compile.
		defer func() { *errIdx++ }()
		if *errIdx == 0 {
			return "Err"
		}
		return "Err" + strconv.Itoa(*errIdx)
	}
	defer func() { *valueIdx++ }()
	if values == 1 {
		return "Result"
	}
	return "Result" + strconv.Itoa(*valueIdx)
}

// isErrorRef reports whether an emit reference names the builtin
// error.
//
// The emit side carries no meta stamp, so this is the spelling
// alone: a builtin ref named `error`, or an external one whose
// package is empty.
func isErrorRef(r emit.Ref) bool {
	b, ok := r.(*emit.BuiltinRef)
	return ok && b.Name == typeError
}

// ErrReturn returns the slot carrying the error, or nil.
//
// Found by flag rather than by position, so a body assigning an
// injected failure names the right field on a signature returning
// `(error, bool)`.
func (s *Sig) ErrReturn() *Return {
	if s == nil {
		return nil
	}
	for i := range s.Returns {
		if s.Returns[i].Error {
			return &s.Returns[i]
		}
	}
	return nil
}

// HasResults reports whether the callable returns anything, which
// decides whether a generated body returns at all.
func (s *Sig) HasResults() bool { return s != nil && len(s.Returns) > 0 }

// ReturnsError reports whether any slot carries the builtin error.
func (s *Sig) ReturnsError() bool { return s.ErrReturn() != nil }

// IsGeneric reports whether the callable carries type parameters.
func (s *Sig) IsGeneric() bool { return s != nil && len(s.TypeParams) > 0 }

// Variadic reports whether the last parameter is variadic.
func (s *Sig) Variadic() bool {
	return s != nil && len(s.Params) > 0 && s.Params[len(s.Params)-1].Variadic
}

// Idents returns the parameter identifiers in order.
func (s *Sig) Idents() []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s.Params))
	for i := range s.Params {
		out[i] = s.Params[i].Name
	}
	return out
}

// Locals returns the capture identifiers in order.
func (s *Sig) Locals() []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s.Returns))
	for i := range s.Returns {
		out[i] = s.Returns[i].Local
	}
	return out
}

// Taken returns every identifier the generated method already
// occupies — its receiver and its parameters.
//
// What a caller passes to [UniqueIdent] when choosing a name of its
// own, so a helper variable a generator introduces cannot shadow
// something the signature declared.
func (s *Sig) Taken() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.Params)+1)
	if s.receiverIdent != "" {
		out = append(out, s.receiverIdent)
	}
	return append(out, s.Idents()...)
}

// ReceiverIdent returns the identifier the generated method binds
// its receiver to, empty for a function.
func (s *Sig) ReceiverIdent() string {
	if s == nil {
		return ""
	}
	return s.receiverIdent
}
