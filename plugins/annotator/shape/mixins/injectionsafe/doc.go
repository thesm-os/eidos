// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package injectionsafe recognises the injectionsafe mixin — the assertion that
// untrusted input reaches an interpreter as data rather than as syntax.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin injectionsafe
//
// `read=` names the callable that reads the value back. The claim is
// about what an interpreter downstream does with the input, and the
// only signature-level evidence is the round trip: a payload that
// would be syntax comes back as the data it was written as.
//
// `unsafe=` names that payload — the value the round trip is made
// of, which no derivation can produce: which separator or quote is
// syntax depends on an interpreter only the author knows, and a
// drawn sample carries nothing dangerous, so a subject sanitising
// nothing passes on it. Optional; declare it as
// `unsafe=HostilePayload` beside `read=`.
package injectionsafe
