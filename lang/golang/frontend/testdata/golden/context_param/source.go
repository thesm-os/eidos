// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises context.Context detection on function
// parameters — the converter stamps go.isContext on the parameter's
// type reference whenever the underlying type resolves to
// context.Context.
package fixture

import "context"

// Fetch reads the resource identified by id, honouring cancellation
// from ctx. Drives the go.isContext stamping path for a directly-
// referenced context.Context parameter.
func Fetch(ctx context.Context, id string) error {
	_ = ctx
	_ = id
	return nil
}
