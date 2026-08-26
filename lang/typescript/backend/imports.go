// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"cmp"
	"slices"
	"strings"
	"sync"

	"go.thesmos.sh/eidos/lang/typescript"
)

// importSet accumulates the module imports one rendered file needs.
//
// Not [writer.ImportSet]. That models Go's `import alias "path"`: one
// alias per path, which is the whole of what a Go import binds.
// TypeScript binds a *set* of names per specifier — `import { A, B }
// from './y'` — alongside an optional default and an optional
// namespace, and any of the three may be type-only. One alias per
// path cannot express that, and forcing it to would render every
// import as `import * as x from './y'` and every use site as `x.A`,
// which is valid and is not what anyone writes.
//
// Safe for concurrent use so a per-target render can dispatch through
// one set without coordination, matching what the framework's own
// import set promises.
type importSet struct {
	mu sync.Mutex

	// order lists specifiers in first-seen order, so the arrangement
	// is decided by the sort in [importSet.Imports] rather than by
	// map iteration.
	order []string

	// byPath holds one entry per module specifier.
	byPath map[string]*moduleImport

	// self is the specifier of the module being rendered. A binding
	// asked for from the file's own module renders bare, since a
	// module importing itself is a cycle the runtime rejects.
	self string
}

// moduleImport is everything one file imports from one specifier.
type moduleImport struct {
	// named are the bindings imported by name, deduplicated.
	named map[string]struct{}

	// def is the default binding's local name, empty when the file
	// imports no default from this module.
	def string

	// namespace is the local name of a `* as ns` import.
	namespace string

	// typeOnly reports that every binding from this specifier is a
	// type. A module contributing both a type and a value cannot be
	// type-only: erasing it would erase the value too.
	typeOnly bool
}

// newImportSet returns an empty set for the module at self. Pass an
// empty self to disable same-module elision.
func newImportSet(self string) *importSet {
	return &importSet{byPath: map[string]*moduleImport{}, self: self}
}

// entry returns the record for path, creating it on first use.
// Callers hold the lock.
func (s *importSet) entry(path string) *moduleImport {
	m, ok := s.byPath[path]
	if !ok {
		m = &moduleImport{named: map[string]struct{}{}, typeOnly: true}
		s.byPath[path] = m
		s.order = append(s.order, path)
	}
	return m
}

// Named records a named binding and returns the local name to use at
// the reference site.
//
// A binding from the file's own module returns bare and records
// nothing: a module cannot import itself, and emitting the import
// would make the file fail to load rather than merely look odd.
func (s *importSet) Named(path, name string, typeOnly bool) string {
	if name == "" {
		return ""
	}
	if path == "" || path == s.self {
		return name
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	m := s.entry(path)
	m.named[name] = struct{}{}
	m.typeOnly = m.typeOnly && typeOnly
	return name
}

// Default records a default import under the supplied local name.
//
// A default export carries no name of its own — the importing file
// chooses one — so the caller supplies it. A second call for the same
// specifier keeps the first name, because two local names for one
// default binding would be two imports of the same thing.
func (s *importSet) Default(path, local string) string {
	if path == "" || local == "" || path == s.self {
		return local
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	m := s.entry(path)
	if m.def == "" {
		m.def = local
	}
	m.typeOnly = false
	return m.def
}

// Namespace records a `* as ns` import.
func (s *importSet) Namespace(path, local string) string {
	if path == "" || local == "" || path == s.self {
		return local
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	m := s.entry(path)
	if m.namespace == "" {
		m.namespace = local
	}
	m.typeOnly = false
	return m.namespace
}

// Len reports how many specifiers the file imports from.
func (s *importSet) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.byPath)
}

// Imports renders the import block, one statement per specifier, in
// the order a TypeScript project conventionally writes them.
//
// Sorted rather than emitted in first-seen order: first-seen depends
// on which declaration the renderer reached first, so a reordering of
// the emit graph would reorder the imports without changing what the
// file means. Sorting makes the block a function of what is imported.
//
// Within the sort, packages precede relative specifiers and each
// group sorts by specifier. That is the grouping every TypeScript
// style guide converges on, and the one a reader scans for "what does
// this file depend on from outside".
func (s *importSet) Imports() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	paths := slices.Clone(s.order)
	slices.SortFunc(paths, func(a, b string) int {
		if c := cmp.Compare(specifierRank(a), specifierRank(b)); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})

	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if stmt := statement(p, s.byPath[p]); stmt != "" {
			out = append(out, stmt)
		}
	}
	return out
}

// statement renders one specifier's import.
func statement(path string, m *moduleImport) string {
	clauses := make([]string, 0, 3)
	if m.def != "" {
		clauses = append(clauses, m.def)
	}
	if m.namespace != "" {
		clauses = append(clauses, "* as "+m.namespace)
	}
	if len(m.named) > 0 {
		names := make([]string, 0, len(m.named))
		for n := range m.named {
			names = append(names, n)
		}
		slices.Sort(names)
		clauses = append(clauses, "{ "+strings.Join(names, ", ")+" }")
	}
	if len(clauses) == 0 {
		return ""
	}

	kind := "import "
	// Type-only erases at compile time. Marking a specifier that also
	// contributes a value would erase the value with it, which is why
	// typeOnly is an AND across every binding rather than a flag the
	// last caller sets.
	if m.typeOnly {
		kind = "import type "
	}
	return kind + strings.Join(clauses, ", ") + " from " + typescript.Quote(path) + ";"
}

// specifierRank orders the two import groups: a package specifier
// sorts before a relative one.
func specifierRank(path string) int {
	if strings.HasPrefix(path, ".") {
		return 1
	}
	return 0
}
