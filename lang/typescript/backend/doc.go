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
// # Imports
//
// The package keeps its own import set rather than using
// [writer.ImportSet]. That models one alias per path, which is the
// whole of a Go import; TypeScript binds a set of names per
// specifier, plus an optional default and namespace, any of which may
// be type-only. See importSet.
package backend
