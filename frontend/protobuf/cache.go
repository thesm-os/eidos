// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protobuf

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// composeCacheKey returns the per-plugin cache key for one
// protobuf frontend Load. The framework treats frontends as cache
// consumers per [plugin.FrontendContext]: the frontend composes
// its own content-addressed key and calls the cache directly. The
// key carries three contributors:
//
//  1. The frontend's plugin name and version (so a version bump
//     invalidates every cached payload the frontend produced under
//     the previous version).
//  2. The frontend's configured options' canonical hash (so an
//     option flip — including the `import_paths` value or the
//     `include_well_known` toggle — invalidates the per-run
//     cache).
//  3. The resolved descriptor-set hash (the content-addressed view
//     of every `.proto` source that contributed to this run,
//     including transitive imports — protocompile's resolver
//     pulls them in, so a referenced `.proto` modified in a
//     different directory changes the hash and invalidates the
//     entry).
//
// The string return is a stable, sort-independent key suitable for
// `cache.Cache.Get` / `cache.Cache.Put`.
func composeCacheKey(
	ps *diag.PluginSink, opts Options, descriptors []protoreflect.FileDescriptor,
) string {
	return cache.NewKey(
		"plugin", FrontendName,
		"version", FrontendVersion,
		"opts", hashOptions(opts),
		"descriptors", hashDescriptorSet(ps, descriptors),
	)
}

// hashOptions returns a SHA-256 hash over the canonicalized
// option values. Each contributing entry is length-prefixed so a
// value containing `=` or `\0` can't collide with a neighbouring
// key/value pair through accidental concatenation. Comma-separated
// fields normalize through [importPathList] so trivial syntactic
// variations (whitespace, trailing commas) collapse to the same
// hash.
func hashOptions(opts Options) string {
	pieces := []string{
		encodeOptionPair("dir", opts.Dir),
	}
	for _, p := range importPathList(opts.ImportPaths) {
		pieces = append(pieces, encodeOptionPair("import_path", p))
	}
	if opts.IncludeWellKnown {
		pieces = append(pieces, encodeOptionPair("include_well_known", "true"))
	} else {
		pieces = append(pieces, encodeOptionPair("include_well_known", "false"))
	}
	return cache.HashStrings(pieces)
}

// encodeOptionPair returns the length-prefixed canonical form
// `<len(name)>:<name>=<len(value)>:<value>`. Length-prefixing
// every component makes accidental collisions across pairs
// impossible regardless of which characters the values contain.
func encodeOptionPair(name, value string) string {
	return fmt.Sprintf("%d:%s=%d:%s", len(name), name, len(value), value)
}

// hashDescriptorSet returns a SHA-256 hash over every resolved
// descriptor in descriptors, normalized through proto's canonical
// wire encoding. Sorted by descriptor path so the result is
// invariant to protocompile's discovery order.
//
// Why descriptors (rather than raw `.proto` bytes): transitive
// import changes affect the resolved view but may not change the
// entry-file bytes. Hashing the resolved descriptor set propagates
// dependency-change invalidation correctly. Local-file edits hash
// through naturally because the descriptors regenerate on re-parse.
//
// Every contributing entry is name-prefixed so a marshal failure
// on file N collapses to a sentinel `<path>=<error>` payload
// rather than silently disappearing from the hash — two pipelines
// with differing inputs can't accidentally hash to the same key
// when one of the differing files happens to fail marshal.
// Marshal failures additionally surface as `diag.Warn` so the
// failure isn't invisible.
func hashDescriptorSet(ps *diag.PluginSink, descriptors []protoreflect.FileDescriptor) string {
	if len(descriptors) == 0 {
		// Empty input: still produces a stable, non-empty hash so
		// callers can pin the empty-cache-state shape if needed.
		return cache.HashBytes(nil)
	}
	type entry struct {
		name string
		fd   protoreflect.FileDescriptor
		body []byte
	}
	entries := make([]entry, 0, len(descriptors))
	for _, fd := range descriptors {
		dp, fallback := protoFileToDescriptorProto(fd)
		if fallback {
			ps.Warnf(
				position.Pos{File: fd.Path()},
				"protobuf: descriptor for %s is a reflect-only fallback; cache key uses the minimal form",
				fd.Path(),
			)
		}
		body, err := proto.MarshalOptions{Deterministic: true}.Marshal(dp)
		if err != nil {
			ps.Warnf(
				position.Pos{File: fd.Path()},
				"protobuf: marshal descriptor for %s: %v; cache key uses the error sentinel",
				fd.Path(), err,
			)
			body = []byte("marshal-error:" + err.Error())
		}
		entries = append(entries, entry{name: fd.Path(), fd: fd, body: body})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	h := sha256.New()
	for _, e := range entries {
		// Length-prefix the file path AND the descriptor body so
		// neither adjacent entries nor entries with similar
		// content can collide through concatenation.
		writeLenPrefixed(h, []byte(e.name))
		writeLenPrefixed(h, e.body)
	}
	sum := h.Sum(nil)
	return cache.HashBytes(sum)
}

// writeLenPrefixed writes a little-endian 8-byte length prefix
// followed by the body to h. Used to keep concatenated entries
// non-collidable in the descriptor-set hash.
func writeLenPrefixed(h interface{ Write([]byte) (int, error) }, body []byte) {
	var lenBuf [8]byte
	binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(body)))
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write(body)
}

