// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"strings"

	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/typescript"
)

// boolKey is the shape every one-word modifier shares — `readonly`,
// `static`, `async`, `abstract`.
//
// Named so the per-builder mark helpers take one parameter instead of
// spelling the generic instantiation at each call site, which is the
// only thing that makes a modifier a one-line method.
type boolKey = meta.Key[bool]

// appendDecorator adds one decorator to n's ordered list.
//
// Read-modify-write rather than a set: order and repetition both
// carry meaning in TypeScript, so a second `@ApiResponse` has to land
// after the first rather than replace it. The read goes through
// [typescript.Decorators], which is the accessor a consumer uses, so
// a fixture and a frontend produce a list nothing downstream can tell
// apart.
func appendDecorator(n contract.Node, name string, args []string) {
	list := typescript.Decorators(n)
	list = append(list, typescript.Decorator{Name: name, Args: joinArgs(args)})
	typescript.MetaDecorators.Set(n.EnsureMeta(), list, markerAuthority)
}

// joinArgs spells a decorator's argument list in the verbatim form
// the model carries — parentheses included, empty for a bare `@deco`
// applied without a call.
//
// The two are different declarations: `@Injectable` and
// `@Injectable()` both compile, and a framework reading the metadata
// reflection API sees them differently. Passing no arguments
// therefore produces the bare form rather than `()`.
func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return "(" + strings.Join(args, ", ") + ")"
}
