// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

import (
	"go.thesmos.sh/eidos/core/contract"
)

// Decorators returns the decorators applied to n, in source order.
//
// Order is part of what a decorator means: expressions evaluate
// top-down and apply bottom-up, so a list read out of order describes
// a different composition than the one written.
func Decorators(n contract.Node) []Decorator {
	if n == nil {
		return nil
	}
	out, _ := MetaDecorators.Get(n.Meta())
	return out
}

// DecoratorNamed returns the first decorator on n with the given
// name.
//
// First rather than only, because a name may appear more than once —
// see [DecoratorsNamed] for the whole set. A consumer reading a
// decorator that is applied once, which is most of them, wants this.
func DecoratorNamed(n contract.Node, name string) (Decorator, bool) {
	for _, d := range Decorators(n) {
		if d.Name == name {
			return d, true
		}
	}
	return Decorator{}, false
}

// DecoratorsNamed returns every decorator on n with the given name,
// in source order.
//
// Repetition is legal and load-bearing: a route documenting several
// responses applies the same decorator once per status code, and a
// consumer collapsing those to one would describe an endpoint that
// returns a single status.
func DecoratorsNamed(n contract.Node, name string) []Decorator {
	var out []Decorator
	for _, d := range Decorators(n) {
		if d.Name == name {
			out = append(out, d)
		}
	}
	return out
}

// HasDecorator reports whether n carries the named decorator.
func HasDecorator(n contract.Node, name string) bool {
	_, ok := DecoratorNamed(n, name)
	return ok
}

// DecoratorNames returns the names of n's decorators in source order,
// repetitions included.
func DecoratorNames(n contract.Node) []string {
	decorators := Decorators(n)
	out := make([]string, 0, len(decorators))
	for _, d := range decorators {
		out = append(out, d.Name)
	}
	return out
}
