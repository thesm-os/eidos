// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package stubgen

import (
	"strconv"

	refconv "go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/sdk"
)

// paramsOf lifts a method's parameters into rendered form.
//
// Naming runs through [refconv.ParamIdents] rather than a local
// fallback: the positional rule, the keyword adjustment, and the
// uniqueness pass are Go's, not this generator's, and three
// generators here had each written their own slice of them. The
// recorded-call field name is the exported form of whichever
// identifier ends up in use.
//
// The variadic flag travels beside the element type rather than being
// folded into it: the four places the parameter is rendered want four
// different spellings, and only the source knows which parameter is
// the tail. See [Param.Variadic].
func paramsOf(m *sdk.Method) []Param {
	idents := refconv.ParamIdents(m.Params)
	out := make([]Param, 0, len(m.Params))
	for i, p := range m.Params {
		out = append(out, Param{
			Name:     idents[i],
			Type:     refconv.FromNode(p.Type),
			Field:    refconv.ExportedName(idents[i]),
			Variadic: refconv.IsVariadic(p),
		})
	}
	return out
}

// returnsOf lifts a method's return slots into rendered form.
//
// Field is populated for every slot regardless of whether the source
// named it: a recorded-call struct needs one field per return, and
// the fallback is positional. This is deliberately independent of
// [namedReturnsUsable] — a signature that cannot carry names on the
// generated method still records both types under readable-where-
// possible field names.
func returnsOf(m *sdk.Method) []Return {
	out := make([]Return, 0, len(m.Returns))
	for i, r := range m.Returns {
		field := "Result" + strconv.Itoa(i)
		if r.Name != "" {
			field = refconv.ExportedName(r.Name)
		}
		out = append(out, Return{
			Name:  r.Name,
			Type:  refconv.FromNode(r.Type),
			Field: field,
		})
	}
	return out
}

// receiverIdentFor returns the identifier the generated method binds
// its receiver to, chosen around the parameter identifiers already
// spoken for.
//
// Derived per method rather than fixed at `s`, because the source
// picks the parameter names and this generator picks nothing: an
// interface declaring `Recv(s string)` produced
// `func (s *StoreStub) Recv(s string)`, where every `s.<Field>` in
// the body resolved to the parameter. That does not compile, and no
// substring assertion over the rendered file could see it.
//
// [refconv.ReceiverIdent] owns the convention — the type name's
// initial, lower-cased, disambiguated on collision — so a reader of
// the generated code sees the same receiver shape every other
// generator here emits.
func receiverIdentFor(typeName string, params []Param) string {
	taken := make([]string, 0, len(params))
	for _, p := range params {
		taken = append(taken, p.Name)
	}
	return refconv.ReceiverIdent(typeName, taken...)
}

// namedReturnsUsable reports whether the generated method may carry
// the source's return names on its own signature.
//
// The rule itself is Go's and lives in [refconv.NamedReturnsUsable];
// what this generator supplies is the occupied scope — the receiver
// identifier it just chose plus the parameter identifiers, which is
// knowledge no shared helper has.
//
// Falling back costs documentation on the generated signature and
// nothing else; the recorded-call struct keeps its field names.
func namedReturnsUsable(m *sdk.Method, recv string, params []Param) bool {
	taken := make([]string, 0, len(params)+1)
	taken = append(taken, recv)
	for _, p := range params {
		taken = append(taken, p.Name)
	}
	return refconv.NamedReturnsUsable(m.Returns, taken...)
}

// withLocals assigns each return slot the identifier the generated
// body binds it to.
//
// Named results are already declared by the signature, so the local
// is the declared name. Anonymous results need a fresh local for the
// capture, which is positional — and prefixed with an underscore on
// the rare occasion a parameter or the receiver already holds that
// identifier, since shadowing either records the wrong value or
// breaks the delegate call.
func withLocals(returns []Return, params []Param, recv string, named bool) []Return {
	taken := make(map[string]struct{}, len(params)+1)
	if recv != "" {
		taken[recv] = struct{}{}
	}
	for _, p := range params {
		taken[p.Name] = struct{}{}
	}
	for i := range returns {
		if named {
			returns[i].Local = returns[i].Name
			continue
		}
		local := "r" + strconv.Itoa(i)
		if _, clash := taken[local]; clash {
			local = "_" + local
		}
		returns[i].Local = local
	}
	return returns
}