// protoFileToDescriptorProto extracts the wire-format-stable
// [descriptorpb.FileDescriptorProto] from a
// [protoreflect.FileDescriptor]. protocompile's `linker.File`
// implements [protoreflect.FileDescriptor] and carries the
// underlying descriptor proto; the framework reads it through
// the standard protoreflect API.
//
// The bool return reports whether the result is a reflect-only
// fallback (true) or the descriptor's authoritative proto form
// (false). Callers surface the fallback case as a Warn so silent
// hash degradation is observable.
func protoFileToDescriptorProto(
	fd protoreflect.FileDescriptor,
) (*descriptorpb.FileDescriptorProto, bool) {
	if dp, ok := fd.(interface {
		FileDescriptorProto() *descriptorpb.FileDescriptorProto
	}); ok {
		return dp.FileDescriptorProto(), false
	}
	// Fallback: synthesize a minimal descriptor proto from the
	// reflect view. protocompile's linker.File always carries the
	// concrete accessor so this path doesn't fire under the
	// frontend's own compiles — it exists for defensive coverage
	// against alternative protoreflect.FileDescriptor providers.
	name := fd.Path()
	pkg := string(fd.Package())
	return &descriptorpb.FileDescriptorProto{
		Name:    &name,
		Package: &pkg,
	}, true
}

// storeCache writes body to c under key. Cache-write failures
// surface as Warn diagnostics through ps; the cache is
// observability for the frontend, not correctness, so a
// disk-full scenario doesn't fail the run.
func storeCache(c cache.Cache, ps *diag.PluginSink, key string, body []byte) {
	if c == nil {
		return
	}
	if err := c.Put(key, body); err != nil {
		ps.Warnf(position.Pos{}, "protobuf: cache put failed: %v", err)
	}
}

// loadPackagesFromCache attempts to read a previously-converted node
// graph for key from c. On hit it deserialises the payload and
// reconstructs each package's back-pointers via [node.RewireOwners];
// on miss, an empty payload, or any decode error it returns
// (nil, false) so the caller falls back to a fresh conversion.
//
// An empty body is treated as a miss rather than as "zero packages".
// Earlier revisions of this frontend stored a nil payload as a
// presence marker, so an empty entry means "a previous run recorded
// the key but never wrote a graph" — honouring it as a hit would
// silently convert nothing.
//
// Cache errors are swallowed deliberately: a corrupt entry is
// equivalent to a miss, and stale cache state must never block a
// run.
func loadPackagesFromCache(c cache.Cache, key string) ([]*node.Package, bool) {
	if c == nil {
		return nil, false
	}
	body, ok := c.Get(key)
	if !ok || len(body) == 0 {
		return nil, false
	}
	var pkgs []*node.Package
	if err := json.Unmarshal(body, &pkgs); err != nil { //nolint:musttag // node types carry JSON tags transitively
		return nil, false
	}
	for _, pkg := range pkgs {
		node.RewireOwners(pkg)
	}
	return pkgs, true
}

// storePackagesInCache serialises pkgs and writes them to c under
// key. Write failures surface as Warn diagnostics through
// [storeCache] — the cache is an optimisation, not correctness, so a
// full disk must not fail the run.
//
// Marshalling a node graph cannot fail in practice: every reachable
// field is JSON-encodable and back-pointer cycles are broken by
// `json:"-"` on Owner fields. The error is dropped as an unreachable
// contract violation.
func storePackagesInCache(c cache.Cache, ps *diag.PluginSink, key string, pkgs []*node.Package) {
	if c == nil || len(pkgs) == 0 {
		return
	}
	body, _ := json.Marshal(pkgs) //nolint:errcheck,musttag // node graphs are JSON-safe by construction
	storeCache(c, ps, key, body)
}
