// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin

import (
	"encoding/json"
	"fmt"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/node"
)

// Frontend caching, and why the framework owns it.
//
// Every frontend performs one dance: hash this unit's inputs, fold in
// the run's composition fingerprint, look the key up, deserialise the
// graph and re-wire its owner pointers, or else convert and write the
// result back. Only the first step is language-specific — which bytes
// are this unit's inputs — and three frontends wrote the other four
// each.
//
// The cost of that is not duplication. [FrontendContext.Fingerprint]
// carries the only capitalised MUST in the frontend contract, and a
// MUST every implementation satisfies by hand is one a conformance
// suite has to police rather than one the framework guarantees. Worse,
// a frontend-local cache is a frontend-local *skip*: the Go frontend
// validated directives inside its converter, which is precisely what a
// hit avoids, so a malformed directive was reported on the cold run
// and accepted silently on every run after it.
//
// Composing the key here makes the fingerprint structural — a frontend
// cannot leave it out, because it never sees the key.

// FrontendInputs reports the hash of one unit's inputs, or an error
// where none can be computed.
//
// The one language-specific half of caching: a Go frontend hashes the
// bytes of the files in a package, a protobuf frontend the descriptors
// it resolved, a TypeScript frontend the sources plus whatever its
// config contributes. An error means the unit cannot be keyed — not
// that it cannot be converted — so [CacheLoad] converts it and skips
// both the lookup and the write.
type FrontendInputs func() (string, error)

// FrontendConvert produces the packages one unit contains.
//
// Called only on a miss, which is the whole point: everything a
// frontend does that a hit should avoid belongs inside it, and
// anything outside it runs on both paths whether that was intended or
// not.
type FrontendConvert func() ([]*node.Package, error)

// CacheLoad adds one unit's packages to the store, converting only
// when the cache cannot answer.
//
// Composes the key from the frontend's name and version, the run's
// composition fingerprint and the unit's input hash, so a plugin set
// that changed invalidates every graph the previous set produced.
// Callers pass the unit's own identity — an import path, a directory —
// through name so two units in one run cannot collide.
//
// A cache that can retain nothing skips the hash and the marshal
// together: under a no-op cache the lookup could never hit and the
// write could never be read back, so both are pure cost.
//
// A write failure is not an error. The graph is already in the store
// and the run is correct; the only consequence is that the next run
// repeats the conversion, and failing here would turn a slow run into
// a broken one.
func CacheLoad(
	ctx *FrontendContext,
	frontend, version, unit string,
	inputs FrontendInputs,
	convert FrontendConvert,
) error {
	key, keyed := cacheKey(ctx, frontend, version, unit, inputs)
	if keyed {
		if pkgs, hit := cachedPackages(ctx.Cache, key); hit {
			return addPackages(ctx, pkgs)
		}
	}
	pkgs, err := convert()
	if err != nil {
		return err
	}
	if err := addPackages(ctx, pkgs); err != nil {
		return err
	}
	// Nothing converted is not nothing to remember, but it is nothing
	// this can key honestly: an empty result and a unit whose inputs
	// could not be read are indistinguishable on the way back in, and
	// serving the first as a hit would hide the second.
	if keyed && len(pkgs) > 0 {
		storePackages(ctx, frontend, key, pkgs)
	}
	return nil
}

// cacheKey composes the key for one unit, reporting false when the run
// cannot use one.
//
// The fingerprint is folded in here rather than by the caller. It is
// the contract's one MUST, and a caller that forgets it produces a key
// that looks right and serves a graph the current plugin set never
// produced — which is indistinguishable from a correct hit.
func cacheKey(
	ctx *FrontendContext, frontend, version, unit string, inputs FrontendInputs,
) (string, bool) {
	if ctx == nil || ctx.Cache == nil || !cacheRetains(ctx.Cache) || inputs == nil {
		return "", false
	}
	hash, err := inputs()
	if err != nil {
		return "", false
	}
	return cache.NewKey(
		"frontend", frontend,
		"version", version,
		"fingerprint", ctx.Fingerprint,
		"unit", unit,
		"input", hash,
	), true
}

// cacheRetains reports whether c can return a hit or retain a write.
//
// Both halves of the work are skipped when it cannot: under a no-op
// cache the input hash could never match and the marshalled graph
// could never be read back, so a run with caching switched off
// otherwise costs more than one with it on.
//
// A type switch rather than a probe write. Asking by behaviour means
// putting an entry in the cache to see whether it comes back, and that
// entry is real: it counts as a write, it survives into the next run,
// and it is keyed identically whatever the composition fingerprint —
// which is precisely the shape the frontend contract forbids.
//
// Conservative in the other direction: an implementation this package
// does not recognise is assumed to retain, which costs only the status
// quo.
func cacheRetains(c cache.Cache) bool {
	_, none := c.(*cache.None)
	return !none
}

// cachedPackages reads a unit's graph back, re-wiring the owner
// pointers encoding strips.
//
// A malformed entry is a miss rather than a failure. The cache is a
// memo of work that can always be redone, so a corrupt payload costs a
// conversion and nothing else — and reporting it would fail a run over
// something the next one repairs by overwriting.
func cachedPackages(c cache.Cache, key string) ([]*node.Package, bool) {
	body, ok := c.Get(key)
	if !ok {
		return nil, false
	}
	var pkgs []*node.Package
	//nolint:musttag // node graphs are JSON-safe by construction
	if json.Unmarshal(body, &pkgs) != nil || len(pkgs) == 0 {
		return nil, false
	}
	for _, pkg := range pkgs {
		node.RewireOwners(pkg)
	}
	return pkgs, true
}

// storePackages writes a unit's graph back for the next run.
func storePackages(ctx *FrontendContext, frontend, key string, pkgs []*node.Package) {
	if len(pkgs) == 0 {
		return
	}
	body, err := json.Marshal(pkgs) //nolint:musttag // node graphs are JSON-safe by construction
	if err != nil {
		ctx.Diag.For(frontend).Warnf(pkgs[0].Pos(),
			"caching this graph failed, so the next run converts it again: %v", err)
		return
	}
	if err := ctx.Cache.Put(key, body); err != nil {
		ctx.Diag.For(frontend).Warnf(pkgs[0].Pos(),
			"caching this graph failed, so the next run converts it again: %v", err)
	}
}

// addPackages writes every package of one unit into the store.
func addPackages(ctx *FrontendContext, pkgs []*node.Package) error {
	for _, pkg := range pkgs {
		if err := ctx.Store.Nodes().AddPackage(pkg); err != nil {
			return fmt.Errorf("add package %q: %w", pkg.Path, err)
		}
	}
	return nil
}
