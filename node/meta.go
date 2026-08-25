// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import "go.thesmos.sh/eidos/core/meta"

// MetaFrontend is the key every frontend stamps on the packages it
// produces, naming the language it parsed — `"golang"`, `"protobuf"`,
// `"typescript"`.
//
// The one fact about a package that no single frontend owns: a bridge
// filters its walk by it, a cross-namespace audit reads it, and a
// plugin asks it to find which language's rules apply to what it is
// looking at. Everything that reads it has to agree with everything
// that writes it.
//
// # Why here
//
// A meta key is interned by name, so a package that re-declares it by
// string forfeits the compile-time link to everyone else using it —
// the key resolves to one singleton by luck of the shared literal, and
// a rename in any one declaration mints a second key that reads empty
// from every writer.
//
// Four packages had done exactly that: the façade and all three
// frontends. The façade was the wrong home for the fix, because a
// frontend sits below it and cannot import it — which is why the
// declaration there never displaced the copies it was written to
// replace. This package is below every frontend and below the façade,
// and a [Package] is what carries the stamp, so there is nowhere left
// for a fifth copy to be needed.
//
// # Why EnsureKey
//
// A second declaration resolves to this one rather than failing. An
// out-of-tree frontend written before this existed spells the name
// itself, and the tolerant constructor is what keeps it stamping the
// same key rather than a private one nothing reads.
//
// # Why no doc audit covers this
//
// The audit asks whether a package's doc.go mentions each key it
// declares, by substring with a right-hand boundary check. This key is
// the bare word `frontend`, and this package's documentation discusses
// frontends — so the audit matches prose that predates the key and
// passes whatever the catalog says. A check that cannot fail is worse
// than none, so the documentation above is held by review.
//
//nolint:gochecknoglobals // meta key registration, immutable after init.
var MetaFrontend = meta.EnsureKey("frontend", meta.StringParser)
