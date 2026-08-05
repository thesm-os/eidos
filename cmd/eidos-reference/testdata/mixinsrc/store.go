// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package mixinsrc is source-fixture input for the end-to-end
// directive test. It is parsed, never built as part of the
// workspace.
package mixinsrc

import "context"

// Store exercises the batched mixin form.
type Store interface {
	//+gen:mixin idempotent concurrent atomic
	Put(ctx context.Context, k string, v []byte) error

	//+gen:mixin bounded limit=100
	List(ctx context.Context) ([]string, error)
}
