// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises the directive-parsing surface across
// every declaration kind the frontend extracts directives from:
// the package itself, individual files, top-level types, struct
// fields, methods, functions, and individual parameters. Each row
// pins one representative directive shape so downstream code locks
// in the wire format.
//
// +gen:scope kind=package owner=core
package fixture

// User is the domain object the directives below decorate. The
// struct itself carries one directive in set form and one in
// negate form so both [directive.Directive.Negated] values appear
// in the golden.
//
// +gen:mock target=UserRepo
// -gen:exported
type User struct {
	// gen:column name="user_id" primary
	ID int
	// +gen:column name="display_name"
	Name string
}

// Save persists u. The method carries a single directive mixing
// positional arguments and key=value pairs; the parameter list is
// laid out across multiple lines so a per-parameter directive
// attaches via the converter's ast.CommentMap index.
//
// gen:rpc verb=POST path=/users idempotent
func (u *User) Save(
	// gen:redact reason="pii"
	ctx Context,
) error {
	_ = ctx
	return nil
}

// Context is a stand-in interface that exists only so the Save
// receiver method has a non-stdlib parameter type. Carries no
// directives intentionally — the golden negative case.
type Context interface{ Done() }

// Greet returns a hello string. The function carries a single
// neutral-form directive without arguments — the smallest valid
// directive shape.
//
// gen:public
func Greet() string { return "hi" }
