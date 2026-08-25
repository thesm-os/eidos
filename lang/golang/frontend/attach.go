// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"maps"
	"slices"
)

// attachMethods flushes the per-receiver method buffer into the
// matching [node.Struct] or [node.Alias] declarations — Go permits
// methods on any named type whose underlying is not an interface,
// so basic-typed `type X int` definitions carry method sets just
// like structs do. The receiver back-pointer was filled in at
// buffer time by [fillReceiver]; this pass only routes each method
// to its host.
//
// Receivers are visited in qname-sorted order so the resulting
// per-host method slices are byte-identical across runs. Go grammar
// guarantees every method receiver names an in-package type that
// either [convertStruct] or [convertAlias] has registered before
// this pass runs.
func (c *converter) attachMethods() {
	for _, qname := range slices.Sorted(maps.Keys(c.methodsByRecv)) {
		methods := c.methodsByRecv[qname]
		switch {
		case c.structByQName[qname] != nil:
			s := c.structByQName[qname]
			s.Methods = append(s.Methods, methods...)
		case c.aliasByQName[qname] != nil:
			a := c.aliasByQName[qname]
			a.Methods = append(a.Methods, methods...)
		}
	}
}
