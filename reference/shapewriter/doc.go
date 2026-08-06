// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package shapewriter detects structs that satisfy the io.Writer
// shape — a method named `Write` taking a `[]byte` and returning
// `(int, error)`. Downstream generators read the stamped metadata
// (`shape.writer.detected`, `shape.writer.method`) to decide
// whether their codegen path applies to a given struct.
//
// Detection is heuristic by default; directive overrides force or
// suppress the detection regardless of the heuristic outcome:
//
//   - `+gen:writer` forces detection on the host struct, even
//     when no method matches the canonical signature.
//   - `-gen:writer` suppresses detection on the host struct, even
//     when the method exists with the matching signature.
//
// # Meta-key catalog
//
// [Plugin.OnStruct] stamps both keys at plugin authority,
// attributed to [Name], on every struct the annotator walk visits
// — writer and non-writer alike. Presence therefore carries no
// signal; consumers read the value.
//
//   - shape.writer.detected — bool, exported as [Detected]. True
//     when the canonical signature matched or `+gen:writer`
//     forced it; false when nothing matched or `-gen:writer`
//     suppressed it. The false case is written rather than
//     omitted, so an absent key means the annotator never ran,
//     not that the struct failed the heuristic.
//   - shape.writer.method — string, exported as [MethodQName].
//     The store's method-bucket key for the matched method,
//     `<ownerQName>.Write`, so a consumer resolves the method
//     node by qname instead of re-running the match. Empty
//     whenever the heuristic did not match — including the
//     `+gen:writer` override, which pairs detected=true with an
//     empty back-link because there is no real method to point
//     at. Guard on the empty string before using the value as a
//     lookup key.
package shapewriter
