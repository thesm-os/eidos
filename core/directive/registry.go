// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package directive

import (
	"errors"
	"fmt"
	"slices"
	"sync"
)

// ErrSchemaConflict is returned by [Registry.Register] when a schema
// with the same [Name] is already registered. The pipeline surfaces
// this at Build() time as a positioned diagnostic so the user can see
// both contributors.
var ErrSchemaConflict = errors.New("directive: schema name already registered")

// Registry stores [Schema] values keyed by [Name].
//
// All methods are safe to call concurrently; the underlying RWMutex
// is uncontended in the common case (Register at init / Build time,
// Lookup at validation time).
//
// The zero value is unusable; construct with [NewRegistry].
type Registry struct {
	mu        sync.RWMutex
	schemas   map[Name]Schema
	unclaimed bool
}

// NewRegistry returns an empty registry ready for use.
//
// Strict about unclaimed names: a directive no schema registers is
// reported by [Validate]. See [Registry.AllowUnclaimed] for the run
// that has to permit them.
func NewRegistry() *Registry {
	return &Registry{schemas: map[Name]Schema{}}
}

// AllowUnclaimed stops [Validate] reporting a directive no schema
// registers.
//
// For a run that is deliberately narrower than the source it reads: a
// consumer registering three plugins over a tree annotated for eight
// has directives nothing in this run claims, and every one of them is
// a line the author meant. Reporting those would make a narrowed run
// unusable rather than merely partial.
//
// Off by default, because the ordinary case is the opposite one. A
// misspelt name parses, matches no schema, stamps nothing and
// generates nothing — output indistinguishable from a declaration
// that asked for nothing at all, which is the failure a directive
// vocabulary can least afford.
func (r *Registry) AllowUnclaimed() *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unclaimed = true
	return r
}

// UnclaimedAllowed reports whether [Registry.AllowUnclaimed] was
// called.
func (r *Registry) UnclaimedAllowed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.unclaimed
}

// Register adds s to r. Returns [ErrSchemaConflict] wrapped with the
// offending name when a schema with that name is already present.
func (r *Registry) Register(s Schema) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.schemas[s.Name]; exists {
		return fmt.Errorf("%w: %q", ErrSchemaConflict, s.Name)
	}
	r.schemas[s.Name] = s
	return nil
}

// Lookup returns the registered schema for name. The second return
// is false when no schema is registered under that name.
func (r *Registry) Lookup(name Name) (Schema, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.schemas[name]
	return s, ok
}

// Names returns every registered directive name in sorted order.
func (r *Registry) Names() []Name {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Name, 0, len(r.schemas))
	for n := range r.schemas {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// Suggest returns the registered name closest to query by edit
// distance, useful for "did you mean?" diagnostics. The second
// return is false when the registry is empty or the best candidate
// is too far from query (distance > len(query)/2 + 1) to be a
// plausible typo.
func (r *Registry) Suggest(query Name) (Name, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.schemas) == 0 {
		return "", false
	}
	bestName := Name("")
	bestDist := -1
	for n := range r.schemas {
		d := editDistance(string(query), string(n))
		if bestDist == -1 || d < bestDist {
			bestDist = d
			bestName = n
		}
	}
	// Reject suggestions that are too distant to plausibly be a typo.
	threshold := len(query)/2 + 1
	if bestDist > threshold {
		return "", false
	}
	return bestName, true
}
