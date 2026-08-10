// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strconv"

	"go.thesmos.sh/eidos/node"
)

// ParamIdent returns the identifier a generated body binds a
// parameter to.
//
// A source signature need not name its parameters — `Read([]byte)`
// is legal Go — but a generated body that references one must call
// it something. The positional fallback is what every generator
// ends up writing, and three in this repository wrote it
// independently.
//
// The declared name wins where there is one, and is made safe: a
// parameter called `type` or `len` is legal in the source signature
// and not in a body that uses it.
func ParamIdent(p *node.Param, index int) string {
	if p == nil {
		return "arg" + strconv.Itoa(index)
	}
	if p.Name == "" || p.Name == "_" {
		return "arg" + strconv.Itoa(index)
	}
	return SafeIdent(p.Name)
}

// ParamIdents returns the identifiers for a whole parameter list,
// each unique within it.
//
// Uniqueness matters because the positional fallback and a declared
// name can collide: `Read(arg0 []byte, []byte)` names the first
// parameter exactly what the second would fall back to, and two
// parameters of one name do not compile.
func ParamIdents(params []*node.Param) []string {
	names := make([]string, len(params))
	for i, p := range params {
		if p != nil {
			names[i] = p.Name
		}
	}
	return ParamIdentsFor(names)
}

// ParamIdentsFor is [ParamIdents] for a caller holding the declared
// names and no [node.Param].
//
// A generator that projects an emit-side and a source-side signature
// onto one internal shape has lowered away the [node.Param] before it
// needs the identifiers. Without this it either fabricates a
// `&node.Param{Name: n}` to satisfy the signature — which is the API
// telling the caller it is the wrong shape — or reaches past this to
// [ParamIdent] per item and loses the uniqueness pass, which is the
// half that keeps `Read(arg0 []byte, []byte)` compiling.
//
// An empty name takes the positional fallback, exactly as an unnamed
// [node.Param] does.
func ParamIdentsFor(names []string) []string {
	out := make([]string, 0, len(names))
	for i, name := range names {
		out = append(out, UniqueIdent(ParamIdent(&node.Param{Name: name}, i), out...))
	}
	return out
}

// ErrorSlot returns the index of the return slot carrying the
// builtin error, or -1 when the signature returns none.
//
// Found by asking each slot rather than by taking the last one. A
// signature returning `(error, string)` is unusual but legal Go,
// and a positional rule would bind the wrong slot without failing
// to compile — the generated code would check a string for
// nil-ness or assign an error where a value was expected.
//
// The first error slot wins when a signature declares several,
// which is the one a caller checks.
func ErrorSlot(returns []*node.Return) int {
	for i, r := range returns {
		if r != nil && IsError(r.Type) {
			return i
		}
	}
	return -1
}

// ReturnsError reports whether a signature returns the builtin
// error in any position.
func ReturnsError(returns []*node.Return) bool { return ErrorSlot(returns) >= 0 }

// NamedReturnsUsable reports whether a generated signature may
// carry the source's return names.
//
// Propagation is all-or-nothing for two independent reasons, either
// of which forces the anonymous form:
//
//   - Go requires a signature's results to be all named or all
//     anonymous, and the emit layer enforces it. A source signature
//     reaches the mixed state legitimately: `(_ User, err error)`
//     is valid Go and the blank identifier normalises to unnamed,
//     so the model holds one named slot and one unnamed.
//   - A return name colliding with the receiver identifier or with
//     a parameter name does not compile. Renaming around it would
//     break the correspondence the names exist to carry, so the
//     whole signature drops back to anonymous.
//
// Falling back costs documentation on the generated signature and
// nothing else.
func NamedReturnsUsable(returns []*node.Return, taken ...string) bool {
	if len(returns) == 0 {
		return false
	}
	used := make(map[string]struct{}, len(taken))
	for _, t := range taken {
		used[t] = struct{}{}
	}
	for _, r := range returns {
		if r == nil || r.Name == "" {
			return false
		}
		if _, clash := used[r.Name]; clash {
			return false
		}
		used[r.Name] = struct{}{}
	}
	return true
}
