// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package writer

import (
	"fmt"
	"go/token"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// AliasFunc derives the default local alias for an import path
// when the caller has not registered an explicit one. The default
// implementation [DefaultAlias] returns the last "/"-delimited
// segment of the path (Go convention); other languages may
// substitute their own derivation.
type AliasFunc func(path string) string

// DefaultAlias returns the last "/"-delimited segment of path —
// "github.com/foo/bar" → "bar", "context" → "context". This is the
// Go-conventional default; pass it (or a custom function) to
// [NewImportSet] to control aliasing.
func DefaultAlias(path string) string {
	// Trailing separators are trimmed before the scan because the
	// last segment of "example.com/foo/" is the empty string, and an
	// empty alias is [ImportSet.Imp]'s sentinel for "this is the
	// file's own package, emit the bare name". Deriving it for a
	// genuinely foreign package renders that package's symbols
	// unqualified — a miscompile, or a silent bind to whatever else
	// is in scope. Go source never yields such a path, but
	// [emit.External] takes one from plugin code, which is precisely
	// where a joined or configured path picks up a trailing slash.
	return sanitiseAlias(rawSegment(path))
}

// sanitiseAlias turns a path segment into a valid, non-reserved Go
// identifier, returning seg unchanged when it already is one.
//
// A path's last segment is not required to be an identifier and
// frequently is not: `if-absent`, `go-redis`, `go-cmp`, `yaml.v3`.
// Emitting such a segment as a qualifier produces source the parser
// rejects, and the caller then writes it anyway. Two segments are
// worse than merely invalid — `if` and `go` are keywords, and a
// segment that lands on `go` yields `import "go"`, which resolves
// against the standard library and fails with "package go is not in
// std".
//
// The result is used as an *explicit* alias, which is what makes
// this correct rather than a guess: `import yamlv3 "gopkg.in/yaml.v3"`
// binds regardless of the package declaring `package yaml`. Deriving
// a name from a path can never know the real package name; an
// explicit alias does not need to.
//
// Non-identifier runes are dropped rather than replaced so the
// result stays readable, a leading digit gains a `pkg` prefix, and a
// Go keyword gains a trailing underscore.
//
// A segment yielding nothing at all returns the empty string rather
// than a synthesised name. That is load-bearing: the empty alias is
// [ImportSet.Imp]'s sentinel for "no alias is derivable", which it
// turns into [ErrEmptyPath]. Substituting a placeholder would accept
// a path that should be rejected and bind a package under a name
// nothing declares.
func sanitiseAlias(seg string) string {
	var b strings.Builder
	for _, r := range seg {
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return ""
	}
	if r, _ := utf8.DecodeRuneInString(out); unicode.IsDigit(r) {
		return "pkg" + out
	}
	if token.IsKeyword(out) {
		return out + "_"
	}
	return out
}

// NeedsExplicitAlias reports whether alias must be written into the
// import statement rather than left implicit.
//
// Go binds an unaliased import to the package's *declared* name,
// which the writer cannot know — it only sees the path. Leaving the
// alias implicit is therefore only safe when it is the path's last
// segment verbatim, the case where the overwhelming convention makes
// the guess right. Anything else — a sanitised segment
// (`if-absent` → `ifabsent`), a collision suffix (`context2`), or a
// caller-supplied override — must be stated, and stating it makes it
// true regardless of what the package calls itself.
func NeedsExplicitAlias(path, alias string) bool {
	return alias != rawSegment(path)
}

// rawSegment returns the path's last "/"-delimited segment with no
// sanitisation, which is what an unaliased import actually binds to
// by convention.
func rawSegment(path string) string {
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}

// Import is one entry in the [ImportSet]: the canonical package
// path and the local alias the writer assigned. Alias is always
// non-empty after [ImportSet.Imp] has been called.
type Import struct {
	Path  string
	Alias string
}

// ImportSet manages the per-file import block. Each path the
// backend asks about via [ImportSet.Imp] is recorded once;
// subsequent calls for the same path return the same alias.
// Collisions on the derived alias are resolved deterministically
// with a numeric suffix (e.g. two paths both derived to "context"
// produce aliases "context" and "context2").
//
// ImportSet is safe for concurrent use; the backend's parallel
// per-file rendering can dispatch through one ImportSet per file
// without coordination.
//
// The zero value is unusable; construct via [NewImportSet].
type ImportSet struct {
	mu       sync.Mutex
	derive   AliasFunc
	order    []string          // paths in insertion order
	aliases  map[string]string // path -> assigned alias
	explicit map[string]string // path -> caller-supplied override
	used     map[string]string // alias -> path it resolved to
	lastSfx  map[string]int    // derived base -> highest suffix handed out
	self     string            // import path of the rendered file's own package
}

// NewImportSet returns an empty ImportSet. Pass nil for derive to
// use [DefaultAlias] (the Go-conventional last-segment derivation).
func NewImportSet(derive AliasFunc) *ImportSet {
	if derive == nil {
		derive = DefaultAlias
	}
	return &ImportSet{
		derive:   derive,
		aliases:  map[string]string{},
		explicit: map[string]string{},
		used:     map[string]string{},
		lastSfx:  map[string]int{},
	}
}

// SetSelf records the import path of the package the rendered file
// itself declares, plus its short name. Two effects:
//
//   - Subsequent [ImportSet.Imp] calls with the same path return
//     the empty alias without recording an import — the same-package
//     elision rule callers (renderType, renderExpr) treat as "emit
//     the bare symbol name, no qualifier".
//   - The short name is reserved upfront in the alias collision
//     table so a cross-package import whose derived alias would
//     match (e.g. `example.com/bar/blog` rendered into a file
//     declaring `package blog`) falls back to a numeric-suffixed
//     alias rather than shadowing the file's own package
//     identifier.
//
// Empty path disables elision; empty name disables the reservation.
// Either can be called with the empty string for the unaffected
// half.
func (i *ImportSet) SetSelf(path, name string) {
	if path == "" && name == "" {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if path != "" {
		i.self = path
	}
	if name != "" {
		// Reserve the short name in `used` keyed against the self
		// path so [ImportSet.Imp] sees it as "taken by self" and
		// picks a suffixed alias for any other path that would
		// derive to it. Storing the self path (vs. a synthetic
		// sentinel) keeps the existing same-path-returns-existing
		// branch coherent — Imp for self path elides before
		// touching `used` at all.
		i.used[name] = i.self
	}
}

// Imp records path and returns the local alias to use in rendered
// output. Repeat calls for the same path return the same alias.
// Returns [ErrEmptyPath] when path is empty.
//
// Collision handling: when the derived alias is already taken by a
// different path, Imp appends a numeric suffix ("alias", "alias2",
// "alias3", …) until a free name is found. The suffix is
// deterministic for the same registration order.
func (i *ImportSet) Imp(path string) (string, error) {
	if path == "" {
		return "", ErrEmptyPath
	}
	i.mu.Lock()
	defer i.mu.Unlock()

	// Same-package elision: a path equal to the rendered file's
	// own import path resolves to the empty alias without
	// registering an import. Callers treat empty as "emit the
	// bare symbol name with no qualifier".
	if path == i.self {
		return "", nil
	}

	if existing, ok := i.aliases[path]; ok {
		return existing, nil
	}

	desired := i.explicit[path]
	if desired == "" {
		desired = i.derive(path)
	}
	// The empty alias is reserved for same-package elision, handled
	// above. Reaching it here means the path carries no derivable
	// identifier — "/" and "///" after trimming, or an injected
	// derive func returning "" — and handing it back would tell the
	// caller to emit the symbol unqualified against a package it
	// does not live in. The invariant belongs here rather than in
	// [DefaultAlias] so a caller-supplied derive cannot bypass it.
	if desired == "" {
		return "", fmt.Errorf("%w: %q has no derivable alias", ErrEmptyPath, path)
	}

	// Collision resolution resumes from the highest suffix already
	// handed out for this base rather than rescanning from 2.
	//
	// An alias is never released once taken, so the prefix below the
	// remembered mark is densely occupied and probing it can only
	// ever fail. Rescanning it made registration quadratic in the
	// number of paths sharing a last segment — 1000 such imports
	// cost 31ms and 1.27M allocations, almost all of them Sprintf
	// results discarded on the next iteration. A centralised-layout
	// run collecting many `<x>/models` packages into one file is
	// exactly that shape.
	//
	// The loop still verifies each candidate against `used`, so an
	// explicit Alias registration that claimed a suffixed name out
	// of band is still skipped rather than double-issued; the mark
	// is an optimisation of where to start, never a substitute for
	// the check.
	alias := desired
	n := i.lastSfx[desired]
	if n != 0 {
		alias = fmt.Sprintf("%s%d", desired, n)
	}
	for {
		owner, taken := i.used[alias]
		if !taken || owner == path {
			break
		}
		if n == 0 {
			n = 2
		} else {
			n++
		}
		alias = fmt.Sprintf("%s%d", desired, n)
	}
	if n != 0 {
		i.lastSfx[desired] = n
	}

	i.order = append(i.order, path)
	i.aliases[path] = alias
	i.used[alias] = path
	return alias, nil
}

// Alias registers an explicit local alias for path, overriding the
// derived default. The override must be registered before path is
// first imported via [ImportSet.Imp]; otherwise Alias returns
// [ErrAliasAfterImp].
//
// Empty path returns [ErrEmptyPath].
func (i *ImportSet) Alias(path, alias string) error {
	if path == "" {
		return ErrEmptyPath
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, alreadyImped := i.aliases[path]; alreadyImped {
		return fmt.Errorf("%w: %q", ErrAliasAfterImp, path)
	}
	i.explicit[path] = alias
	return nil
}

// AliasOf returns the assigned alias for path along with true; when
// the path has not been imported yet, returns "" and false.
func (i *ImportSet) AliasOf(path string) (string, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	a, ok := i.aliases[path]
	return a, ok
}

// Imports returns every recorded import in insertion order. The
// returned slice is safe for the caller to mutate; subsequent
// changes to the ImportSet are not reflected.
func (i *ImportSet) Imports() []Import {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make([]Import, len(i.order))
	for k, p := range i.order {
		out[k] = Import{Path: p, Alias: i.aliases[p]}
	}
	return out
}

// Len returns the number of recorded imports.
func (i *ImportSet) Len() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return len(i.order)
}
