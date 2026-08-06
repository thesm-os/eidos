// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"go.thesmos.sh/eidos/plugin"
)

// TestDefaultPlugins_DeclareAVersion pins that every plugin the
// reference binary ships declares a non-empty [plugin.Versioned].
//
// The version is what the pipeline hashes into its composition
// fingerprint, which frontends fold into their cache keys. A plugin
// that declares none contributes `name@""`, so changing its behaviour
// can never invalidate a warm cache — the caller gets stale output and
// no signal, which is what made `--no-cache` a workaround.
//
// The conformance suite's AssertVersionedStability cannot catch this.
// It checks that a version is *stable* if one is declared, and a
// plugin whose Version method sits on the wrong receiver simply fails
// the interface assertion and passes the check vacuously. This guard
// asserts the declaration itself, against the exact set the binary
// registers rather than a hand-maintained list that could drift from
// it.
func TestDefaultPlugins_DeclareAVersion(t *testing.T) {
	t.Parallel()

	plugins := defaultPlugins()
	if len(plugins) == 0 {
		t.Fatalf("defaultPlugins returned nothing — the guard would pass vacuously")
	}

	for _, p := range plugins {
		t.Run(p.Name(), func(t *testing.T) {
			t.Parallel()
			versioned, ok := any(p).(plugin.Versioned)
			if !ok {
				t.Fatalf("%T does not implement plugin.Versioned; it contributes an "+
					"empty version to the cache key and can never invalidate it", p)
			}
			if versioned.Version() == "" {
				t.Errorf("%T declares an empty version, which opts it out of cache "+
					"invalidation entirely", p)
			}
		})
	}
}
