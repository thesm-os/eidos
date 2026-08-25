// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go.thesmos.sh/eidos/core/meta"
)

// MetaFrontend stamps the producing frontend's plugin name on every
// [node.Package] this frontend emits — the string "typescript".
//
// The one key this package declares rather than reading from
// `lang/typescript`, and the one exception to the `ts.*` namespace:
// it carries the bare name `frontend` because it is a cross-frontend
// convention rather than a TypeScript fact. Every frontend declares
// the same key independently through [meta.EnsureKey], which is what
// keeps them from depending on each other's init order — [meta.NewKey]
// panics on a duplicate, so whichever package's init ran second would
// take the process down once both were linked into one binary.
//
// Consumers filter a mixed store by it: a pipeline reading both Go
// and TypeScript sources has two frontends writing into one store,
// and this is what tells their packages apart.
var MetaFrontend = meta.EnsureKey(
	"frontend",
	meta.StringParser,
) //nolint:gochecknoglobals // shared registry-singleton key
