// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package cursor recognises the cursor contract — a next-element
// reader paired with the close callable that releases the underlying
// iterator's resources.
//
// The contract has two arms. A cursor that declares itself carries the
// directive on its reader, and names its siblings as partner roles:
//
//	//+gen:contract cursor role=next close=Close sentinel=ErrClosed
//	Next() bool
//
// A cursor that is produced carries it on the factory, and names the
// handle's methods as members of the type the factory answers:
//
//	//+gen:contract cursor role=open next=Next close=Close sentinel=ErrClosed
//	Scan(ctx context.Context) (Cursor, error)
//
// `next` and `close` are the same words in both, resolved through
// different scopes — see [Params]. `open` requires `next=`, because a
// factory is the one arm where the reader is not the host.
//
// The producer arm does not back-stamp: `Cursor.Next` and
// `Cursor.Close` gain no membership from it, since member references
// are not partner references. Read the produced cursor's protocol off
// the `open` host's params rather than off the handle's methods.
package cursor
