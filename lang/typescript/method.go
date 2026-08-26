// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

import (
	"strconv"

	"go.thesmos.sh/eidos/core/naming"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// ReceiverIdent is what a generated method body calls the value it
// was called on.
//
// TypeScript binds it as a keyword rather than as a declared
// parameter, so unlike Go's receiver it cannot collide with anything
// the signature declares and is the same in every method.
const ReceiverIdent = "this"

// paramPrefix and localPrefix are the positional fallbacks for a
// parameter with no declared name and for a captured result.
const (
	paramPrefix = "arg"
	localPrefix = "r"
)

// resultField is the recorded-call field name for a lone return slot.
//
// TypeScript callables return one value, so this is what almost every
// projection produces. The numbered form exists for a signature
// carrying a tuple the model recorded as several slots.
const resultField = "Result"

// SigOf is [Source.SigOf] as a plain function.
//
// Returns nil for a nil method, so a caller iterating a resolved
// method set that contains one skips rather than panics.
func SigOf(m *node.Method) *emit.SigInfo {
	if m == nil {
		return nil
	}
	s := &emit.SigInfo{
		Name:          m.Name,
		TypeParams:    m.TypeParams,
		Receiver:      m.Receiver,
		Source:        m,
		ReceiverIdent: ReceiverIdent,
		// TypeScript signatures name their parameters and not their
		// results, so there is never a source name for a return slot
		// to carry.
		NamedReturns: false,
	}
	s.Params = sigParams(m.Params)
	s.Returns = sigReturns(m.Returns, s.Params)
	return s
}

// SigOfFunc projects a free function into rendered form.
//
// The same projection as [SigOf] with no receiver, which for
// TypeScript is the only difference between the two — a method and a
// function declare their parameters and results identically.
func SigOfFunc(f *node.Function) *emit.SigInfo {
	if f == nil {
		return nil
	}
	s := &emit.SigInfo{Name: f.Name, TypeParams: f.TypeParams}
	s.Params = sigParams(f.Params)
	s.Returns = sigReturns(f.Returns, s.Params)
	return s
}

// sigParams resolves the identifier, field name and type of every
// parameter.
//
// Each identifier is made unique within the list, because the
// positional fallback and a declared name can collide: `read(arg0:
// Buffer, x: Buffer)` names the first parameter exactly what the
// second would fall back to, and two parameters of one name do not
// compile.
func sigParams(params []*node.Param) []emit.SigParam {
	out := make([]emit.SigParam, 0, len(params))
	idents := make([]string, 0, len(params))
	for i, p := range params {
		declared := ""
		if p != nil {
			declared = p.Name
		}
		name := UniqueIdent(identFor(declared, i), idents...)
		idents = append(idents, name)

		entry := emit.SigParam{Name: name, Declared: declared, Field: naming.Pascal(name)}
		if p != nil {
			entry.Source, entry.Type, entry.Variadic = p.Type, FromNode(p.Type), p.Variadic
		}
		out = append(out, entry)
	}
	return out
}

// sigReturns resolves the field name and capture local of every
// return slot.
//
// [emit.SigReturn.Error] stays false throughout: TypeScript reports
// failure by throwing, so a returned value is a value. A generator
// that read a slot as an error here would emit a check against a
// return the callable never makes.
func sigReturns(returns []*node.Return, params []emit.SigParam) []emit.SigReturn {
	taken := make(map[string]struct{}, len(params))
	for _, p := range params {
		taken[p.Name] = struct{}{}
	}

	out := make([]emit.SigReturn, 0, len(returns))
	for i, r := range returns {
		entry := emit.SigReturn{
			Field: returnField(i, len(returns)),
			Local: localFor(i, taken),
		}
		if r != nil {
			entry.Source, entry.Type = r.Type, FromNode(r.Type)
		}
		out = append(out, entry)
	}
	return out
}

// identFor picks a parameter's identifier: the declared name made
// bindable, or the positional fallback.
func identFor(declared string, index int) string {
	if declared == "" {
		return paramPrefix + strconv.Itoa(index)
	}
	return Ident(declared)
}

// returnField picks the exported field name for one return slot.
//
// A lone slot is `Result` rather than `Result0`, since a number
// distinguishes it from nothing.
func returnField(index, total int) string {
	if total == 1 {
		return resultField
	}
	return resultField + strconv.Itoa(index)
}

// localFor picks the identifier a body captures a return into,
// prefixed with an underscore when a parameter already holds it —
// shadowing a parameter would capture the wrong value.
func localFor(index int, taken map[string]struct{}) string {
	local := localPrefix + strconv.Itoa(index)
	if _, clash := taken[local]; clash {
		return "_" + local
	}
	return local
}

// IsConstraint reports whether an interface declares a type set
// rather than a method-set contract.
//
// Always false. TypeScript bounds a generic parameter with a type
// expression in the declaration that uses it — `<T extends Shape>` —
// so there is no interface whose body is a set of types, and the
// shape Go's constraint interface has does not exist to be
// recognised. Answered rather than left off [Source] because the
// question is asked of every language a doubling generator meets, and
// a language that cannot answer it is one that generator skips.
func IsConstraint(_ *node.Interface) bool { return false }
