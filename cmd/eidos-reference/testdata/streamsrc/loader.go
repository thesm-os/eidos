// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package streamsrc is source-fixture input for the shape
// end-to-end test. It is parsed, never built as part of the
// workspace.
package streamsrc

import (
	"context"
	"io"
)

// Document is the value a decode produces.
type Document struct{ N int }

// Loader exercises the stream-consuming shape alongside its
// nearest neighbours, so a misclassification shows as a swap
// rather than an absence.
type Loader interface {
	Load(ctx context.Context, r io.Reader) (int, error)
	Decode(ctx context.Context, r io.Reader) (Document, error)
	Get(ctx context.Context, id string) (Document, error)
	Put(ctx context.Context, d Document) error
}

// ID is a write receipt.
type ID string

// Registry exercises the receipt-returning write, which no shape
// expresses.
type Registry interface {
	Insert(ctx context.Context, d Document) (ID, error)
}
