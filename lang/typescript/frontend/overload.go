// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// signature is what the fold needs to know about one converted
// callable and the graph does not carry.
//
// Held on the converter rather than stamped, because it answers a
// question that stops existing once the fold has run: afterwards a
// declaration's signature is its Params and Returns, and the
// alternatives are on [typescript.MetaOverloads]. A stamped copy
// would be a second, staler answer to both, and a tombstoned one
// would still show up in the bag's key list.
type signature struct {
	// text is the declaration's signature as written, without the
	// body or the trailing semicolon.
	text string

	// hasBody reports whether an implementation was declared.
	hasBody bool
}

// foldOverloads collapses same-named functions in decls into one
// declaration apiece, carrying the alternatives on metadata.
//
// TypeScript writes an overloaded function as several bodiless
// signatures followed by one implementation. Each parses as its own
// declaration, but they are one callable — and the model keys a
// declaration by qualified name, so leaving them separate produces
// several Functions named alike and the store rejects the package for
// holding a duplicate.
//
// The implementation survives, because its signature is the one a
// body was written against. An ambient set has no implementation, so
// the first signature survives instead.
func (c *conv) foldOverloads(decls []node.Node) []node.Node {
	index := map[string]int{}
	out := make([]node.Node, 0, len(decls))

	for _, d := range decls {
		fn, ok := d.(*node.Function)
		if !ok {
			out = append(out, d)
			continue
		}

		at, seen := index[fn.Name]
		if !seen {
			index[fn.Name] = len(out)
			out = append(out, fn)
			continue
		}

		kept, _ := out[at].(*node.Function)
		out[at] = c.mergeOverload(kept, fn)
	}
	return out
}

// mergeOverload folds later into kept and returns whichever survives
// as the declaration.
func (c *conv) mergeOverload(kept, later *node.Function) *node.Function {
	existing, _ := typescript.MetaOverloads.Get(kept.Meta())
	keptSig := c.signatures[kept]
	laterSig := c.signatures[later]

	if laterSig.hasBody && !keptSig.hasBody {
		// The implementation arriving after its signatures. It takes
		// over as the declaration, and the signature it displaces
		// joins the alternatives.
		//
		// Prepended, not appended: the displaced signature was the
		// first one written, and the alternatives already collected
		// all followed it. TypeScript resolves an overloaded call
		// against the signatures top-down and takes the first that
		// matches, so a list out of source order describes a callable
		// that resolves differently.
		merged := append([]typescript.Overload{{Text: keptSig.text}}, existing...)
		c.setOverloads(later, merged)
		return later
	}

	c.setOverloads(kept, append(append([]typescript.Overload{}, existing...),
		typescript.Overload{Text: laterSig.text}))
	return kept
}

// setOverloads records the alternatives on fn, dropping any that
// duplicate fn's own signature.
//
// Three signatures plus an implementation yield three alternatives,
// not four: the implementation's own spelling is not one of the ways
// it may be called.
func (c *conv) setOverloads(fn *node.Function, overloads []typescript.Overload) {
	own := c.signatures[fn].text
	kept := make([]typescript.Overload, 0, len(overloads))
	for _, o := range overloads {
		if o.Text == "" || o.Text == own {
			continue
		}
		kept = append(kept, o)
	}
	if len(kept) == 0 {
		return
	}
	typescript.MetaOverloads.SetAt(
		fn.EnsureMeta(), kept, meta.AuthorityPlugin, FrontendName, fn.Pos(),
	)
}

// foldMethodOverloads applies the same collapse to a declaration's
// methods.
//
// Takes and returns the slice rather than a host, because a class and
// an interface both declare methods and both may overload them; the
// fold has no reason to know which one it is serving.
//
// A class or interface body may overload a method exactly as a module
// overloads a function. The store does not key methods by qualified
// name, so this does not fail a run the way the function case does —
// it produces a method list with repeats instead, and a generator
// emitting one wrapper per entry would emit several for one method.
func (c *conv) foldMethodOverloads(methods []*node.Method) []*node.Method {
	if len(methods) < 2 {
		return methods
	}
	index := map[string]int{}
	out := make([]*node.Method, 0, len(methods))

	for _, m := range methods {
		at, seen := index[m.Name]
		if !seen {
			index[m.Name] = len(out)
			out = append(out, m)
			continue
		}
		out[at] = c.mergeMethodOverload(out[at], m)
	}
	return out
}

// mergeMethodOverload is [conv.mergeOverload] for methods.
func (c *conv) mergeMethodOverload(kept, later *node.Method) *node.Method {
	existing, _ := typescript.MetaOverloads.Get(kept.Meta())
	keptSig := c.methodSignatures[kept]
	laterSig := c.methodSignatures[later]

	if laterSig.hasBody && !keptSig.hasBody {
		merged := append([]typescript.Overload{{Text: keptSig.text}}, existing...)
		c.setMethodOverloads(later, merged)
		return later
	}

	c.setMethodOverloads(kept, append(append([]typescript.Overload{}, existing...),
		typescript.Overload{Text: laterSig.text}))
	return kept
}

// setMethodOverloads is [conv.setOverloads] for methods.
func (c *conv) setMethodOverloads(m *node.Method, overloads []typescript.Overload) {
	own := c.methodSignatures[m].text
	kept := make([]typescript.Overload, 0, len(overloads))
	for _, o := range overloads {
		if o.Text == "" || o.Text == own {
			continue
		}
		kept = append(kept, o)
	}
	if len(kept) == 0 {
		return
	}
	typescript.MetaOverloads.SetAt(
		m.EnsureMeta(), kept, meta.AuthorityPlugin, FrontendName, m.Pos(),
	)
}
