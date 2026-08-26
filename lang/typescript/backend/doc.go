// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package backend renders emit graphs to TypeScript source.
//
// It implements [plugin.Backend] and exposes the extension surface a
// plugin author interacts with: a canonical template set, a canonical
// funcmap, slot composition, and an envelope with header and footer
// extension points.
//
// # Formatting without a formatter
//
// The Go backend ends in [go/format.Source], which does two jobs: it
// makes output canonical, and it rejects output that is not valid Go.
// No Go-native TypeScript formatter exists, so the two are answered
// separately.
//
// Canonical form comes from templates that emit it directly, plus a
// normaliser for what a template cannot express cleanly — the import
// block's arrangement, blank-line runs, trailing whitespace. Each
// choice has exactly one value, because byte-identical output admits
// no second one: single quotes, semicolons always, two-space indent,
// trailing commas on multi-line lists.
//
// Line wrapping is absent by decision. Matching Prettier means
// implementing its layout algorithm, which is a full printer and the
// largest component this package could acquire; unwrapped output is
// deterministic and cheap, and a consumer's own formatter will
// rewrite it anyway, since a generated file's line breaks are not a
// contract.
//
// Validity is not checked here. Parsing TypeScript needs tree-sitter,
// tree-sitter lives in the frontend, and depguard forbids a backend
// importing a frontend — correctly, since the alternative is a
// backend that carries a C toolchain. The check lives in the
// acceptance tier, which parses every generated file and fails on a
// syntax error. A broken template therefore fails under test rather
// than on every run, which is a real reduction against the Go
// backend and is worth knowing.
//
// # Declarations, not bodies
//
// The backend renders no statement and no method body, which decides
// the class spelling: a bodiless method in a plain class is TS2391
// and an uninitialised property TS2564, so every class is `declare`d.
// Runtime constructs the expression renderer can spell trivially —
// an enum member's value, a constant's initialiser — are spelled, and
// the initialised form drops `declare` because an ambient declaration
// admits none.
//
// # The ts.* vocabulary, and what becomes of it here
//
// Rendered: visibility (whatever was stamped, `public` included — a
// stamped key is one the author wrote), `static`, `abstract`,
// `readonly`, `?` on properties, parameters and methods, `get`/`set`
// accessors, `#` hard-private names, overload signatures (in place of
// the derived signature — the implementation's is a body fact),
// index and construct signatures, `const enum`, type-parameter
// defaults, and initialisers on variables and constants.
//
// Absorbed before this backend runs: the union / intersection / tuple
// markers, operator and literal type text, nullability and mapped
// types all ride the ref shapes [typescript.FromNode] projects, and
// the import-graph keys (re-exports, specifiers, type-only) describe
// the source module graph rather than the emit graph.
//
// Not rendered, deliberately:
//
//   - `async` and generators: illegal on an interface method and in
//     an ambient class alike. Both say how a body produces its
//     result; the Promise or iterator return type is the contract.
//   - Decorators: runtime metadata on runtime classes, which an
//     ambient declaration cannot carry.
//   - Definite assignment (`!`): an assertion about a body's
//     initialisation order, rejected in ambient contexts.
//   - `export default` and namespaces: authoring-style facts the
//     graph records for readers; generated modules use named exports
//     at the top level.
//
// # Imports
//
// The package keeps its own import set rather than using
// [writer.ImportSet]. That models one alias per path, which is the
// whole of a Go import; TypeScript binds a set of names per
// specifier, plus an optional default and namespace, any of which may
// be type-only. See importSet.
package backend
