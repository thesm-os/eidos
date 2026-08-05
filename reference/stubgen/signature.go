// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package stubgen

import (
	"strconv"

	"go.thesmos.sh/eidos/core/naming"
	refconv "go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// paramsOf lifts a method's parameters into rendered form.
//
// A parameter with no declared name gets a positional identifier so
// the generated body can reference it when recording the call — the
// same `arg<N>` fallback mockgen uses. The recorded-call field name
// is the exported form of whichever identifier ends up in use.
func paramsOf(m *node.Method) []Param {
	out := make([]Param, 0, len(m.Params))
	for i, p := range m.Params {
		name := p.Name
		if name == "" {
			name = "arg" + strconv.Itoa(i)
		}
		out = append(out, Param{
			Name:  name,
			Type:  refconv.FromNode(p.Type),
			Field: naming.Pascal(name),
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
func returnsOf(m *node.Method) []Return {
	out := make([]Return, 0, len(m.Returns))
	for i, r := range m.Returns {
		field := "Result" + strconv.Itoa(i)
		if r.Name != "" {
			field = naming.Pascal(r.Name)
		}
		out = append(out, Return{
			Name:  r.Name,
			Type:  refconv.FromNode(r.Type),
			Field: field,
		})
	}
	return out
}

// namedReturnsUsable reports whether the generated method may carry
// the source's return names on its own signature.
//
// Propagation is all-or-nothing for two independent reasons, either
// of which forces the unnamed form:
//
//   - Go requires a signature's results to be all named or all
//     anonymous, and the emit layer enforces it — a mixed slice
//     fails the render with [emit.ErrMixedNamedReturns]. A source
//     signature reaches that state legitimately: `(_ User, err
//     error)` is valid Go, and the blank identifier normalises to
//     unnamed, so the model holds one named and one unnamed slot.
//   - A return name that collides with the receiver identifier or
//     with a parameter name does not compile. Renaming around it
//     would break the correspondence the names exist to carry, so
//     the whole signature drops back to anonymous results.
//
// Falling back costs documentation on the generated signature and
// nothing else; the recorded-call struct keeps its field names.
func namedReturnsUsable(m *node.Method) bool {
	if len(m.Returns) == 0 {
		return false
	}
	taken := map[string]struct{}{receiverIdent: {}}
	for i, p := range m.Params {
		name := p.Name
		if name == "" {
			name = "arg" + strconv.Itoa(i)
		}
		taken[name] = struct{}{}
	}
	for _, r := range m.Returns {
		if r.Name == "" {
			return false
		}
		if _, clash := taken[r.Name]; clash {
			return false
		}
	}
	return true
}

// receiverIdent is the identifier the generated methods bind their
// receiver to. Declared here rather than in the template because
// [namedReturnsUsable] has to reason about collisions against it.
const receiverIdent = "s"

// withLocals assigns each return slot the identifier the generated
// body binds it to.
//
// Named results are already declared by the signature, so the local
// is the declared name. Anonymous results need a fresh local for the
// capture, which is positional — and prefixed with an underscore on
// the rare occasion a parameter already holds that identifier, since
// shadowing a parameter would record the wrong value.
func withLocals(returns []Return, params []Param, named bool) []Return {
	taken := make(map[string]struct{}, len(params))
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
