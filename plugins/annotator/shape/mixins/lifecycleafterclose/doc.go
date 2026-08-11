// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package lifecycleafterclose recognises the
// lifecycle-after-close mixin — the assertion that the annotated
// callable continues to behave correctly (typically returning a
// sentinel error) when invoked after the host has been closed.
//
// The `close` param names the callable that closes the host. Go
// convention spells it Close, but a host spelling it Shutdown needs
// to say so.
//
// The param is optional: the bare form still classifies the
// callable, and a consumer wanting only the classification writes
// it. A generated check that has to call the partner needs it
// named, and an unresolvable name is reported by the resolver.
//
// The recognised directive is:
//
//	//+gen:mixin lifecycleafterclose close=Close
package lifecycleafterclose
