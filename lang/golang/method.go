// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strconv"

	"go.thesmos.sh/eidos/core/naming"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// Deriving the projection: a Go callable in the form a generator
// renders.
//
// The projection itself is [emit.SigInfo], because nothing in its
// shape is Go's and a plugin core that asks its rules for one has to
// name the type. What lives here is the deriving, which is Go's
// throughout: which identifier is legal, how a keyword is adjusted,
// what an anonymous parameter is called, which slot carries the error,
// and what a receiver is bound to without colliding with a parameter.
//
// # Both directions
//
// [SigOf] takes a source method and [SigOfEmit] one another generator
// produced. A generator consuming upstream emit output needs the same
// projection over a shape that carries no source node, which is why
// the reference mock generator grew a private intermediate to lower
// both onto.

// Param is [emit.SigParam].
type Param = emit.SigParam

// Return is [emit.SigReturn].
type Return = emit.SigReturn

// Sig is [emit.SigInfo].
type Sig = emit.SigInfo

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
		ReceiverIdent: o.receiver,
	}
	s.Params = paramsFrom(m.Params, o)
	// After the parameters, because both forms are made unique against
	// exactly those identifiers.
	switch {
	case o.receiverType != "":
		s.ReceiverIdent = ReceiverIdent(o.receiverType, s.Idents()...)
	case o.receiver != "":
		// A literal receiver is disambiguated too. It used not to be,
		// so `WithReceiverIdent("s")` on a method taking a parameter
		// named `s` bound both to the same identifier and the generated
		// body silently read the parameter where it meant the receiver
		// — the collision this projection reserves an identifier to
		// prevent, reintroduced by the option that names it.
		s.ReceiverIdent = UniqueIdent(o.receiver, s.Idents()...)
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
		ReceiverIdent: "",
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
	s := &Sig{Name: m.Name, ReceiverIdent: o.receiver}

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
