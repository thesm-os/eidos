// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin

import (
	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/store"
)

// Frontend is the role for plugins that parse input into the source
// side of the [store.Store]. The pipeline runs every Frontend in
// the frontend phase; multiple frontends may coexist (e.g. a Go
// frontend alongside an OpenAPI-schema frontend), and their loaded
// packages share the same store.
//
// Load is called once per pipeline run per user-supplied pattern.
// Errors that prevent the frontend from making any progress are
// returned directly; per-input issues are emitted as positioned
// diagnostics on ctx.Diag and execution continues with the next
// pattern.
type Frontend interface {
	Plugin

	// Load parses the input identified by ctx.Pattern and records
	// the resulting nodes in ctx.Store.Nodes(). Per-input issues
	// attach to ctx.Diag; fatal failures return a non-nil error.
	Load(ctx *FrontendContext) error
}

// FrontendContext is the per-call context handed to every
// [Frontend.Load] invocation. The pipeline constructs one per
// (frontend, pattern) pair so a frontend handling multiple patterns
// in a single run sees a fresh context per call.
//
// FrontendContext mirrors the shape of [AnnotatorContext] /
// [GeneratorContext] / [BackendContext] in carrying Store and Diag;
// it adds the directive registry and the cache so frontends can
// validate parsed directives against the pipeline's schemas and
// participate in skip-on-hit caching.
type FrontendContext struct {
	// Store is the shared in-memory database. Frontends write
	// parsed declarations into Store.Nodes() via AddPackage.
	Store *store.Store

	// Diag is the diagnostic sink shared with every plugin in the
	// run. Per-input issues attach here as positioned diagnostics.
	Diag *diag.Sink

	// Registry is the directive-schema registry built from every
	// [pipeline.Builder.WithDirective] schema declared on the
	// pipeline. Frontends look up registered schemas when validating
	// parsed `+gen:` / `-gen:` directives; unregistered directives
	// parse cleanly so in-development plugins remain forward-
	// compatible.
	Registry *directive.Registry

	// Parser is the project-wide directive parser shared by every
	// frontend in the run — typically [directive.DefaultParser].
	// Frontends call [directive.Parser.ParseComment] on each
	// comment they encounter; the parser handles marker stripping,
	// prefix detection, and argument tokenisation uniformly so
	// every plugin sees the same canonical directive shape.
	Parser *directive.Parser

	// Cache is the configured [cache.Cache] the pipeline runs with.
	// Frontends compose a content-addressed key from their inputs +
	// [Versioned.Version] (when implemented) and consult the cache
	// to skip parsing on hit. May be the no-op [cache.None] when
	// caching is disabled for the run; never nil.
	Cache cache.Cache

	// Pattern is the user-supplied input identifier for this Load
	// call. Typically a Go-style import path ("./...",
	// "github.com/foo/bar/...") or a filesystem glob. Frontends
	// interpret it in a language-appropriate way.
	Pattern string

	// Fingerprint identifies the pipeline composition this Load runs
	// within: a hash over every registered plugin's name paired with
	// its [Versioned] string, order-independent.
	//
	// Frontends that cache a parsed node graph MUST fold this into
	// their cache key. A frontend's output does not depend on the
	// plugin set in principle — but the graph it caches carries the
	// metadata downstream plugins read, and an upgraded plugin
	// expecting metadata an older frontend never stamped is exactly
	// the stale-cache failure this closes. Re-parsing when the
	// plugin set changes is the deliberate cost of never handing a
	// plugin a graph it cannot interpret.
	//
	// A plugin implementing no [Versioned] contributes its name with
	// an empty version, so its upgrades do not invalidate anything.
	// Declaring a version is how a plugin opts into being able to
	// invalidate the cache its output depends on.
	Fingerprint string
}
