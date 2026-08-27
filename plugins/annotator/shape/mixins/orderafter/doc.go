// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package orderafter recognises the order-after mixin — the
// assertion that the annotated callable's effect is observable
// only after the named sibling has run. The `fn` param names the
// sibling; the resolver rewrites it into a qualified name.
//
// The recognised directive is:
//
//	//+gen:mixin orderafter fn=Initialise unready=ErrNotReady
package orderafter
