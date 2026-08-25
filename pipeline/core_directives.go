// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"slices"

	"go.thesmos.sh/eidos/core/directive"
)

// OutDirective is the name of the canonical `out` directive — the
// per-source routing override the Layout phase consumes. Source
// authors annotate an entity with `+gen:out <path>` (with optional
// `plugin=<name>` scope and `pkg=<name>` override) to influence
// where the framework places whatever the plugins emit from that
// entity. The directive name is brand-independent; the parser
// strips the prefix before dispatch.
const OutDirective directive.Name = "out"

// ValueDirective is the name of the canonical `value` directive
// — a per-source override on any node that pins the rendered or
// serialised string form of the host. Generic by design: any
// plugin that translates node identity into a string
// representation (struct-field serialisation tags, sentinel
// error codes, configuration-key naming, identifier renames,
// route patterns, …) reads `+gen:value <override>` to honour the
// user's chosen value. The framework reserves the directive
// centrally so plugins share the schema rather than each
// reinventing it under per-plugin names.
const ValueDirective directive.Name = "value"

// coreDirectives returns the directive schemas the pipeline registers
// unconditionally, before any builder-supplied or plugin-supplied
// schemas. These are framework-level directives the Layout phase
// and other internal subsystems depend on; reserving them centrally
// prevents accidental collisions with plugin-defined directives.
//
// # The routing surface
//
// Output placement has three equivalent user-facing forms — pick the
// one that fits the source. All three feed the same precedence
// pipeline in the Layout phase; only the syntactic anchor differs.
//
// 1. **Default** — no directive. Files land alongside source in the
// origin's source package. The `_test.go → <pkg>_test` shift fires
// automatically for any plugin whose filename suffix produces a
// test file.
//
//	type Store interface { ... }
//	// → store/store_mock.go (package store)
//	// → store/store_mock_test.go (package store_test)
//
// 2. **Standalone `+gen:out`** — anchored on the source node, not
// on any particular emitter. Supports positional path (`testkit/`,
// `mocks/handler.go`), optional `plugin=<name>` scope to narrow the
// override to one plugin, and `pkg=<name>` to pin the rendered
// `package` clause:
//
//	//+gen:out testkit/
//	//+gen:mock
//	type Store interface { ... }
//	// → store/testkit/store_mock.go (package testkit)
//	// → store/testkit/store_mock_test.go (package testkit_test)
//
// 3. **Per-directive keys** — `out=` and `pkg=` keys on any plugin's
// own directive. The pipeline auto-recognises them on every
// directive owned by a plugin (tracked at Build time from
// [plugin.DirectiveProvider.Directives]). The override is
// companion-aware: applies to every plugin emitting against the
// same origin, so sibling generators discovering output via meta
// inherit the routing without restating it.
//
//	//+gen:mock out=testkit/ pkg=storetest
//	type Store interface { ... }
//	// → store/testkit/store_mock.go (package storetest)
//	// → store/testkit/store_mock_test.go (package storetest_test)
//
// # Precedence
//
// Layout composes [emit.Target] through layers, low → high:
// framework default → plugin's [plugin.FilenameProvider] suffix →
// project layout policy (config) → per-source directive
// (forms 2 + 3) → CLI `-o` / `-p`. The `_test.go → <pkg>_test`
// shift runs at the framework-default layer and is skipped when
// the package was set at any higher layer (or when it already
// ends in `_test`, so plugins that opted into the convention
// themselves don't double-shift).
//
// # Strict per-plugin scope
//
// Per-directive keys (form 3) propagate to every plugin on the
// origin by design. When a user needs strict per-plugin scope —
// `+gen:mock out=mocks/` should affect ONLY the mock plugin, not
// its companion mocktest — they reach for the standalone form
// with the `plugin=` selector:
//
//	//+gen:out mocks/ plugin=mock
//	//+gen:mock
//	type Store interface { ... }
//
// This is the rare case; the per-directive form covers the common
// "this directive's outputs travel together" intent without typing.

// RoutingKeys are the KV keys the Layout phase honours on any
// plugin-owned directive: `out=<path>` redirects the emitted file,
// `pkg=<name>` pins its package clause, and `tag=<name>` scopes both
// to one of the owning plugin's declared outputs.
//
// They are reserved rather than plugin-declared because routing is a
// framework concern that applies uniformly. A plugin author writing
// `AllowedKeys("defaults")` is describing their own option surface
// and has no reason to suspect they are also switching off
// redirection — which is exactly what a literal reading of
// AllowedKeys would do. [widenRoutingKeys] closes that gap at
// registration.
var RoutingKeys = []string{"out", "pkg", "tag"}

// widenRoutingKeys returns s with [RoutingKeys] added to its
// AllowedKeys, so a plugin that restricts its own key surface does
// not accidentally reject the framework's routing overrides.
//
// A schema with an empty AllowedKeys already accepts every key and
// is returned untouched — appending would convert "accepts
// anything" into "accepts exactly the routing keys", which is the
// opposite of the intent.
//
// Applied only to plugin-owned directives. The standalone `+gen:out`
// directive spells its own scope with `plugin=` and takes the path
// positionally, so it neither needs nor accepts these.
func widenRoutingKeys(s directive.Schema) directive.Schema {
	if len(s.AllowedKeys) == 0 {
		return s
	}
	for _, k := range RoutingKeys {
		if !slices.Contains(s.AllowedKeys, k) {
			s.AllowedKeys = append(s.AllowedKeys, k)
		}
	}
	return s
}

func coreDirectives() []directive.Schema {
	return []directive.Schema{
		directive.NewSchema(OutDirective).
			Describe(
				"Routing override for emit entities anchored to this source node. "+
					"Positional path (filename or relative dir + filename) is required; "+
					"optional `plugin=<name>` scopes the override to one plugin's output, "+
					"optional `tag=<name>` scopes it to one of that plugin's outputs, "+
					"and optional `pkg=<name>` pins the rendered package clause. CLI -o "+
					"and -p override this directive in turn. Per-directive `out=` / `pkg=` "+
					"keys on any plugin's own directive serve the same role with the "+
					"emitter as natural anchor.",
			).
			Positional("filename").
			AllowedKeys("plugin", "tag", "pkg").
			DenyNegation().
			Build(),
		directive.NewSchema(MetaDirectiveName).
			Describe(
				"Sets or clears metadata on the host node. The positive form " +
					"takes `KEY=VALUE` pairs, or a bare positional KEY to set it " +
					"true; the negated form tombstones each named key, and a name " +
					"no meta key matches is read as a prefix so a group can be " +
					"cleared without enumerating its leaves. Applied by the " +
					"pipeline between the annotator and generator phases.",
			).
			// Negation is not denied: the tombstoning form is half of what
			// this directive is for.
			Positional("key").
			AllowExtraPositional().
			Build(),
		directive.NewSchema(ValueDirective).
			Describe(
				"Per-source override for the rendered or serialised string " +
					"form of the host node. Generic by design — consumed by any " +
					"plugin that translates node identity into a string " +
					"representation (serialisation tags, error codes, " +
					"configuration keys, identifier renames, route patterns, …). " +
					"Reserved centrally so plugins share the schema rather than " +
					"re-registering per-plugin variants.",
			).
			Positional("override").
			DenyKeys().
			DenyNegation().
			Build(),
	}
}
