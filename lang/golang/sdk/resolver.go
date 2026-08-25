// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import (
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/sdk"
)

// The reader a plugin is handed is already the resolver `lang/golang`
// asks for, so a generator needing a resolved answer passes
// `ctx.Reader` and writes no adapter:
//
//	sample, alt := golang.SampleFor(f.Type, f.Name, ctx.Reader)
//	fields, missing := golang.ExportedFieldSet(s, ctx.Reader)
//
// Asserted here rather than left to the call site because the two
// packages cannot see each other — `lang/golang` declares the port and
// must stay free of the store, `store` implements it structurally and
// must stay free of any one language. This is the only place both are
// in scope, which makes it the only place the connection can be
// stated, and a plugin author looking for "where do I get a Resolver"
// has no other way to find out that the answer is "you already have
// one".
var _ golang.Resolver = (*sdk.StoreReader)(nil)
