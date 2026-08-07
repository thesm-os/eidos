// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/core/naming"
)

// The expression lists a Go template writes around a signature.
//
// Every generator rendering a method spells the same lists: the
// argument list of a forwarding call, the field assignments of a
// recorded call, the locals a result is captured into. Written as
// template `define` blocks they are duplicated per generator and
// the duplicates are silent — a copy that picks the wrong return
// slot generates code that compiles and asserts the wrong thing.
//
// Written here they are ordinary functions: unit-tested,
// compile-checked, and shared without one generator naming
// another's templates. The backend merges every plugin's template
// define names into one tree, so a shared define would make one
// plugin's private template name another plugin's API, and a rename
// would break output at render time rather than at build time.
//
// # Hazards
//
// Nothing here can render a type. `renderType` is bound to the
// backend's render state because it registers the file's imports
// and elides same-package qualifiers; a free function reproduces
// neither, and one that formatted types itself would name packages
// the rendered file never imports. Type-bearing lists therefore
// stay as template constructs or go through [EmitParams] and the
// emit tree.
//
// Every function takes a [Sig]. A caller passing a hand-built slice
// is reconstructing the field, local and error-slot conventions
// that projection owns, which is the duplication this removes.

// listSep is the separator every list here uses.
//
// Fixed rather than a parameter because each of these is a Go
// expression list and the backend runs gofmt over the result — a
// caller choosing its own separator would be choosing formatting
// the formatter overrides.
const listSep = ", "

// Args renders the parameters as an argument list — `ctx, id`, or
// `ctx, keys...` where the tail is variadic.
//
// The ellipsis is not decoration: forwarding a variadic parameter
// without it passes the slice as a single element, which
// type-checks against `...any` and silently records one argument
// where the caller passed several.
func Args(s *Sig) string {
	if s == nil {
		return ""
	}
	return join(len(s.Params), func(i int) string {
		if s.Params[i].Variadic {
			return s.Params[i].Name + "..."
		}
		return s.Params[i].Name
	})
}

// ParamNames renders the parameter identifiers with no spread —
// `ctx, keys`.
//
// The declaration form, where an ellipsis would be a syntax error.
// Distinct from [Args], which is the call form.
func ParamNames(s *Sig) string {
	if s == nil {
		return ""
	}
	return join(len(s.Params), func(i int) string { return s.Params[i].Name })
}

// Idents renders n positional identifiers under prefix —
// `Idents("a", 2)` gives `a0, a1`.
//
// Positional rather than named because these stand for values a
// generated check never inspects by name: the arguments a dispatch
// test passes, the results it boxes. A name would suggest a meaning
// the value does not carry.
func Idents(prefix string, n int) string {
	return join(n, func(i int) string { return prefix + strconv.Itoa(i) })
}

// IdentArgs renders positional identifiers as an argument list,
// spreading a variadic tail — `a0, a1...`.
//
// Separate from [Idents] because a call site needs the spread and a
// declaration list must not have it.
func IdentArgs(prefix string, s *Sig) string {
	if s == nil {
		return ""
	}
	return join(len(s.Params), func(i int) string {
		ident := prefix + strconv.Itoa(i)
		if s.Params[i].Variadic {
			return ident + "..."
		}
		return ident
	})
}

// Blanks renders n discards — `_, _`.
func Blanks(n int) string {
	return join(n, func(int) string { return "_" })
}

// CallFields renders the parameters as recorded-call field
// assignments — `Ctx: ctx, ID: id`.
func CallFields(s *Sig) string {
	if s == nil {
		return ""
	}
	return join(len(s.Params), func(i int) string {
		return s.Params[i].Field + ": " + s.Params[i].Name
	})
}

// Locals renders the capture identifiers a result binds to —
// `r0, r1`, or the source's own names where the signature declares
// them.
func Locals(s *Sig) string {
	if s == nil {
		return ""
	}
	return join(len(s.Returns), func(i int) string { return s.Returns[i].Local })
}

// LocalFields renders the return tuple built from those captures —
// `Result: r0, Err: r1`.
func LocalFields(s *Sig) string {
	if s == nil {
		return ""
	}
	return join(len(s.Returns), func(i int) string {
		return s.Returns[i].Field + ": " + s.Returns[i].Local
	})
}

// IdentFields renders the return tuple built from positional
// identifiers — `IdentFields("got", …)` gives
// `Result: got0, Err: got1`.
func IdentFields(prefix string, s *Sig) string {
	if s == nil {
		return ""
	}
	return join(len(s.Returns), func(i int) string {
		return s.Returns[i].Field + ": " + prefix + strconv.Itoa(i)
	})
}

// NamedFields renders the return tuple built from consumer-facing
// parameter names — `Result: result, Err: err`.
//
// Named after the recorded-call fields rather than after the
// internal capture locals: this is the surface a caller of a
// generated setter reads, and it should read as the thing being
// set.
func NamedFields(s *Sig) string {
	if s == nil {
		return ""
	}
	return join(len(s.Returns), func(i int) string {
		return s.Returns[i].Field + ": " + naming.Camel(s.Returns[i].Field)
	})
}

// Reads renders the return tuple read back off a held value —
// `r.Result, r.Err`.
func Reads(recv string, s *Sig) string {
	if s == nil {
		return ""
	}
	return join(len(s.Returns), func(i int) string {
		return recv + "." + s.Returns[i].Field
	})
}

// Fails renders an assignment list binding only the error slot and
// discarding the rest — `_, err`.
//
// The error slot is found by flag rather than by position. A
// signature returning `(error, string)` is unusual but legal Go,
// and a positional rule would bind the wrong local without failing
// to compile.
func Fails(s *Sig) string {
	if s == nil {
		return ""
	}
	return join(len(s.Returns), func(i int) string {
		if s.Returns[i].Error {
			return s.Returns[i].Local
		}
		return "_"
	})
}

// ZeroArgs renders one zero literal per parameter, or the empty
// string when any parameter's zero is not derivable.
//
// All-or-nothing because the list is a call's whole argument set: a
// partial one does not compile, so a caller that cannot fill every
// position must emit no call rather than a shorter one.
func ZeroArgs(s *Sig) (string, bool) {
	if s == nil {
		return "", false
	}
	parts := make([]string, 0, len(s.Params))
	for i := range s.Params {
		lit, ok := ZeroLiteral(s.Params[i].Source)
		if !ok {
			return "", false
		}
		parts = append(parts, lit)
	}
	return strings.Join(parts, listSep), true
}

// join renders n comma-separated entries produced by at.
func join(n int, at func(i int) string) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range n {
		parts[i] = at(i)
	}
	return strings.Join(parts, listSep)
}
