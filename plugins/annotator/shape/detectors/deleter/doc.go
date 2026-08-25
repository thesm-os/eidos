// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package deleter recognises the delete shape — a callable named for
// removal that takes the key of the entity to remove and reports only
// an error.
//
// Structurally this is [writer]: one non-context parameter, a bare
// error out. Nothing in the signature separates `Delete(ctx, key
// string) error` from `Put(ctx, v string) error`, so the name is the
// discrimination, as it is for [closer] — the only other detector
// that reads one.
//
// The classification matters because the two license opposite
// checks. A law selecting writers derives write-then-read-back and
// expects the value present; run against a delete it asserts the
// reverse of correct behaviour. The `deleteremoves` mixin states the
// removal directly and was, until this detector, the only way a
// consumer could tell the two apart.
//
// The key records as `shape.key_type` rather than `shape.value_type`,
// which is what [writer] recorded when a delete fell through to it —
// naming the key it addresses as the value it stores.
//
// The recognised shapes are:
//
//	Delete(ctx context.Context, key string) error
//	Remove(key string) error
package deleter
