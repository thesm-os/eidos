// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package sdk is the plugin base every TypeScript-generating plugin
// embeds.
//
// A plugin's language-neutral core declares what the plugin is; each
// binding file declares what it emits for one language. This package
// is the TypeScript half of that second half:
//
//	sdk.NewPlugin(Name).Version(Version).For(tsSupport()).Build()
//
// where tsSupport lives in the plugin's `_ts.go` file beside its
// embedded template tree.
//
// Returning a (language, support) pair rather than a bare struct is
// what lets the core spread it straight into the builder and never
// name a language itself.
package sdk
