// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"strconv"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
)

// setTagEntry stamps a struct-tag key/value pair on a [meta.Bag]
// with full provenance — author "golang" at AuthorityPlugin with
// the field's source position. The key argument is the
// fully-prefixed name (callers pass [MetaTagPrefix]+name);
// [meta.EnsureKey] is used because tag names ("json", "db",
// "yaml", …) are not known at compile time, and the dynamic
// registration path returns the existing registry entry on
// duplicate.
func setTagEntry(bag *meta.Bag, key, value string, pos position.Pos) {
	tagKey := meta.EnsureKey(key, meta.StringParser)
	tagKey.SetAt(bag, value, meta.AuthorityPlugin, FrontendName, pos)
}

// strconvUnquoteWrapped wraps [strconv.Unquote] without leaking the
// wrap-error chain through every call site. Used by the struct-tag
// parser, which reads quoted values produced by reflect.StructTag.
func strconvUnquoteWrapped(s string) (string, error) {
	return strconv.Unquote(s) //nolint:wrapcheck // strconv error already self-describing for tag-parse callers
}
