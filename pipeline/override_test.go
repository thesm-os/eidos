// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

// overrideTestKey is the meta.Key used by the directive-override
// tests. Declared once so the global key registry never sees
// re-registration on -count > 1.
var overrideTestKey = meta.NewKey("pipeline.override.test", meta.BoolParser)

func TestPipeline_RunDirectiveOverride(t *testing.T) {
	t.Parallel()

	t.Run("runs between annotator and generator (visible in verbose phase logs)", func(t *testing.T) {
		t.Parallel()
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			WithVerbose(true).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context()))
		want := []string{"frontend", "annotator", "directive-override", "generator", "backend"}
		got := phaseLogs(d)
		for i, w := range want {
			if i >= len(got) || !strings.Contains(got[i], w) {
				t.Fatalf("phase log mismatch at %d: got %v, want %v", i, got, want)
			}
		}
	})

	t.Run("+meta KEY=VALUE on a node sets the key at AuthorityDirective", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				captured = &node.Struct{
					Name: "User", Package: "x",
					BaseNode: node.BaseNode{
						DirectiveList: []*directive.Directive{{
							Name: pipeline.MetaDirectiveName,
							KV:   map[string]string{overrideTestKey.Name(): "true"},
							Pos:  position.At("user.go", 1, 1),
						}},
					},
				}
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					Structs: []*node.Struct{captured},
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		got, ok := overrideTestKey.Get(captured.Meta())
		if !ok || !got {
			t.Fatalf("override should set the key to true; got %v ok=%v", got, ok)
		}
	})

	t.Run("-meta KEY on a node tombstones the key at AuthorityDirective", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				captured = &node.Struct{
					Name: "User", Package: "x",
					BaseNode: node.BaseNode{
						DirectiveList: []*directive.Directive{{
							Name:    pipeline.MetaDirectiveName,
							Negated: true,
							Args:    []string{overrideTestKey.Name()},
							Pos:     position.At("user.go", 1, 1),
						}},
					},
				}
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					Structs: []*node.Struct{captured},
				})
			},
		}
		// The tombstone makes the key resolve as "not set" via the
		// authority resolution rules — no prior Set is needed for the
		// tombstone itself to take effect on resolution.
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if overrideTestKey.Has(captured.Meta()) {
			t.Fatalf("tombstone should make the key resolve as not-set")
		}
	})

	t.Run("positional +meta KEY arg sets the key to true", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				captured = &node.Struct{
					Name: "User", Package: "x",
					BaseNode: node.BaseNode{
						DirectiveList: []*directive.Directive{{
							Name: pipeline.MetaDirectiveName,
							Args: []string{overrideTestKey.Name()},
							Pos:  position.At("user.go", 1, 1),
						}},
					},
				}
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					Structs: []*node.Struct{captured},
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		got, ok := overrideTestKey.Get(captured.Meta())
		if !ok || !got {
			t.Fatalf("positional arg should set key to true; got %v ok=%v", got, ok)
		}
	})

	t.Run("-meta KEY=VALUE tombstones the key (value ignored on negation)", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				captured = &node.Struct{
					Name: "User", Package: "x",
					BaseNode: node.BaseNode{
						DirectiveList: []*directive.Directive{{
							Name:    pipeline.MetaDirectiveName,
							Negated: true,
							KV:      map[string]string{overrideTestKey.Name(): "true"},
							Pos:     position.At("user.go", 1, 1),
						}},
					},
				}
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					Structs: []*node.Struct{captured},
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if overrideTestKey.Has(captured.Meta()) {
			t.Fatalf("negated KV directive should tombstone the key")
		}
	})

	t.Run("-meta with an unregistered prefix tombstones via prefix-walk semantics", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				captured = &node.Struct{
					Name: "User", Package: "x",
					BaseNode: node.BaseNode{
						DirectiveList: []*directive.Directive{{
							Name:    pipeline.MetaDirectiveName,
							Negated: true,
							Args:    []string{"never.registered.prefix"},
							Pos:     position.At("user.go", 1, 1),
						}},
					},
				}
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					Structs: []*node.Struct{captured},
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		// The tombstone is recorded but there's nothing to suppress
		// — the test verifies the path doesn't panic and the
		// pipeline still completes cleanly.
	})

	t.Run("a parser error in +meta KEY=VALUE emits a Warn diagnostic", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				captured = &node.Struct{
					Name: "User", Package: "x",
					BaseNode: node.BaseNode{
						DirectiveList: []*directive.Directive{{
							Name: pipeline.MetaDirectiveName,
							// overrideTestKey is a bool-parser key;
							// supplying a non-bool value forces a
							// parser error inside SetDirectiveFromString.
							KV:  map[string]string{overrideTestKey.Name(): "not-a-bool"},
							Pos: position.At("user.go", 1, 1),
						}},
					},
				}
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					Structs: []*node.Struct{captured},
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if p.Diag().Count(diag.Warn) == 0 {
			t.Fatalf("parser error should produce a Warn diagnostic")
		}
	})

	t.Run("non-meta directives are ignored by the override step", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				captured = &node.Struct{
					Name: "User", Package: "x",
					BaseNode: node.BaseNode{
						DirectiveList: []*directive.Directive{{
							// A registered directive that is not the override
							// name. An unregistered one would be refused before
							// the override step ever saw it, so the subtest
							// would pass for the wrong reason.
							Name: pipeline.OutDirective,
							Args: []string{"user_gen.go"},
							Pos:  position.At("user.go", 1, 1),
						}},
					},
				}
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					Structs: []*node.Struct{captured},
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		// The directive isn't a meta-override; the override step
		// should skip it without producing diagnostics.
		if p.Diag().Count(diag.Warn) != 0 {
			t.Fatalf("non-meta directives should not produce diagnostics from the override step")
		}
	})

	t.Run("positional arg with unknown key name emits a Warn diagnostic", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				captured = &node.Struct{
					Name: "User", Package: "x",
					BaseNode: node.BaseNode{
						DirectiveList: []*directive.Directive{{
							Name: pipeline.MetaDirectiveName,
							Args: []string{"never.registered.positional"},
							Pos:  position.At("user.go", 1, 1),
						}},
					},
				}
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					Structs: []*node.Struct{captured},
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if p.Diag().Count(diag.Warn) == 0 {
			t.Fatalf("unknown positional key should produce a Warn diagnostic")
		}
	})

	t.Run("override iterates every recorded node kind", func(t *testing.T) {
		t.Parallel()
		// Load a package populated with one entity of every kind that
		// rangeNodeView iterates so every per-kind closure fires.
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					Files:   []*node.File{{Name: "x.go", Path: "x/x.go"}},
					Imports: []*node.Import{{Path: "context"}},
					Structs: []*node.Struct{{
						Name: "U", Package: "x",
						Fields:  []*node.Field{{Name: "F"}},
						Methods: []*node.Method{{Name: "M"}},
					}},
					Interfaces: []*node.Interface{{
						Name: "I", Package: "x",
						Methods: []*node.Method{{Name: "Get"}},
					}},
					Functions: []*node.Function{{Name: "Fn", Package: "x"}},
					Variables: []*node.Variable{{Name: "V", Package: "x"}},
					Constants: []*node.Constant{{Name: "C", Package: "x"}},
					Enums: []*node.Enum{{
						Name: "E", Package: "x",
						Variants: []*node.EnumVariant{{Name: "A"}},
					}},
					Aliases: []*node.Alias{{Name: "A", Package: "x"}},
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
	})

	t.Run("unknown meta key in +meta directive emits a Warn diagnostic", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		fe := &recFE{
			name: "fe",
			loadFn: func(s *store.Store) {
				captured = &node.Struct{
					Name: "User", Package: "x",
					BaseNode: node.BaseNode{
						DirectiveList: []*directive.Directive{{
							Name: pipeline.MetaDirectiveName,
							KV:   map[string]string{"never.registered.key": "true"},
							Pos:  position.At("user.go", 1, 1),
						}},
					},
				}
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					Structs: []*node.Struct{captured},
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(fe).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if p.Diag().Count(diag.Warn) == 0 {
			t.Fatalf("unknown meta key should produce a Warn diagnostic")
		}
	})
}
