// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package domain holds the core entity types referenced by every
// other package in the multipkg fixture. The generator-targeted
// types live here so cross-package emit, generic instantiation,
// and same-package elision all converge on one well-known origin.
package domain

import "time"

// ID is a named type-definition layered over string. Aliases and
// type-definitions are different node-kinds; using one here
// exercises the type-definition path through refconv and renderType
// when storage's generic Repository instantiates over `domain.ID`.
type ID string

// User is one of the headline entities. Carries repo + builder
// directives so the storage-side mock and the in-package builder
// both emit alongside the source.
//
// +gen:repo
// +gen:builder
type User struct {
	ID       ID
	Name     string
	Email    string
	Created  time.Time
	Tags     []string
	Metadata map[string]string
}

// Order references User across the same package — testing
// same-package field-type elision when User and Order are both
// emitted/referenced from generated files in this package.
//
// +gen:repo
// +gen:builder
// +gen:register
type Order struct {
	ID         ID
	BuyerID    ID
	Buyer      *User
	Items      []OrderItem
	PlacedAt   time.Time
	TotalCents int64
}

// OrderItem isn't directly generator-targeted but participates in
// Order's composition. The plain (no-directive) sibling exercises
// the case where a referenced type is *defined* in the same package
// but is not itself emit-targeted — the builder for Order must
// reference OrderItem bare (no qualifier) without a self-import.
type OrderItem struct {
	ProductID ID
	Quantity  int
	Notes     string
}

// Product carries the third generator triple — repo + builder +
// register — so the multi-generator file-composition path is
// exercised for a non-User, non-Order entity too. The
// `+gen:out product_codegen.go` directive renames the rendered
// builder file from the conventional `types_builder.go` to
// demonstrate the [pipeline.OutDirective] override path. The
// directive's `filename` parameter is positional (space-separated);
// `plugin=<name>` and `pkg=<name>` are optional key=value
// modifiers per the directive schema.
//
// Two directives are stamped:
//
//   - The unscoped `+gen:out product_codegen.go` pins the filename
//     for repogen / registrygen — they share the domain package and
//     compose into one rendered file.
//
//   - The scoped `+gen:out product_mock_test.go plugin=mockgen`
//     pins mockgen separately because mockgen's test-package
//     mode lands in `package <src>_test` and would otherwise
//     trigger the one-file-one-package invariant if it shared
//     a filename with the domain-package output.
//
// No `+gen:builder` here, and the omission is the point. An unscoped
// override pinning a *filename* cannot apply to a plugin emitting
// more than one file — there is one name and two outputs, and the
// pipeline refuses rather than silently dropping one. Builder is
// exercised on six other types across this fixture; keeping it off
// the type that carries the unscoped pin is what lets both be
// covered.
//
// +gen:repo
// +gen:register
// +gen:out product_codegen.go
// +gen:out product_mock_test.go plugin=mockgen
type Product struct {
	ID    ID
	Name  string
	Price int64
}
