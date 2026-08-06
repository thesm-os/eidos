// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline_test

import (
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
)

// TestLayout_AlongsideSource exercises the framework default: a
// decl with a non-nil Origin and a plugin-declared filename suffix
// resolves to alongside-source — Target.Dir derived from the
// source file's directory and Target.Filename composed from the
// source basename + suffix.
func TestLayout_AlongsideSource(t *testing.T) {
	t.Parallel()

	t.Run("Dir + Filename derived from origin Pos.File + plugin suffix", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go", Line: 10},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserRepo", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_repo.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Dir, "internal/users"; got != want {
			t.Fatalf("Target.Dir = %q, want %q", got, want)
		}
		if got, want := s.Target.Filename, "user_repo.go"; got != want {
			t.Fatalf("Target.Filename = %q, want %q", got, want)
		}
	})

	t.Run("Package + ImportPath derived from origin node.Package lookup", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserRepo", Package: "ignored",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_repo.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Package, "users"; got != want {
			t.Fatalf("Target.Package = %q, want %q", got, want)
		}
		if got, want := s.Target.ImportPath, "example.com/users"; got != want {
			t.Fatalf("Target.ImportPath = %q, want %q", got, want)
		}
	})
}

// TestLayout_AlongsideSource_HonoursEmitPackage pins the
// plugin-overrides-source-package contract: when a plugin emits a
// decl into an [emit.Package] whose Name / Path differ from the
// origin's source package, Layout's alongside-source path stamps
// Target.Package / Target.ImportPath from the emit.Package the
// plugin chose. The source package's directory still drives
// Target.Dir.
//
// mockgen is the canonical consumer: it emits mocks into a
// `<srcPkg>_test` emit.Package so the rendered file declares
// `package <pkg>_test` and its import identity diverges from the
// regular source package — same-package elision stays inert when
// the test mock references back into the regular package.
func TestLayout_AlongsideSource_HonoursEmitPackage(t *testing.T) {
	t.Parallel()

	t.Run("plugin emit.Package overrides source-package routing", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users_test",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "users_test", Path: "example.com/users_test",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Dir, "internal/users"; got != want {
			t.Fatalf("Target.Dir = %q, want %q (still derived from origin)", got, want)
		}
		if got, want := s.Target.Package, "users_test"; got != want {
			t.Fatalf("Target.Package = %q, want %q (from emit.Package.Name)", got, want)
		}
		if got, want := s.Target.ImportPath, "example.com/users_test"; got != want {
			t.Fatalf("Target.ImportPath = %q, want %q (from emit.Package.Path)", got, want)
		}
	})
}

// TestLayout_OutDirective verifies the +gen:out directive on an
// origin overrides the plugin-suffix-derived Filename — precedence
// layer 5.
func TestLayout_OutDirective(t *testing.T) {
	t.Parallel()

	t.Run("out directive on origin overrides Filename", func(t *testing.T) {
		t.Parallel()
		origin := &node.Interface{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{Name: pipeline.OutDirective, Args: []string{"user_mock_gen.go"}},
				},
			},
			Name: "UserRepo", Package: "example.com/users",
		}
		i := &emit.Interface{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserRepoMock", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Interfaces: []*emit.Interface{i},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := i.Target.Filename, "user_mock_gen.go"; got != want {
			t.Fatalf("Target.Filename = %q, want %q (out directive)", got, want)
		}
		if got, want := i.Target.Dir, "internal/users"; got != want {
			t.Fatalf("Target.Dir = %q, want %q (alongside-source)", got, want)
		}
	})
}

// TestLayout_OutDirective_PathAware pins the path-aware form of
// the +gen:out directive: a value carrying a directory component
// splits into Target.Dir (resolved relative to the origin's
// source directory) + Target.Filename, while a bare filename keeps
// the existing behaviour.
func TestLayout_OutDirective_PathAware(t *testing.T) {
	t.Parallel()

	t.Run("path with directory routes to sibling directory under source dir", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{Name: pipeline.OutDirective, Args: []string{"test/user_mock_test.go"}},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Dir, filepath.Join("internal", "users", "test"); got != want {
			t.Fatalf("Target.Dir = %q, want %q (origin dir + directive path)", got, want)
		}
		if got, want := s.Target.Filename, "user_mock_test.go"; got != want {
			t.Fatalf("Target.Filename = %q, want %q (basename of directive path)", got, want)
		}
	})
}

// TestLayout_OutDirectivePkg_ImportPathIsAPath pins that a `pkg=`
// override resolves Target.ImportPath to an import path rather than
// to the package clause name.
//
// The two are not interchangeable. ImportPath feeds the renderer's
// same-package elision, which compares it against each ExternalRef's
// package path; a clause name like "storetest" can never match one,
// so every self-reference rendered qualified against a package the
// decl no longer lived in.
func TestLayout_OutDirectivePkg_ImportPathIsAPath(t *testing.T) {
	t.Parallel()

	t.Run("pkg= with a directory joins the path onto the source package", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: pipeline.OutDirective,
						Args: []string{"testkit/"},
						KV:   map[string]string{"pkg": "userstest"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: &node.Package{Name: "users", Path: "example.com/users"}}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Package, "userstest"; got != want {
			t.Fatalf("Target.Package = %q, want %q", got, want)
		}
		if got, want := s.Target.ImportPath, "example.com/users/testkit"; got != want {
			t.Fatalf("Target.ImportPath = %q, want %q; the clause name is not an import path", got, want)
		}
	})

	t.Run("pkg= without a directory keeps the source package path", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: pipeline.OutDirective,
						Args: []string{"user_mock_gen.go"},
						KV:   map[string]string{"pkg": "userstest"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: &node.Package{Name: "users", Path: "example.com/users"}}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.ImportPath, "example.com/users"; got != want {
			t.Fatalf("Target.ImportPath = %q, want %q", got, want)
		}
	})
}

// TestLayout_OutDirective_PluginScope pins the plugin-scoped
// variant of +gen:out: when the directive carries `plugin=<name>`,
// the override applies only to the named plugin's output for that
// origin; every other plugin targeting the same source routes
// per the framework default.
func TestLayout_OutDirective_PluginScope(t *testing.T) {
	t.Parallel()

	t.Run("plugin=<name> filters the override to one plugin", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: pipeline.OutDirective,
						Args: []string{"custom.go"},
						KV:   map[string]string{"plugin": "mg"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		mocked := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		built := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserBuilder", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{mocked},
			}}).
			WithGenerator(&layoutGen{name: "bg", suffix: "_builder.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{built},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := mocked.Target.Filename, "custom.go"; got != want {
			t.Fatalf("mocked plugin's Target.Filename = %q, want %q (plugin-scoped override)", got, want)
		}
		if got, want := built.Target.Filename, "user_builder.go"; got != want {
			t.Fatalf("non-targeted plugin's Target.Filename = %q, want %q (framework default)", got, want)
		}
	})
}

// TestLayout_OutDirective_PluginScopeWithUnscopedFallback pins the
// canonical two-directive shape for resolving the one-file-one-package
// conflict that a plain `+gen:out filename.go` triggers when one
// plugin (mockgen's test-package mode) lands in a different package
// than the rest. An unscoped `+gen:out shared.go` applies to every
// plugin no scoped directive targets; a `plugin=<name>` scoped
// `+gen:out custom.go` overrides only the named plugin's output.
// The two land in distinct rendered files, side-stepping the
// invariant.
func TestLayout_OutDirective_PluginScopeWithUnscopedFallback(t *testing.T) {
	t.Parallel()

	t.Run("unscoped + plugin-scoped routes split into two filenames", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "x/x.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: pipeline.OutDirective,
						Args: []string{"shared.go"},
					},
					{
						Name: pipeline.OutDirective,
						Args: []string{"custom.go"},
						KV:   map[string]string{"plugin": "mg"},
					},
				},
			},
			Name: "X", Package: "example.com/x",
		}
		repogenEmit := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "XRepo", Package: "x",
		}
		buildergenEmit := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "XBuilder", Package: "x",
		}
		mockgenEmit := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "XMock", Package: "x",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_repo.go", pkg: &emit.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*emit.Struct{repogenEmit},
			}}).
			WithGenerator(&layoutGen{name: "bg", suffix: "_builder.go", pkg: &emit.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*emit.Struct{buildergenEmit},
			}}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*emit.Struct{mockgenEmit},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := repogenEmit.Target.Filename, "shared.go"; got != want {
			t.Errorf("repogen filename = %q, want %q (unscoped +gen:out fallback)", got, want)
		}
		if got, want := buildergenEmit.Target.Filename, "shared.go"; got != want {
			t.Errorf("buildergen filename = %q, want %q (unscoped +gen:out fallback)", got, want)
		}
		if got, want := mockgenEmit.Target.Filename, "custom.go"; got != want {
			t.Errorf(
				"mockgen filename = %q, want %q (plugin-scoped +gen:out wins over unscoped)",
				got,
				want,
			)
		}
	})
}

// TestLayout_OutDirective_PkgOverride pins the pkg=<name> arg on
// +gen:out: the supplied package overrides Target.Package and
// Target.ImportPath at directive layer (5), winning over the
// alongside-source emit.Package default but losing to CLI -p.
func TestLayout_OutDirective_PkgOverride(t *testing.T) {
	t.Parallel()

	t.Run("pkg=<name> pins Target.Package to the supplied value", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: pipeline.OutDirective,
						Args: []string{"user_mock.go"},
						KV:   map[string]string{"pkg": "users_mocks"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Package, "users_mocks"; got != want {
			t.Fatalf("Target.Package = %q, want %q (directive pkg= override)", got, want)
		}
	})
}

// TestLayout_OutputFilenameOverride verifies the CLI -o override
// (precedence layer 6) wins over the +gen:out directive (layer 5)
// for Target.Filename.
func TestLayout_OutputFilenameOverride(t *testing.T) {
	t.Parallel()

	t.Run("WithOutputFilename pins Target.Filename, overriding +gen:out", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{Name: pipeline.OutDirective, Args: []string{"directive.go"}},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserRepo", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_repo.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithOutputFilename("cli_pinned.go").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Filename, "cli_pinned.go"; got != want {
			t.Fatalf("Target.Filename = %q, want %q (CLI -o wins)", got, want)
		}
	})
}

// TestLayout_PluginOutputFilename pins the per-plugin and
// per-(plugin, tag) CLI overrides — the `-o <plugin>=<path>` and
// `-o <plugin>:<tag>=<path>` forms documented by the M2 spec.
// Specificity: a (plugin, tag) override wins over a plugin-only
// override; both win over the legacy global unscoped override.
func TestLayout_PluginOutputFilename(t *testing.T) {
	t.Parallel()

	t.Run("per-plugin override pins the matching plugin's outputs only", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		target := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "Target", Package: "users",
		}
		other := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "Other", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "target", suffix: "_t.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{target},
			}}).
			WithGenerator(&layoutGen{name: "other", suffix: "_o.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{other},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithPluginOutputFilename("target", "", "pinned.go").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := target.Target.Filename, "pinned.go"; got != want {
			t.Errorf("target.Filename = %q, want %q (per-plugin override applies)", got, want)
		}
		if got, want := other.Target.Filename, "user_o.go"; got != want {
			t.Errorf("other.Filename = %q, want %q (per-plugin override should not propagate)", got, want)
		}
	})

	t.Run("per-(plugin, tag) override pins only the matching output", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		primary := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "Primary", Package: "users",
		}
		tagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "Tagged", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{
				name: "enum",
				outputs: []plugin.Output{
					{Suffix: "_enum.go"},
					{Tag: "test", Suffix: "_enum_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{primary, tagged},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithPluginOutputFilename("enum", "test", "tests/pinned_test.go").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := primary.Target.Filename, "user_enum.go"; got != want {
			t.Errorf("primary.Filename = %q, want %q (untagged should keep default)", got, want)
		}
		if got, want := tagged.Target.Filename, "pinned_test.go"; got != want {
			t.Errorf("tagged.Filename = %q, want %q", got, want)
		}
		if got, want := tagged.Target.Dir, filepath.Join("internal", "users", "tests"); got != want {
			t.Errorf("tagged.Dir = %q, want %q (path-aware override)", got, want)
		}
	})

	t.Run("per-(plugin, tag) wins over per-plugin override", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		primary := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "Primary", Package: "users",
		}
		tagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "Tagged", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{
				name: "enum",
				outputs: []plugin.Output{
					{Suffix: "_enum.go"},
					{Tag: "test", Suffix: "_enum_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{primary, tagged},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithPluginOutputFilename("enum", "", "broad.go").
			WithPluginOutputFilename("enum", "test", "narrow_test.go").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := primary.Target.Filename, "broad.go"; got != want {
			t.Errorf("primary.Filename = %q, want %q (per-plugin override applies)", got, want)
		}
		if got, want := tagged.Target.Filename, "narrow_test.go"; got != want {
			t.Errorf("tagged.Filename = %q, want %q (per-tag wins over per-plugin)", got, want)
		}
	})
}

// TestLayout_OutputFilename_PathAware pins the path-aware form of
// CLI -o: a value carrying a directory component splits into
// Target.Dir (stacked under the origin's source directory) +
// Target.Filename, mirroring +gen:out's path-aware semantics. A
// bare filename keeps the existing behaviour and pins Filename
// only.
func TestLayout_OutputFilename_PathAware(t *testing.T) {
	t.Parallel()

	t.Run("path with directory routes under origin's source dir", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserRepo", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_repo.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithOutputFilename("test/cli_pinned.go").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Dir, filepath.Join("internal", "users", "test"); got != want {
			t.Fatalf("Target.Dir = %q, want %q (CLI -o path stacked under origin dir)", got, want)
		}
		if got, want := s.Target.Filename, "cli_pinned.go"; got != want {
			t.Fatalf("Target.Filename = %q, want %q (basename of CLI -o path)", got, want)
		}
	})
}

// TestLayout_OutputPackageOverride_ConsolidatesImportPath pins the
// bugfix for the silent-output-loss case the `-o` + `-p` combo
// previously triggered: when two plugins emit decls anchored to the
// same source origin but each builds its own `emit.Package`
// (mockgen's test-package mode produces `<srcPkg>_test`), the
// Layout-phase ImportPath used to diverge even though Dir +
// Filename + Package matched. The recording sink keyed by the full
// Target then captured two manifest entries for the same on-disk
// path, and the backend's last write silently overwrote the first.
//
// Under the fix, `-p generated` resets ImportPath to the source
// package's path (or empty when no source package resolves), so
// every decl landing in the same composed (Dir, Filename, Package)
// shares one Target.
func TestLayout_OutputPackageOverride_ConsolidatesImportPath(t *testing.T) {
	t.Parallel()

	t.Run(
		"two emit.Package paths collapse to one Target when policy.Package overrides",
		func(t *testing.T) {
			t.Parallel()
			origin := &node.Struct{
				BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
				Name:     "X", Package: "example.com/x",
			}
			nodePkg := &node.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*node.Struct{origin},
			}
			pkgA := &emit.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*emit.Struct{
					{
						BaseEmit: emit.BaseEmit{OriginNode: origin},
						Name:     "FromA", Package: "x",
					},
				},
			}
			pkgB := &emit.Package{
				Name: "x_test", Path: "example.com/x_test",
				Structs: []*emit.Struct{
					{
						BaseEmit: emit.BaseEmit{OriginNode: origin},
						Name:     "FromB", Package: "x_test",
					},
				},
			}
			genA := &layoutGen{name: "a", suffix: "_gen.go", pkg: pkgA}
			genB := &layoutGen{name: "b", suffix: "_gen.go", pkg: pkgB}
			p, err := pipeline.New().
				WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
				WithGenerator(genA).
				WithGenerator(genB).
				WithBackend(&stubBE{name: "be"}).
				WithSink(sink.NewMemory()).
				WithOutputPackage("generated").
				Build()
			assertNoError(t, err)
			assertNoError(t, p.Run(t.Context(), "x"))
			aTarget := pkgA.Structs[0].Target
			bTarget := pkgB.Structs[0].Target
			if aTarget != bTarget {
				t.Fatalf("Targets diverged: a=%+v b=%+v", aTarget, bTarget)
			}
			if aTarget.ImportPath != "example.com/x" {
				t.Fatalf(
					"ImportPath = %q, want %q (source package, not plugin emit.Package)",
					aTarget.ImportPath,
					"example.com/x",
				)
			}
		},
	)
}

// TestLayout_OutputPackageOverride verifies CLI -p
// ([Builder.WithOutputPackage]) pins Target.Package under
// alongside-source layout while leaving Target.Dir derived from
// origin — precedence layer 6 applied to Package only.
func TestLayout_OutputPackageOverride(t *testing.T) {
	t.Parallel()

	t.Run("WithOutputPackage pins Target.Package alongside-source", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserRepo", Package: "ignored",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_repo.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithOutputPackage("generated").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Package, "generated"; got != want {
			t.Fatalf("Target.Package = %q, want %q (-p override)", got, want)
		}
		if got, want := s.Target.Dir, "internal/users"; got != want {
			t.Fatalf("Target.Dir = %q, want %q (alongside-source preserved)", got, want)
		}
	})
}

// TestLayout_Centralised verifies [pipeline.LayoutCentralised]
// routes Dir + Package from the resolved policy rather than the
// origin's source location.
func TestLayout_Centralised(t *testing.T) {
	t.Parallel()

	t.Run(
		"centralised layout uses policy.Package for Dir + Package when Dir empty",
		func(t *testing.T) {
			t.Parallel()
			origin := &node.Struct{
				BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
				Name:     "User", Package: "example.com/users",
			}
			s := &emit.Struct{
				BaseEmit: emit.BaseEmit{OriginNode: origin},
				Name:     "UserRepo", Package: "ignored",
			}
			p, err := pipeline.New().
				WithFrontend(&stubFE{name: "fe"}).
				WithGenerator(&layoutGen{name: "rg", suffix: "_repo.go", pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{s},
				}}).
				WithBackend(&stubBE{name: "be"}).
				WithSink(sink.NewMemory()).
				WithOutputLayout(pipeline.LayoutCentralised).
				WithOutputPackage("gen").
				Build()
			assertNoError(t, err)
			assertNoError(t, p.Run(t.Context(), "x"))
			if got, want := s.Target.Dir, "gen"; got != want {
				t.Fatalf(
					"Target.Dir = %q, want %q (centralised uses policy.Package as Dir)",
					got,
					want,
				)
			}
			if got, want := s.Target.Package, "gen"; got != want {
				t.Fatalf("Target.Package = %q, want %q", got, want)
			}
		},
	)

	t.Run("centralised layout with explicit WithOutputDir uses it for Dir", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserRepo", Package: "ignored",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_repo.go", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithOutputLayout(pipeline.LayoutCentralised).
			WithOutputPackage("gen").
			WithOutputDir("internal/gen").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Dir, "internal/gen"; got != want {
			t.Fatalf("Target.Dir = %q, want %q (-output-dir override)", got, want)
		}
		if got, want := s.Target.Package, "gen"; got != want {
			t.Fatalf("Target.Package = %q, want %q", got, want)
		}
	})
}

// TestLayout_SyntheticDecl pins the routing error a decl with nil
// Origin produces. The Layout phase records an Error diagnostic
// and clears the decl's Target so the backend skips it; the run
// returns [pipeline.ErrRunHadErrors].
func TestLayout_SyntheticDecl(t *testing.T) {
	t.Parallel()

	t.Run("nil Origin surfaces a routing error and clears Target", func(t *testing.T) {
		t.Parallel()
		v := &emit.Variable{
			Name: "Sentinel", Package: "boot",
		}
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_gen.go", pkg: &emit.Package{
				Name: "boot", Path: "boot",
				Variables: []*emit.Variable{v},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			Build()
		assertNoError(t, err)
		runErr := p.Run(t.Context(), "x")
		if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run = %v, want ErrRunHadErrors", runErr)
		}
		if v.Target != (emit.Target{}) {
			t.Fatalf("Target = %+v, want zero (synthetic decl)", v.Target)
		}
		if !hasDiagContaining(d, "synthetic variable") {
			t.Fatalf("expected synthetic-decl diagnostic; got %+v", d.Diagnostics())
		}
	})
}

// TestLayout_MissingFilenameProvider pins the routing error a decl
// emitted by a plugin without [plugin.FilenameProvider] produces.
// The sentinel [pipeline.ErrMissingFilenameProvider] appears in the
// diagnostic message; the offending decl's Target is cleared.
func TestLayout_MissingFilenameProvider(t *testing.T) {
	t.Parallel()

	t.Run("plugin without FilenameProvider surfaces the typed sentinel", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserRepo", Package: "users",
		}
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGenNoSuffix{name: "rg", pkg: &emit.Package{
				Name: "users", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			Build()
		assertNoError(t, err)
		runErr := p.Run(t.Context(), "x")
		if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run = %v, want ErrRunHadErrors", runErr)
		}
		if s.Target != (emit.Target{}) {
			t.Fatalf("Target = %+v, want zero (missing FilenameProvider)", s.Target)
		}
		if !hasDiagContaining(d, pipeline.ErrMissingFilenameProvider.Error()) {
			t.Fatalf("expected ErrMissingFilenameProvider diagnostic; got %+v", d.Diagnostics())
		}
	})
}

// TestLayout_AllKinds pins per-kind dispatch — each routable emit
// kind is rerouted via its own dispatch arm. Synthesised origins
// per kind have their basename joined with the plugin's suffix.
func TestLayout_AllKinds(t *testing.T) {
	t.Parallel()

	t.Run(
		"Struct, Interface, Function, Variable, Constant, Enum route via Origin",
		func(t *testing.T) {
			t.Parallel()
			mkOrigin := func(file string) node.Node {
				return &node.Struct{
					BaseNode: node.BaseNode{SourcePos: position.Pos{File: file}},
					Package:  "example.com/x",
				}
			}
			st := &emit.Struct{
				BaseEmit: emit.BaseEmit{OriginNode: mkOrigin("internal/x/types.go")},
				Name:     "S",
				Package:  "x",
			}
			i := &emit.Interface{
				BaseEmit: emit.BaseEmit{OriginNode: mkOrigin("internal/x/types.go")},
				Name:     "I",
				Package:  "x",
			}
			fn := &emit.Function{
				BaseEmit: emit.BaseEmit{OriginNode: mkOrigin("internal/x/fns.go")},
				Name:     "F",
				Package:  "x",
			}
			vd := &emit.Variable{
				BaseEmit: emit.BaseEmit{OriginNode: mkOrigin("internal/x/vars.go")},
				Name:     "V",
				Package:  "x",
			}
			c := &emit.Constant{
				BaseEmit: emit.BaseEmit{OriginNode: mkOrigin("internal/x/consts.go")},
				Name:     "C",
				Package:  "x",
			}
			e := &emit.Enum{
				BaseEmit: emit.BaseEmit{OriginNode: mkOrigin("internal/x/enums.go")},
				Name:     "E",
				Package:  "x",
			}
			p, err := pipeline.New().
				WithFrontend(&stubFE{name: "fe"}).
				WithGenerator(&layoutGen{name: "rg", suffix: "_gen.go", pkg: &emit.Package{
					Name: "x", Path: "example.com/x",
					Structs: []*emit.Struct{st}, Interfaces: []*emit.Interface{i},
					Functions: []*emit.Function{fn}, Variables: []*emit.Variable{vd},
					Constants: []*emit.Constant{c}, Enums: []*emit.Enum{e},
				}}).
				WithBackend(&stubBE{name: "be"}).
				WithSink(sink.NewMemory()).
				Build()
			assertNoError(t, err)
			assertNoError(t, p.Run(t.Context(), "x"))
			cases := []struct {
				kind string
				got  emit.Target
				want string
			}{
				{"Struct", st.Target, "internal/x/types_gen.go"},
				{"Interface", i.Target, "internal/x/types_gen.go"},
				{"Function", fn.Target, "internal/x/fns_gen.go"},
				{"Variable", vd.Target, "internal/x/vars_gen.go"},
				{"Constant", c.Target, "internal/x/consts_gen.go"},
				{"Enum", e.Target, "internal/x/enums_gen.go"},
			}
			for _, tc := range cases {
				if got := tc.got.JoinPath(); got != tc.want {
					t.Errorf("%s routed to %q, want %q", tc.kind, got, tc.want)
				}
			}
		},
	)
}

// TestLayout_AliasFileField pins the alias-specific dispatch:
// [emit.Alias] stores its file Target in the File field, not the
// Target field (which holds the aliased type Ref).
func TestLayout_AliasFileField(t *testing.T) {
	t.Parallel()

	t.Run("Alias.File is resolved from Origin", func(t *testing.T) {
		t.Parallel()
		origin := &node.Alias{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/types/id.go"}},
			Name:     "UserID", Package: "example.com/types",
		}
		a := &emit.Alias{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserID", Package: "types",
			Target: emit.Builtin("string"), IsAlias: true,
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_alias.go", pkg: &emit.Package{
				Name: "types", Path: "example.com/types",
				Aliases: []*emit.Alias{a},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := a.File.JoinPath(), "internal/types/id_alias.go"; got != want {
			t.Fatalf("Alias.File = %q, want %q", got, want)
		}
	})
}

// TestLayout_PendingOriginSlots pins the slot-materialisation pass:
// an origin-anchored slot contribution lands in the resolved File's
// named slot. The File is created via [store.EmitView.FileFor] when
// absent, so the slot lookup after Run finds the materialised
// contribution.
func TestLayout_PendingOriginSlots(t *testing.T) {
	t.Parallel()

	t.Run("AppendOriginSlot tuple lands in resolved File's named slot", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		item := &emit.Constant{Name: "Init", Package: "users"}
		gen := &slotContributingGen{
			name:   "rg",
			suffix: "_meta.go",
			contribute: func(ctx *plugin.GeneratorContext) error {
				return ctx.Store.Emit().
					AppendOriginSlot(origin, "init", item, emit.Provenance{SetBy: "rg"})
			},
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(gen).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		f, ok := p.Store().Emit().Files().ByQName("internal/users/user_meta.go")
		if !ok {
			t.Fatalf("expected File at internal/users/user_meta.go; have %v",
				p.Store().Emit().Files().Items())
		}
		slot := f.Slot("init")
		if got := len(slot.Items); got != 1 {
			t.Fatalf("init slot items = %d, want 1", got)
		}
	})
}

// TestLayout_OneFileOnePackage pins the invariant enforcer: two
// decls routing to the same (Dir, Filename) with conflicting
// Package values surface an error and both decls have their
// Target cleared so the backend skips them.
func TestLayout_OneFileOnePackage(t *testing.T) {
	t.Parallel()

	t.Run(
		"conflicting Package values clear both Targets and surface a diagnostic",
		func(t *testing.T) {
			t.Parallel()
			// Two distinct origins in two distinct source packages.
			// Both carry the same +gen:out directive so the Layout
			// phase composes the same Filename for each; the source
			// directory is the same too (the directive's directory
			// is implicit from origin Pos), so (Dir, Filename) align.
			// Their owning source packages differ — pkgX vs pkgY —
			// so the Layout phase composes divergent Target.Package
			// values under alongside-source mode and the
			// one-file-one-package invariant fires.
			other := &node.Struct{
				BaseNode: node.BaseNode{
					SourcePos: position.Pos{File: "internal/x/x.go"},
					DirectiveList: []*directive.Directive{
						{Name: pipeline.OutDirective, Args: []string{"shared.go"}},
					},
				},
				Name: "Other", Package: "example.com/other",
			}
			altOrigin := &node.Struct{
				BaseNode: node.BaseNode{
					SourcePos: position.Pos{File: "internal/x/x.go"},
					DirectiveList: []*directive.Directive{
						{Name: pipeline.OutDirective, Args: []string{"shared.go"}},
					},
				},
				Name: "Alt", Package: "example.com/x",
			}
			// Two node.Packages so the alongside-source Package field
			// resolves to different values for the two origins.
			pkgX := &node.Package{Name: "x", Path: "example.com/x"}
			pkgY := &node.Package{Name: "y", Path: "example.com/y"}
			altOrigin.Package = "example.com/y"
			s1 := &emit.Struct{BaseEmit: emit.BaseEmit{OriginNode: other}, Name: "S1", Package: "x"}
			s2 := &emit.Struct{
				BaseEmit: emit.BaseEmit{OriginNode: altOrigin},
				Name:     "S2",
				Package:  "y",
			}
			d := diag.New()
			p, err := pipeline.New().
				WithFrontend(&multiNodePackageFE{name: "fe", pkgs: []*node.Package{pkgX, pkgY}}).
				WithGenerator(&layoutGen{name: "rg", suffix: "_gen.go", pkg: &emit.Package{
					Name: "x", Path: "example.com/x",
					Structs: []*emit.Struct{s1, s2},
				}}).
				WithBackend(&stubBE{name: "be"}).
				WithSink(sink.NewMemory()).
				WithDiag(d).
				Build()
			assertNoError(t, err)
			runErr := p.Run(t.Context(), "x")
			if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
				t.Fatalf("Run = %v, want ErrRunHadErrors", runErr)
			}
			if s1.Target != (emit.Target{}) || s2.Target != (emit.Target{}) {
				t.Fatalf("Targets not cleared: s1=%+v s2=%+v", s1.Target, s2.Target)
			}
			if !hasDiagContaining(d, "one-file-one-package") {
				t.Fatalf("expected one-file-one-package diagnostic; got %+v", d.Diagnostics())
			}
		},
	)
}

// TestLayout_OneFileOnePackage_AllKinds extends the conflict path
// across every routable emit kind. One decl per kind from one
// source package collides with another decl-per-kind from a
// second source package on the same (Dir, Filename); each kind's
// arm in [clearConflictedTargets] runs and zeroes its Target /
// File field.
func TestLayout_OneFileOnePackage_AllKinds(t *testing.T) {
	t.Parallel()

	t.Run("every routable kind's Target is cleared when packages conflict", func(t *testing.T) {
		t.Parallel()
		mkOrigin := func(name, pkgPath string) node.Node {
			return &node.Struct{
				BaseNode: node.BaseNode{
					SourcePos: position.Pos{File: "x/x.go"},
					DirectiveList: []*directive.Directive{
						{Name: pipeline.OutDirective, Args: []string{"shared.go"}},
					},
				},
				Name: name, Package: pkgPath,
			}
		}
		oA := mkOrigin("A", "example.com/a")
		oB := mkOrigin("B", "example.com/b")
		pkgA := &node.Package{Name: "a", Path: "example.com/a"}
		pkgB := &node.Package{Name: "b", Path: "example.com/b"}
		// Two decls per kind: one rooted in pkgA, one in pkgB. Their
		// composed (Dir, Filename) is (x, shared.go); their composed
		// Package values are "a" vs "b" — the spec-violating
		// conflict the invariant fires on.
		mk := func() (*emit.Struct, *emit.Struct) {
			return &emit.Struct{
					BaseEmit: emit.BaseEmit{OriginNode: oA},
					Name:     "S_A", Package: "a",
				},
				&emit.Struct{
					BaseEmit: emit.BaseEmit{OriginNode: oB},
					Name:     "S_B", Package: "b",
				}
		}
		_ = mk
		structA := &emit.Struct{BaseEmit: emit.BaseEmit{OriginNode: oA}, Name: "S_A", Package: "a"}
		structB := &emit.Struct{BaseEmit: emit.BaseEmit{OriginNode: oB}, Name: "S_B", Package: "b"}
		ifA := &emit.Interface{BaseEmit: emit.BaseEmit{OriginNode: oA}, Name: "I_A", Package: "a"}
		ifB := &emit.Interface{BaseEmit: emit.BaseEmit{OriginNode: oB}, Name: "I_B", Package: "b"}
		fnA := &emit.Function{BaseEmit: emit.BaseEmit{OriginNode: oA}, Name: "F_A", Package: "a"}
		fnB := &emit.Function{BaseEmit: emit.BaseEmit{OriginNode: oB}, Name: "F_B", Package: "b"}
		varA := &emit.Variable{BaseEmit: emit.BaseEmit{OriginNode: oA}, Name: "V_A", Package: "a"}
		varB := &emit.Variable{BaseEmit: emit.BaseEmit{OriginNode: oB}, Name: "V_B", Package: "b"}
		cA := &emit.Constant{BaseEmit: emit.BaseEmit{OriginNode: oA}, Name: "C_A", Package: "a"}
		cB := &emit.Constant{BaseEmit: emit.BaseEmit{OriginNode: oB}, Name: "C_B", Package: "b"}
		eA := &emit.Enum{BaseEmit: emit.BaseEmit{OriginNode: oA}, Name: "E_A", Package: "a"}
		eB := &emit.Enum{BaseEmit: emit.BaseEmit{OriginNode: oB}, Name: "E_B", Package: "b"}
		aA := &emit.Alias{
			BaseEmit: emit.BaseEmit{OriginNode: oA},
			Name:     "AL_A", Package: "a",
			Target: emit.Builtin("string"), IsAlias: true,
		}
		aB := &emit.Alias{
			BaseEmit: emit.BaseEmit{OriginNode: oB},
			Name:     "AL_B", Package: "b",
			Target: emit.Builtin("string"), IsAlias: true,
		}
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&multiNodePackageFE{name: "fe", pkgs: []*node.Package{pkgA, pkgB}}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_gen.go", pkg: &emit.Package{
				Name: "x", Path: "example.com/x",
				Structs:    []*emit.Struct{structA, structB},
				Interfaces: []*emit.Interface{ifA, ifB},
				Functions:  []*emit.Function{fnA, fnB},
				Variables:  []*emit.Variable{varA, varB},
				Constants:  []*emit.Constant{cA, cB},
				Enums:      []*emit.Enum{eA, eB},
				Aliases:    []*emit.Alias{aA, aB},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			Build()
		assertNoError(t, err)
		runErr := p.Run(t.Context(), "x")
		if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run = %v, want ErrRunHadErrors", runErr)
		}
		// Every kind's pair must be cleared by the invariant enforcer.
		cases := []struct {
			name    string
			cleared emit.Target
		}{
			{"struct_A", structA.Target},
			{"struct_B", structB.Target},
			{"interface_A", ifA.Target},
			{"interface_B", ifB.Target},
			{"function_A", fnA.Target},
			{"function_B", fnB.Target},
			{"variable_A", varA.Target},
			{"variable_B", varB.Target},
			{"constant_A", cA.Target},
			{"constant_B", cB.Target},
			{"enum_A", eA.Target},
			{"enum_B", eB.Target},
			{"alias_A", aA.File},
			{"alias_B", aB.File},
		}
		for _, tc := range cases {
			if tc.cleared != (emit.Target{}) {
				t.Errorf("%s Target not cleared: %+v", tc.name, tc.cleared)
			}
		}
	})
}

// TestLayout_OriginKinds covers every owner-walked node kind
// [originPackagePath] handles: Method and Field walk Owner chains
// up to a packaged ancestor; EnumVariant terminates at its
// *Enum owner; File terminates at its *Package owner; Package
// itself is its own answer. The test exercises each through the
// origin-anchored slot path so a varied Origin reaches
// [composeTarget] for every kind.
func TestLayout_OriginKinds(t *testing.T) {
	t.Parallel()

	mkPkg := func() *node.Package {
		pkg := &node.Package{Name: "x", Path: "example.com/x"}
		owner := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			Name:     "Host", Package: "example.com/x",
		}
		owner.Methods = []*node.Method{
			{
				Name:     "M",
				Owner:    owner,
				BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			},
		}
		owner.Fields = []*node.Field{
			{
				Name:     "F",
				Owner:    owner,
				BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			},
		}
		enum := &node.Enum{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			Name:     "E", Package: "example.com/x",
		}
		enum.Variants = []*node.EnumVariant{
			{
				Name:     "V",
				Owner:    enum,
				BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			},
		}
		file := &node.File{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			Name:     "x.go", Path: "x/x.go", Owner: pkg,
		}
		pkg.Structs = []*node.Struct{owner}
		pkg.Enums = []*node.Enum{enum}
		pkg.Files = []*node.File{file}
		return pkg
	}

	t.Run("Method origin resolves package via Owner chain", func(t *testing.T) {
		t.Parallel()
		pkg := mkPkg()
		method := pkg.Structs[0].Methods[0]
		runOriginKind(t, pkg, method, "method")
	})
	t.Run("Field origin resolves package via Owner chain", func(t *testing.T) {
		t.Parallel()
		pkg := mkPkg()
		field := pkg.Structs[0].Fields[0]
		runOriginKind(t, pkg, field, "field")
	})
	t.Run("EnumVariant origin resolves package via owning Enum", func(t *testing.T) {
		t.Parallel()
		pkg := mkPkg()
		variant := pkg.Enums[0].Variants[0]
		runOriginKind(t, pkg, variant, "variant")
	})
	t.Run("File origin resolves package via its Owner Package", func(t *testing.T) {
		t.Parallel()
		pkg := mkPkg()
		file := pkg.Files[0]
		runOriginKind(t, pkg, file, "file")
	})

	// Top-level kinds expose Package directly — exercising each
	// as an Origin covers the [originPackagePath] arm per kind.
	mkPos := func() position.Pos { return position.Pos{File: "x/x.go"} }
	t.Run("Interface origin reads Package field directly", func(t *testing.T) {
		t.Parallel()
		pkg := &node.Package{Name: "x", Path: "example.com/x"}
		iface := &node.Interface{
			BaseNode: node.BaseNode{SourcePos: mkPos()},
			Name:     "I", Package: "example.com/x",
		}
		pkg.Interfaces = []*node.Interface{iface}
		runOriginKind(t, pkg, iface, "interface")
	})
	t.Run("Function origin reads Package field directly", func(t *testing.T) {
		t.Parallel()
		pkg := &node.Package{Name: "x", Path: "example.com/x"}
		fn := &node.Function{
			BaseNode: node.BaseNode{SourcePos: mkPos()},
			Name:     "F", Package: "example.com/x",
		}
		pkg.Functions = []*node.Function{fn}
		runOriginKind(t, pkg, fn, "function")
	})
	t.Run("Variable origin reads Package field directly", func(t *testing.T) {
		t.Parallel()
		pkg := &node.Package{Name: "x", Path: "example.com/x"}
		v := &node.Variable{
			BaseNode: node.BaseNode{SourcePos: mkPos()},
			Name:     "V", Package: "example.com/x",
		}
		pkg.Variables = []*node.Variable{v}
		runOriginKind(t, pkg, v, "variable")
	})
	t.Run("Constant origin reads Package field directly", func(t *testing.T) {
		t.Parallel()
		pkg := &node.Package{Name: "x", Path: "example.com/x"}
		c := &node.Constant{
			BaseNode: node.BaseNode{SourcePos: mkPos()},
			Name:     "C", Package: "example.com/x",
		}
		pkg.Constants = []*node.Constant{c}
		runOriginKind(t, pkg, c, "constant")
	})
	t.Run("Enum origin reads Package field directly", func(t *testing.T) {
		t.Parallel()
		pkg := &node.Package{Name: "x", Path: "example.com/x"}
		e := &node.Enum{
			BaseNode: node.BaseNode{SourcePos: mkPos()},
			Name:     "E", Package: "example.com/x",
		}
		pkg.Enums = []*node.Enum{e}
		runOriginKind(t, pkg, e, "enum")
	})
	t.Run("Alias origin reads Package field directly", func(t *testing.T) {
		t.Parallel()
		pkg := &node.Package{Name: "x", Path: "example.com/x"}
		a := &node.Alias{
			BaseNode: node.BaseNode{SourcePos: mkPos()},
			Name:     "A", Package: "example.com/x",
		}
		pkg.Aliases = []*node.Alias{a}
		runOriginKind(t, pkg, a, "alias")
	})
}

// TestLayout_SlotMissingFilenameProvider pins
// [materialiseOriginSlots]'s sentinel-firing path: a slot
// contribution carrying a [emit.Provenance.SetBy] that doesn't
// match any registered [plugin.FilenameProvider] surfaces
// [pipeline.ErrMissingFilenameProvider] and drops the
// contribution.
func TestLayout_SlotMissingFilenameProvider(t *testing.T) {
	t.Parallel()

	t.Run("slot tuple from unknown plugin surfaces the typed sentinel", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		gen := &slotContributingGen{
			name:   "rg",
			suffix: "_gen.go",
			contribute: func(ctx *plugin.GeneratorContext) error {
				// Attribute the contribution to a plugin name that
				// is not registered on the pipeline so the suffix
				// lookup misses.
				return ctx.Store.Emit().AppendOriginSlot(
					origin, "init",
					&emit.Constant{Name: "X", Package: "x"},
					emit.Provenance{SetBy: "ghost"},
				)
			},
		}
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(gen).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			Build()
		assertNoError(t, err)
		runErr := p.Run(t.Context(), "x")
		if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run = %v, want ErrRunHadErrors", runErr)
		}
		if !hasDiagContaining(d, pipeline.ErrMissingFilenameProvider.Error()) {
			t.Fatalf("expected ErrMissingFilenameProvider diagnostic; got %+v", d.Diagnostics())
		}
	})
}

// TestLayout_EmptyOriginPackage pins the fall-through path:
// [originSourcePackage] returns nil when origin's owner chain
// never reaches a packaged ancestor (a Method with no Owner is
// the canonical case). Layout still produces a routable Target,
// but with empty Package / ImportPath fields — downstream
// renderers either emit a generic header or surface the missing
// attribution as needed.
func TestLayout_EmptyOriginPackage(t *testing.T) {
	t.Parallel()

	t.Run("Method with nil Owner leaves Target.Package empty", func(t *testing.T) {
		t.Parallel()
		// Method with no Owner — the chain walks to nil and the
		// helper returns the empty package path.
		method := &node.Method{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			Name:     "M",
		}
		item := &emit.Constant{Name: "K", Package: "x"}
		gen := &slotContributingGen{
			name:   "rg",
			suffix: "_meta.go",
			contribute: func(ctx *plugin.GeneratorContext) error {
				return ctx.Store.Emit().AppendOriginSlot(
					method, "init", item, emit.Provenance{SetBy: "rg"},
				)
			},
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(gen).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		// The materialised File exists; its Package value is empty
		// since the lookup found no node.Package.
		f, ok := p.Store().Emit().Files().ByQName("x/x_meta.go")
		if !ok {
			t.Fatalf("expected materialised File; got none")
		}
		if f.Package != "" {
			t.Fatalf("File.Package = %q, want empty (no source package resolution)", f.Package)
		}
	})
}

// TestLayout_EmptyOriginPos pins [originSourceDirBasename]'s
// empty-Pos.File branch: a non-nil origin without a source
// position is treated as having no derivable directory or
// basename. Layout still composes the suffix-only filename
// (e.g. "_meta.go") and leaves the directory empty; the
// downstream sink rejects the empty directory at write time, so
// the failure surfaces a second time at IO if the run still
// attempts the write.
func TestLayout_EmptyOriginPos(t *testing.T) {
	t.Parallel()

	t.Run("Method origin with empty Pos.File yields suffix-only Filename", func(t *testing.T) {
		t.Parallel()
		method := &node.Method{Name: "M"} // no SourcePos
		item := &emit.Constant{Name: "K", Package: "x"}
		gen := &slotContributingGen{
			name:   "rg",
			suffix: "_meta.go",
			contribute: func(ctx *plugin.GeneratorContext) error {
				return ctx.Store.Emit().AppendOriginSlot(
					method, "init", item, emit.Provenance{SetBy: "rg"},
				)
			},
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(gen).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		// Filename is suffix-only; Dir is empty.
		var found *emit.File
		p.Store().Emit().Files().Range(func(f *emit.File) bool {
			if f.Name == "_meta.go" && f.Dir == "" {
				found = f
			}
			return true
		})
		if found == nil {
			t.Fatalf("expected suffix-only file %q with empty Dir", "_meta.go")
		}
	})
}

// runOriginKind drives one slot contribution anchored at origin
// through the pipeline and asserts the resulting File lands under
// the expected (Dir, Filename) — `x/x_meta.go` for every origin
// kind in the fixture above.
func runOriginKind(t *testing.T, pkg *node.Package, origin node.Node, label string) {
	t.Helper()
	item := &emit.Constant{Name: "Init_" + label, Package: "x"}
	gen := &slotContributingGen{
		name:   "rg",
		suffix: "_meta.go",
		contribute: func(ctx *plugin.GeneratorContext) error {
			return ctx.Store.Emit().AppendOriginSlot(
				origin, "init", item, emit.Provenance{SetBy: "rg"},
			)
		},
	}
	p, err := pipeline.New().
		WithFrontend(&nodePackageFE{name: "fe", pkg: pkg}).
		WithGenerator(gen).
		WithBackend(&stubBE{name: "be"}).
		WithSink(sink.NewMemory()).
		Build()
	assertNoError(t, err)
	assertNoError(t, p.Run(t.Context(), "x"))
	f, ok := p.Store().Emit().Files().ByQName("x/x_meta.go")
	if !ok {
		t.Fatalf("%s origin: file at x/x_meta.go missing", label)
	}
	if got := len(f.Slot("init").Items); got != 1 {
		t.Fatalf("%s origin: init slot items = %d, want 1", label, got)
	}
}

// TestLayout_SlotAppendError pins the error path materialiseOriginSlots
// surfaces when [emit.Slot.Append] rejects a kind-mismatched item.
// The slot under test pins its element kind through the typed
// per-host accessor (Method's body slot accepts only Stmt nodes);
// passing a Struct as the slot item triggers the kind check.
func TestLayout_SlotAppendError(t *testing.T) {
	t.Parallel()

	t.Run("kind-mismatched item surfaces a diagnostic", func(t *testing.T) {
		t.Parallel()
		// Layout composes the slot's owning Target from origin —
		// Dir from source dir, Filename from basename+suffix,
		// Package + ImportPath from the node-store lookup. To
		// match the Target the generator pre-pins below, the test
		// loads a node.Package so the lookup resolves to its short
		// name "x" rather than empty.
		nodePkg := &node.Package{Name: "x", Path: "example.com/x"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		nodePkg.Structs = []*node.Struct{origin}
		gen := &slotContributingGen{
			name:   "rg",
			suffix: "_meta.go",
			contribute: func(ctx *plugin.GeneratorContext) error {
				// Pre-create the File at the Target Layout will
				// compose and pin its imports slot's element kind
				// via the typed accessor — the generic File.Slot
				// re-uses the same Slot so the kind check fires on
				// subsequent appends through the by-name path.
				target := emit.Target{
					Dir: "x", Filename: "x_meta.go",
					Package: "x", ImportPath: "example.com/x",
				}
				f, _ := ctx.Store.Emit().FileFor(target)
				_ = f.ImportsSlot()
				// Queue a Constant for the same slot via origin
				// anchoring. Layout composes the same Target and
				// tries to append the Constant to the import-kinded
				// slot — Append rejects on kind mismatch.
				return ctx.Store.Emit().AppendOriginSlot(
					origin, "imports",
					&emit.Constant{Name: "WrongKind", Package: "x"},
					emit.Provenance{SetBy: "rg"},
				)
			},
		}
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(gen).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			Build()
		assertNoError(t, err)
		runErr := p.Run(t.Context(), "x")
		if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run = %v, want ErrRunHadErrors (kind mismatch)", runErr)
		}
		if !hasDiagContaining(d, `slot "imports"`) {
			t.Fatalf("expected slot-append diagnostic; got %+v", d.Diagnostics())
		}
	})
}

// TestLayout_OutDirective_NoArgs pins the defensive arm of
// [outDirectiveFilename]: an `out` directive without a positional
// argument is treated as absent, so the Filename composition
// falls through to the source-basename + suffix default.
func TestLayout_OutDirective_NoArgs(t *testing.T) {
	t.Parallel()

	t.Run("out directive without args leaves Filename composed from suffix", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "x/host.go"},
				DirectiveList: []*directive.Directive{
					{Name: pipeline.OutDirective}, // no Args
				},
			},
			Name: "Host", Package: "example.com/x",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "HostX", Package: "x",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_x.go", pkg: &emit.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Filename, "host_x.go"; got != want {
			t.Fatalf("Filename = %q, want %q (no-args directive falls through)", got, want)
		}
	})
}

// TestLayout_MultiOutputDispatch pins per-output Target
// composition: a plugin returning multiple [plugin.Output]
// entries from [plugin.FilenameProvider.Outputs] dispatches each
// emit decl to its declared output's suffix according to the
// decl's [emit.BaseEmit.OutputTag]. Empty OutputTag resolves to
// the plugin's primary (empty-tag) output; non-empty values
// resolve to the matching Output.Tag.
func TestLayout_MultiOutputDispatch(t *testing.T) {
	t.Parallel()

	t.Run("primary decl uses primary Output.Suffix; tagged decl uses tagged Output.Suffix", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
			},
			Name: "User", Package: "example.com/users",
		}
		primary := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserEnum", Package: "users",
		}
		tagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "UserEnumTest", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{
				name: "enum",
				outputs: []plugin.Output{
					{Suffix: "_enum.go"},
					{Tag: "test", Suffix: "_enum_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{primary, tagged},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := primary.Target.Filename, "user_enum.go"; got != want {
			t.Errorf("primary Target.Filename = %q, want %q", got, want)
		}
		if got, want := tagged.Target.Filename, "user_enum_test.go"; got != want {
			t.Errorf("tagged Target.Filename = %q, want %q", got, want)
		}
	})

	t.Run("unknown tag surfaces a Layout-time error", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		decl := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "nonexistent"},
			Name:     "X", Package: "x",
		}
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{
				name:    "gen",
				outputs: []plugin.Output{{Suffix: "_x.go"}},
				pkg: &emit.Package{
					Name: "x", Path: "example.com/x",
					Structs: []*emit.Struct{decl},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			Build()
		assertNoError(t, err)
		runErr := p.Run(t.Context(), "x")
		if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run = %v, want ErrRunHadErrors", runErr)
		}
		if !hasDiagContaining(d, pipeline.ErrUnknownOutputTag.Error()) {
			t.Fatalf("expected ErrUnknownOutputTag diagnostic; got %+v", d.Diagnostics())
		}
	})

	t.Run("empty OutputTag on plugin without empty-Tag output surfaces a Layout-time error", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		decl := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin}, // empty OutputTag
			Name:     "X", Package: "x",
		}
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{
				name: "gen",
				outputs: []plugin.Output{
					{Tag: "test", Suffix: "_x_test.go"},
				},
				pkg: &emit.Package{
					Name: "x", Path: "example.com/x",
					Structs: []*emit.Struct{decl},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			Build()
		assertNoError(t, err)
		runErr := p.Run(t.Context(), "x")
		if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run = %v, want ErrRunHadErrors", runErr)
		}
		if !hasDiagContaining(d, pipeline.ErrNoDefaultOutput.Error()) {
			t.Fatalf("expected ErrNoDefaultOutput diagnostic; got %+v", d.Diagnostics())
		}
	})
}

// TestLayout_OutDirective_TagScope pins the `tag=` keyword on the
// +gen:out directive: scoping the override to one of the
// plugin's tagged outputs leaves every other output on the
// framework default.
func TestLayout_OutDirective_TagScope(t *testing.T) {
	t.Parallel()

	t.Run("tag= scopes the override to the matching output only", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: pipeline.OutDirective,
						Args: []string{"testkit/"},
						KV:   map[string]string{"tag": "test"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		primary := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserEnum", Package: "users",
		}
		tagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "UserEnumTest", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{
				name: "enum",
				outputs: []plugin.Output{
					{Suffix: "_enum.go"},
					{Tag: "test", Suffix: "_enum_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{primary, tagged},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := primary.Target.Dir, "internal/users"; got != want {
			t.Errorf("primary Target.Dir = %q, want %q (override should not apply)", got, want)
		}
		if got, want := tagged.Target.Dir, filepath.Join("internal", "users", "testkit"); got != want {
			t.Errorf("tagged Target.Dir = %q, want %q", got, want)
		}
	})

	t.Run("tag= composes with pkg= for per-output package override", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: pipeline.OutDirective,
						Args: []string{"testkit/"},
						KV:   map[string]string{"tag": "test", "pkg": "storetest"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		primary := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserEnum", Package: "users",
		}
		tagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "UserEnumTest", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{
				name: "enum",
				outputs: []plugin.Output{
					{Suffix: "_enum.go"},
					{Tag: "test", Suffix: "_enum_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{primary, tagged},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := primary.Target.Package, "users"; got != want {
			t.Errorf("primary Target.Package = %q, want %q (override should not apply)", got, want)
		}
		// pkg= names the package the generator's output belongs to;
		// the _test.go suffix independently says the file is an
		// external test of it. Both apply, so the tagged output lands
		// in storetest_test rather than storetest.
		if got, want := tagged.Target.Package, "storetest_test"; got != want {
			t.Errorf("tagged Target.Package = %q, want %q", got, want)
		}
	})

	t.Run("plugin= + tag= compose as intersection", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: pipeline.OutDirective,
						Args: []string{"testkit/"},
						KV:   map[string]string{"plugin": "enum", "tag": "test"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		// enum:test should match; other:test should NOT.
		enumTest := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "EnumTest", Package: "users",
		}
		otherTest := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "OtherTest", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{
				name: "enum",
				outputs: []plugin.Output{
					{Suffix: "_enum.go"},
					{Tag: "test", Suffix: "_enum_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{enumTest},
				},
			}).
			WithGenerator(&layoutGen{
				name: "other",
				outputs: []plugin.Output{
					{Suffix: "_other.go"},
					{Tag: "test", Suffix: "_other_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{otherTest},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := enumTest.Target.Dir, filepath.Join("internal", "users", "testkit"); got != want {
			t.Errorf("enum:test Target.Dir = %q, want %q (intersection applies)", got, want)
		}
		if got, want := otherTest.Target.Dir, "internal/users"; got != want {
			t.Errorf("other:test Target.Dir = %q, want %q (intersection excludes)", got, want)
		}
	})

	t.Run("unscoped tag= propagates across plugins declaring the same tag", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: pipeline.OutDirective,
						Args: []string{"testkit/"},
						KV:   map[string]string{"tag": "test"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		enumTest := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "EnumTest", Package: "users",
		}
		otherTest := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "OtherTest", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{
				name: "enum",
				outputs: []plugin.Output{
					{Suffix: "_enum.go"},
					{Tag: "test", Suffix: "_enum_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{enumTest},
				},
			}).
			WithGenerator(&layoutGen{
				name: "other",
				outputs: []plugin.Output{
					{Suffix: "_other.go"},
					{Tag: "test", Suffix: "_other_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{otherTest},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		want := filepath.Join("internal", "users", "testkit")
		if got := enumTest.Target.Dir; got != want {
			t.Errorf("enum:test Target.Dir = %q, want %q (cross-plugin propagation)", got, want)
		}
		if got := otherTest.Target.Dir; got != want {
			t.Errorf("other:test Target.Dir = %q, want %q (cross-plugin propagation)", got, want)
		}
	})
}

// TestLayout_UnscopedMultiOutputOverride pins the routing-override
// rejection that prevents a filename-pinning override from
// silently collapsing two outputs into one file. An override
// without `tag=` scoping against a plugin with multiple declared
// outputs surfaces [pipeline.ErrUnscopedMultiOutputOverride] when
// the override pins a filename; directory-only overrides remain
// permitted because per-output suffixes keep the resulting
// filenames distinct.
func TestLayout_UnscopedMultiOutputOverride(t *testing.T) {
	t.Parallel()

	t.Run("unscoped filename-pinning override on multi-output plugin is rejected", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{Name: pipeline.OutDirective, Args: []string{"forced.go"}},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		primary := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "Primary", Package: "users",
		}
		tagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "Tagged", Package: "users",
		}
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{
				name: "enum",
				outputs: []plugin.Output{
					{Suffix: "_enum.go"},
					{Tag: "test", Suffix: "_enum_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{primary, tagged},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			Build()
		assertNoError(t, err)
		runErr := p.Run(t.Context(), "x")
		if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run = %v, want ErrRunHadErrors", runErr)
		}
		if !hasDiagContaining(d, pipeline.ErrUnscopedMultiOutputOverride.Error()) {
			t.Fatalf("expected ErrUnscopedMultiOutputOverride diagnostic; got %+v", d.Diagnostics())
		}
	})

	t.Run("unscoped directory-only override on multi-output plugin is permitted", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{Name: pipeline.OutDirective, Args: []string{"testkit/"}},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		primary := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "Primary", Package: "users",
		}
		tagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "Tagged", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{
				name: "enum",
				outputs: []plugin.Output{
					{Suffix: "_enum.go"},
					{Tag: "test", Suffix: "_enum_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{primary, tagged},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		want := filepath.Join("internal", "users", "testkit")
		if got := primary.Target.Dir; got != want {
			t.Errorf("primary Target.Dir = %q, want %q (directory override applies)", got, want)
		}
		if got := tagged.Target.Dir; got != want {
			t.Errorf("tagged Target.Dir = %q, want %q (directory override applies)", got, want)
		}
		// Filenames still differ via per-output suffix.
		if primary.Target.Filename == tagged.Target.Filename {
			t.Errorf("filenames collapsed onto %q", primary.Target.Filename)
		}
	})
}

// TestLayout_SlotMultiOutputRouting pins the per-output dispatch
// for origin-anchored slot contributions: a slot item whose
// [emit.BaseEmit.OutputTag] names a tagged output lands in the
// matching tagged file's slot, leaving the plugin's primary
// output's file slot untouched. The path makes
// `pkg.File(tag).AppendOriginSlot`-style multi-output slot
// scoping real even though the slot tuple itself doesn't carry
// a tag — the dispatch threads through the item's stamped
// OutputTagName.
func TestLayout_SlotMultiOutputRouting(t *testing.T) {
	t.Parallel()

	t.Run("tagged slot item resolves to the tagged output's File slot", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		gen := &multiOutputSlotGen{origin: origin}
		s := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				// Observe the resolved files via the emit store —
				// slot-materialised files live in the view's Files
				// index, not under any Package.Files slice.
				ctx.Reader.EmitFiles().Each(func(f *emit.File) {
					_ = ctx.Sink.Write(f.Target(), []byte("body"))
				})
			},
		}
		mem := sink.NewMemory()
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(gen).
			WithBackend(s).
			WithSink(mem).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))

		// Layout creates one File per resolved Target. The two slot
		// items (one primary, one tagged) should land in two
		// distinct Files with the per-output filenames.
		files := mem.Files()
		var primaryFound, taggedFound bool
		for tgt := range files {
			switch tgt.Filename {
			case "user_enum.go":
				primaryFound = true
			case "user_enum_test.go":
				taggedFound = true
			}
		}
		if !primaryFound {
			t.Errorf("primary output file (user_enum.go) not created; files=%v", files)
		}
		if !taggedFound {
			t.Errorf("tagged output file (user_enum_test.go) not created; files=%v", files)
		}
	})
}

// multiOutputSlotGen is the fixture for
// [TestLayout_SlotMultiOutputRouting]: a multi-output plugin that
// queues two slot contributions against its origin — one
// untagged and one tagged — and lets the framework dispatch them
// to per-output Files via the OutputTag on each item.
type multiOutputSlotGen struct {
	origin node.Node
}

// Name returns the configured plugin identifier.
func (*multiOutputSlotGen) Name() string { return "enum" }

// Outputs declares the production + test output set.
func (*multiOutputSlotGen) Outputs(_ string) []plugin.Output {
	return []plugin.Output{
		{Suffix: "_enum.go"},
		{Tag: "test", Suffix: "_enum_test.go"},
	}
}

// Generate appends one untagged and one tagged slot contribution
// against the fixture's origin. Both contributions share the same
// slot name; the dispatch routes each to its own File via the
// item's OutputTag.
func (g *multiOutputSlotGen) Generate(ctx *plugin.GeneratorContext) error {
	prov := emit.Provenance{SetBy: "enum"}
	primary := &emit.Struct{
		BaseEmit: emit.BaseEmit{
			SetByName:  "enum",
			OriginNode: g.origin,
		},
		Name:    "PrimaryStub",
		Package: "users",
	}
	tagged := &emit.Struct{
		BaseEmit: emit.BaseEmit{
			SetByName:     "enum",
			OriginNode:    g.origin,
			OutputTagName: "test",
		},
		Name:    "TaggedStub",
		Package: "users",
	}
	if err := ctx.Store.Emit().AppendOriginSlot(g.origin, "top", primary, prov); err != nil {
		return err
	}
	return ctx.Store.Emit().AppendOriginSlot(g.origin, "top", tagged, prov)
}

// TestLayout_TestShift_FiresPerOutput pins per-Target
// independence of the `_test.go → <pkg>_test` package shift.
// Each tagged output composes its own Target; the shift fires
// only on outputs whose resolved filename ends in `_test.go`,
// independently of every other output the same plugin emits.
func TestLayout_TestShift_FiresPerOutput(t *testing.T) {
	t.Parallel()

	t.Run("tagged _test.go output gets the shift; primary stays", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		primary := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "Production", Package: "users",
		}
		tagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "Tested", Package: "users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{
				name: "enum",
				outputs: []plugin.Output{
					{Suffix: "_enum.go"},
					{Tag: "test", Suffix: "_enum_test.go"},
				},
				pkg: &emit.Package{
					Name: "users", Path: "example.com/users",
					Structs: []*emit.Struct{primary, tagged},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := primary.Target.Package, "users"; got != want {
			t.Errorf("primary Target.Package = %q, want %q (no shift)", got, want)
		}
		if got, want := tagged.Target.Package, "users_test"; got != want {
			t.Errorf("tagged Target.Package = %q, want %q (shifted)", got, want)
		}
		if got, want := tagged.Target.ImportPath, "example.com/users_test"; got != want {
			t.Errorf("tagged Target.ImportPath = %q, want %q (shifted)", got, want)
		}
	})
}

// TestLayout_EmptyOutputsIsFiltered pins the collection-boundary
// contract: a plugin that implements [plugin.FilenameProvider]
// but returns an empty Outputs slice for the active language is
// filtered out of the suffix lookup and surfaces
// [pipeline.ErrMissingFilenameProvider] when it emits a routable
// decl — the same code path as a plugin that doesn't implement
// the capability at all.
func TestLayout_EmptyOutputsIsFiltered(t *testing.T) {
	t.Parallel()

	t.Run("empty Outputs() is treated as no declaration", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "X", Package: "x",
		}
		// layoutGen with suffix="" implements FilenameProvider
		// but returns the empty string — must surface the typed
		// sentinel just like a plugin without the capability.
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "", pkg: &emit.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			Build()
		assertNoError(t, err)
		runErr := p.Run(t.Context(), "x")
		if !errors.Is(runErr, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run = %v, want ErrRunHadErrors", runErr)
		}
		if !hasDiagContaining(d, pipeline.ErrMissingFilenameProvider.Error()) {
			t.Fatalf("expected ErrMissingFilenameProvider diagnostic; got %+v", d.Diagnostics())
		}
	})
}

// TestLayout_OutDirective_MixedDirectives pins the directive-skip
// loop in [outDirectiveFilename]: a directive list mixing
// non-`out` directives with the `out` directive must skip the
// non-matching ones and find the `out` entry.
func TestLayout_OutDirective_MixedDirectives(t *testing.T) {
	t.Parallel()

	t.Run("non-out directives are skipped before reaching the out directive", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "x/x.go"},
				DirectiveList: []*directive.Directive{
					{Name: "other"},
					{Name: "another"},
					{Name: pipeline.OutDirective, Args: []string{"pinned.go"}},
				},
			},
			Name: "X", Package: "example.com/x",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "X", Package: "x",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_gen.go", pkg: &emit.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Filename, "pinned.go"; got != want {
			t.Fatalf("Target.Filename = %q, want %q (mixed-directive find)", got, want)
		}
	})
}

// TestLayout_TargetDirRelativeToSourceRoot pins the bugfix that
// kept the manifest from storing absolute paths in `Target.Dir`.
// Absolute paths broke prune (path-doubling under
// `filepath.Join(workdir, absoluteDir, …)`) and made the manifest
// non-portable across machines / working directories.
//
// Under the fix, alongside-source routing strips the configured
// SourceRoot prefix from the origin's source-position directory
// when present, so Target.Dir is the path relative to the project
// root.
func TestLayout_TargetDirRelativeToSourceRoot(t *testing.T) {
	t.Parallel()

	t.Run("absolute origin Pos.File under SourceRoot yields relative Target.Dir", func(t *testing.T) {
		t.Parallel()
		// Absolute origin path that lives under "/var/repo/proj".
		// SourceRoot configured to that same prefix collapses to
		// "internal/x" — every machine that clones the project sees
		// the same Target regardless of where the checkout lives.
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "/var/repo/proj/internal/x/x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "X", Package: "x",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_gen.go", pkg: &emit.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithSourceRoot("/var/repo/proj").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Dir, "internal/x"; got != want {
			t.Fatalf("Target.Dir = %q, want %q (relative to SourceRoot)", got, want)
		}
	})

	t.Run("origin outside SourceRoot retains the absolute Pos.File directory", func(t *testing.T) {
		t.Parallel()
		// SourceRoot at "/a/b" but origin lives at "/c/d" — the
		// helper returns the original absolute path so attribution
		// stays correct even when a fixture sits outside the project
		// tree (rare, but the contract holds).
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "/c/d/file.go"}},
			Name:     "X", Package: "example.com/x",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "X", Package: "x",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_gen.go", pkg: &emit.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithSourceRoot("/a/b").
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Dir, "/c/d"; got != want {
			t.Fatalf("Target.Dir = %q, want %q (absolute fallback)", got, want)
		}
	})
}

// TestLayout_OriginSourceDirBasename_NoDir pins the
// current-directory normalisation in [originSourceDirBasename]:
// a Pos.File without a directory component ("x.go") yields
// filepath.Dir == ".", which the helper normalises to the empty
// string so downstream consumers don't accumulate stray dots in
// rendered paths.
func TestLayout_OriginSourceDirBasename_NoDir(t *testing.T) {
	t.Parallel()

	t.Run("Pos.File without a directory yields empty Target.Dir", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "X", Package: "x",
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&layoutGen{name: "rg", suffix: "_gen.go", pkg: &emit.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if s.Target.Dir != "" {
			t.Fatalf("Target.Dir = %q, want empty (`.` normalisation)", s.Target.Dir)
		}
		if got, want := s.Target.Filename, "x_gen.go"; got != want {
			t.Fatalf("Target.Filename = %q, want %q", got, want)
		}
	})
}

// TestLayout_UnknownLayoutValue pins the production-grade contract
// for an impossible-by-design layout value reaching the Layout
// phase: the switch is exhaustive, the default arm surfaces an
// Internal diagnostic naming the offending plugin / layout, and
// the decl / slot tuple drops from byTarget so the backend skips
// it. The path is normally unreachable because
// [cli.ValidateConfig] rejects bogus layout values; this test
// drives the Builder directly to bypass that gate so the
// invariant guard is observable.
func TestLayout_UnknownLayoutValue(t *testing.T) {
	t.Parallel()

	t.Run(
		"bogus layout surfaces Internal diagnostic and drops decl + pending slot",
		func(t *testing.T) {
			t.Parallel()
			declOrigin := &node.Struct{
				BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
				Name:     "X", Package: "example.com/x",
			}
			slotOrigin := &node.Struct{
				BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/y.go"}},
				Name:     "Y", Package: "example.com/x",
			}
			slotItem := &emit.Constant{
				Name: "K", Package: "x",
				BaseEmit: emit.BaseEmit{SetByName: "rg"},
			}
			gen := &slotContributingGen{
				name:   "rg",
				suffix: "_gen.go",
				contribute: func(ctx *plugin.GeneratorContext) error {
					if err := ctx.Store.Emit().AppendOriginSlot(
						slotOrigin, "init", slotItem, emit.Provenance{SetBy: "rg"},
					); err != nil {
						return err
					}
					decl := &emit.Struct{
						BaseEmit: emit.BaseEmit{OriginNode: declOrigin, SetByName: "rg"},
						Name:     "X", Package: "x",
					}
					return ctx.Store.Emit().AddPackage(&emit.Package{
						Name: "x", Path: "example.com/x",
						Structs: []*emit.Struct{decl},
					})
				},
			}
			d := diag.New()
			p, err := pipeline.New().
				WithFrontend(&stubFE{name: "fe"}).
				WithGenerator(gen).
				WithBackend(&stubBE{name: "be"}).
				WithSink(sink.NewMemory()).
				WithDiag(d).
				WithProjectOutput("bogus-layout", "", "").
				Build()
			assertNoError(t, err)
			_ = p.Run(t.Context())
			if !hasDiagContaining(d, "unknown layout") {
				t.Fatalf("expected Internal diagnostic naming unknown layout; got %+v", d.Diagnostics())
			}
		},
	)
}

// TestLayout_OriginPackagePath_NilOwner pins the helper's
// owner-chain walk for kinds whose Owner can legitimately be nil:
// the helper returns the empty string, [originSourcePackage]
// returns nil, and composeTarget proceeds without overwriting
// Package / ImportPath from the source side. This covers the
// Method, Field, EnumVariant, and File nil-Owner arms.
func TestLayout_OriginPackagePath_NilOwner(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		origin node.Node
	}{
		{"Method with nil Owner", &node.Method{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/m.go"}},
			Name:     "M",
		}},
		{"Field with nil Owner", &node.Field{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/f.go"}},
			Name:     "F",
		}},
		{"EnumVariant with nil Owner", &node.EnumVariant{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/e.go"}},
			Name:     "V",
		}},
		{"File with nil Owner", &node.File{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/f.go"}},
			Name:     "f.go",
		}},
		{"Embed kind not enumerated in the switch", &node.Embed{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/e.go"}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+" routes with empty Package", func(t *testing.T) {
			t.Parallel()
			s := &emit.Struct{
				BaseEmit: emit.BaseEmit{OriginNode: tc.origin},
				Name:     "S", Package: "ignored",
			}
			p, err := pipeline.New().
				WithFrontend(&stubFE{name: "fe"}).
				WithGenerator(&layoutGen{name: "rg", suffix: "_gen.go", pkg: &emit.Package{
					Name: "x", Path: "example.com/x",
					Structs: []*emit.Struct{s},
				}}).
				WithBackend(&stubBE{name: "be"}).
				WithSink(sink.NewMemory()).
				Build()
			assertNoError(t, err)
			assertNoError(t, p.Run(t.Context(), "x"))
			if s.Target.Package != "" {
				t.Fatalf("Target.Package = %q, want empty (no resolvable package)", s.Target.Package)
			}
		})
	}
}

// TestLayout_DivergentResolvedLayout pins the production-grade
// invariant guard in [Pipeline.recordResolvedLayout]: when two
// plugins emit decls that compose to the same [emit.Target] but
// resolve their layout policy via different precedence layers,
// the second write surfaces an Internal diagnostic naming the
// divergent value. The path catches a hypothetical regression
// that would let manifest attribution flip-flop across runs.
//
// Setup: project sets `Package=shared-pkg`; plugin A redundantly
// re-declares the same Package as a per-plugin override (value
// matches but the precedence layer stamps differ); plugin B has
// no per-plugin override. Both emit a routable decl attributed
// to the same source origin so the composed Target aligns but
// the per-field [manifest.Layer] attribution diverges (A stamps
// per-plugin, B stamps project).
func TestLayout_DivergentResolvedLayout(t *testing.T) {
	t.Parallel()

	t.Run("same Target with different ResolvedFrom layer fires Internal diagnostic", func(t *testing.T) {
		t.Parallel()
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		nodePkg := &node.Package{
			Name: "x", Path: "example.com/x",
			Structs: []*node.Struct{origin},
		}
		sA := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "A", Package: "x",
		}
		sB := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "B", Package: "x",
		}
		genA := &layoutGen{name: "a", suffix: "_gen.go", pkg: &emit.Package{
			Name: "x", Path: "example.com/x",
			Structs: []*emit.Struct{sA},
		}}
		genB := &layoutGen{name: "b", suffix: "_gen.go", pkg: &emit.Package{
			Name: "x", Path: "example.com/x",
			Structs: []*emit.Struct{sB},
		}}
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(genA).
			WithGenerator(genB).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			WithProjectOutput(pipeline.LayoutAlongsideSource, "shared-pkg", "").
			WithPluginOutput("a", pipeline.LayoutAlongsideSource, "shared-pkg", "").
			Build()
		assertNoError(t, err)
		_ = p.Run(t.Context(), "x")
		if !hasDiagContaining(d, "divergent ResolvedLayout") {
			t.Fatalf("expected divergence diagnostic; got %+v", d.Diagnostics())
		}
	})
}

// TestPipeline_PluginNames_IncludesAnnotators pins
// [Pipeline.pluginNames]'s registration-order enumeration of
// every role — including annotators. The manifest's
// per-output Plugins list is filtered against this enumeration,
// so registering an annotator alongside a contributing
// generator confirms the annotator's name flows through the
// enumeration (even when no entity carries the annotator's
// SetBy).
func TestPipeline_PluginNames_IncludesAnnotators(t *testing.T) {
	t.Parallel()

	t.Run("annotator registration flows through pluginNames enumeration", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, "manifest.json")
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/x.go"}},
			Name:     "X", Package: "example.com/x",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "X", Package: "x",
		}
		gen := &layoutGen{name: "rg", suffix: "_gen.go", pkg: &emit.Package{
			Name: "x", Path: "example.com/x",
			Structs: []*emit.Struct{s},
		}}
		be := &recBE{name: "be", lang: "stub", render: func(ctx *plugin.BackendContext) {
			ctx.Reader.EmitStructs().Each(func(s *emit.Struct) {
				_ = ctx.Sink.Write(s.Target, []byte("body"))
			})
		}}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: &node.Package{
				Name: "x", Path: "example.com/x",
				Structs: []*node.Struct{origin},
			}}).
			WithAnnotator(&stubAnn{name: "ann"}).
			WithGenerator(gen).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			WithManifestPath(manifestPath).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		if len(m.Outputs) != 1 {
			t.Fatalf("expected 1 output; got %d", len(m.Outputs))
		}
		// pluginNames must have iterated annotators for the
		// manifest write to complete. Plugins list filters by
		// contribution, so "ann" is absent — but the enumeration
		// step ran. The successful manifest write is the
		// observable signal.
		if got := m.Outputs[0].Plugins; len(got) != 1 || got[0].Name != "rg" {
			t.Fatalf("Plugins = %v, want [rg]", got)
		}
	})
}

// TestLayout_CollectContributors_InterfaceMethods pins the
// Interface-methods recursion arm in [collectTargetContributors]:
// when an Interface decl carries methods attributed to a different
// plugin, the manifest's per-output Plugins list includes both
// the Interface's SetBy and each method's SetBy.
func TestLayout_CollectContributors_InterfaceMethods(t *testing.T) {
	t.Parallel()

	t.Run("Interface method SetBy contributes to manifest Plugins list", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		manifestPath := filepath.Join(root, "manifest.json")
		origin := &node.Interface{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/i.go"}},
			Name:     "I", Package: "example.com/x",
		}
		method := &emit.Method{
			BaseEmit: emit.BaseEmit{SetByName: "methgen"}, Name: "Do",
		}
		iface := &emit.Interface{
			BaseEmit: emit.BaseEmit{OriginNode: origin, SetByName: "rg"},
			Name:     "I", Package: "x",
			Methods: []*emit.Method{method},
		}
		nodePkg := &node.Package{
			Name: "x", Path: "example.com/x",
			Interfaces: []*node.Interface{origin},
		}
		gen := &layoutGen{
			name: "rg", suffix: "_gen.go",
			pkg: &emit.Package{
				Name: "x", Path: "example.com/x",
				Interfaces: []*emit.Interface{iface},
			},
		}
		be := &recBE{
			name: "be", lang: "stub",
			render: func(ctx *plugin.BackendContext) {
				ctx.Reader.EmitInterfaces().Each(func(i *emit.Interface) {
					_ = ctx.Sink.Write(i.Target, []byte("body"))
				})
			},
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(gen).
			WithGenerator(&recGen{name: "methgen"}).
			WithBackend(be).
			WithSink(sink.NewMemory()).
			WithManifestPath(manifestPath).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		m, err := manifest.Read(manifestPath)
		assertNoError(t, err)
		if len(m.Outputs) != 1 {
			t.Fatalf("expected 1 output; got %d", len(m.Outputs))
		}
		got := m.Outputs[0].Plugins
		want := map[string]bool{"rg": true, "methgen": true}
		for _, attr := range got {
			delete(want, attr.Name)
		}
		if len(want) != 0 {
			t.Fatalf("Plugins = %v, missing contributors %v", got, want)
		}
	})
}

// TestLayout_OutDirectiveRegistered confirms the `out` directive
// is registered in the framework's core directive set at Build
// time, regardless of whether the caller also supplies directives
// via [Builder.WithDirective].
func TestLayout_OutDirectiveRegistered(t *testing.T) {
	t.Parallel()

	t.Run("out directive registered with no caller-supplied schemas", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		if _, ok := p.DirectiveRegistry().Lookup(pipeline.OutDirective); !ok {
			t.Fatalf("OutDirective should be registered in the core set")
		}
	})
}

// TestLayout_EmptyEmitPackageFallsThroughToSource pins the
// fall-through behaviour for plugins that emit into a package
// builder without a meaningful Name: the resolved Target.Package
// should follow the origin's source package rather than the
// upstream emit.Package's empty Name. This is what lets plugins
// construct their builder with `Package("", "")` (no opinion on
// placement) and have the framework supply the correct identity.
func TestLayout_EmptyEmitPackageFallsThroughToSource(t *testing.T) {
	t.Parallel()

	t.Run("emitPkg.Name empty falls through to srcPkg.Name", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "", Path: "example.com/users", // empty Name — no opinion
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Package, "users"; got != want {
			t.Fatalf("Target.Package = %q, want %q (source-pkg fall-through)", got, want)
		}
		if got, want := s.Target.ImportPath, "example.com/users"; got != want {
			t.Fatalf("Target.ImportPath = %q, want %q (source-pkg fall-through)", got, want)
		}
	})
}

// TestLayout_TestFilenameShiftsPackage pins the framework's
// _test.go convention: when a plugin's filename suffix produces
// a `<basename>_test.go` filename, the resolved Target.Package
// gets an automatic `_test` suffix at the framework-default
// precedence layer. The shift fires only when Package was not
// pinned by a higher precedence layer (explicit `pkg=` on the
// directive, layout policy, CLI -p) — those layers replace the
// value wholesale and short-circuit the shift.
func TestLayout_TestFilenameShiftsPackage(t *testing.T) {
	t.Parallel()

	t.Run("framework default: _test.go suffix shifts pkg to <pkg>_test", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "mt", suffix: "_mock_test.go", pkg: &emit.Package{
				Name: "", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Filename, "user_mock_test.go"; got != want {
			t.Fatalf("Target.Filename = %q, want %q", got, want)
		}
		if got, want := s.Target.Package, "users_test"; got != want {
			t.Fatalf("Target.Package = %q, want %q (_test.go shift)", got, want)
		}
		if got, want := s.Target.ImportPath, "example.com/users_test"; got != want {
			t.Fatalf("Target.ImportPath = %q, want %q (_test.go shift on import path)", got, want)
		}
	})

	// An explicit pkg= answers "which package does this output
	// belong to", not "this file is not a test". Honouring it
	// literally on a _test.go file would put the test inside the
	// package it exists to exercise from outside, silently converting
	// an external test into an internal one.
	t.Run("explicit pkg= via +gen:out still takes the _test shift", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: pipeline.OutDirective,
						Args: []string{""},
						KV:   map[string]string{"pkg": "userstest"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "mt", suffix: "_mock_test.go", pkg: &emit.Package{
				Name: "", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Package, "userstest_test"; got != want {
			t.Fatalf("Target.Package = %q, want %q (explicit pkg= plus shift)", got, want)
		}
		if got, want := s.Target.ImportPath, "example.com/users_test"; got != want {
			t.Fatalf("Target.ImportPath = %q, want %q", got, want)
		}
	})

	// The escape hatch: a caller who genuinely wants the internal
	// form writes the suffix themselves, and the shift leaves it
	// alone rather than producing userstest_test_test.
	t.Run("a pkg= already suffixed _test is not shifted again", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: pipeline.OutDirective,
						Args: []string{""},
						KV:   map[string]string{"pkg": "userstest_test"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "mt", suffix: "_mock_test.go", pkg: &emit.Package{
				Name: "", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Package, "userstest_test"; got != want {
			t.Fatalf("Target.Package = %q, want %q (no double shift)", got, want)
		}
	})

	t.Run("non-_test.go suffix never shifts", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Package, "users"; got != want {
			t.Fatalf("Target.Package = %q, want %q (non-test suffix, no shift)", got, want)
		}
	})
}

// TestLayout_OutDirectiveDirDerivesPackage pins the dir-basename
// derivation: when `+gen:out <relpath>/` carries a directory
// component but no explicit `pkg=` key, the resolved
// Target.Package derives from the resolved directory's basename.
// The _test.go shift still applies on top — the derived pkg is
// framework-attributed, not directive-explicit.
func TestLayout_OutDirectiveDirDerivesPackage(t *testing.T) {
	t.Parallel()

	t.Run("dir-only +gen:out derives pkg from dir basename", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{Name: pipeline.OutDirective, Args: []string{"userstest/"}},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Dir, filepath.Join("internal", "users", "userstest"); got != want {
			t.Fatalf("Target.Dir = %q, want %q", got, want)
		}
		if got, want := s.Target.Package, "userstest"; got != want {
			t.Fatalf("Target.Package = %q, want %q (dir-basename derivation)", got, want)
		}
	})

	t.Run("dir-only +gen:out + _test.go suffix shifts pkg", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{Name: pipeline.OutDirective, Args: []string{"userstest/"}},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "mt", suffix: "_mock_test.go", pkg: &emit.Package{
				Name: "", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Package, "userstest_test"; got != want {
			t.Fatalf("Target.Package = %q, want %q (dir basename + _test shift)", got, want)
		}
	})
}

// TestLayout_PerDirectiveRoutingKeys pins the per-directive
// routing surface: when a plugin owns a directive, the framework
// auto-recognises `out=` and `pkg=` keys on that directive as
// routing overrides scoped to that plugin. Semantically identical
// to a standalone `+gen:out plugin=<name>` directive, but anchored
// at the directive that actually triggers the emission.
func TestLayout_PerDirectiveRoutingKeys(t *testing.T) {
	t.Parallel()

	t.Run("out= on plugin's directive routes to sibling dir + derived pkg", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Interface{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: "mg",
						KV:   map[string]string{"out": "userstest/"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGenWithDirective{
				layoutGen: layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
					Name: "", Path: "example.com/users",
					Structs: []*emit.Struct{s},
				}},
				schema: directive.NewSchema("mg").Build(),
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Dir, filepath.Join("internal", "users", "userstest"); got != want {
			t.Fatalf("Target.Dir = %q, want %q", got, want)
		}
		if got, want := s.Target.Package, "userstest"; got != want {
			t.Fatalf("Target.Package = %q, want %q", got, want)
		}
	})

	t.Run("pkg= on plugin's directive overrides Target.Package", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Interface{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: "mg",
						KV:   map[string]string{"pkg": "userstest"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGenWithDirective{
				layoutGen: layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
					Name: "", Path: "example.com/users",
					Structs: []*emit.Struct{s},
				}},
				schema: directive.NewSchema("mg").Build(),
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Package, "userstest"; got != want {
			t.Fatalf("Target.Package = %q, want %q", got, want)
		}
	})

	t.Run("per-directive keys are scoped to the directive's owner", func(t *testing.T) {
		t.Parallel()
		// A routing override written on one plugin's directive
		// moves that plugin's output and nothing else. Scoping is
		// what lets two generators on one node travel to different
		// places, which is the ordinary case the moment a type
		// carries more than one directive — propagating instead
		// meant the first directive written silently decided where
		// every other plugin's output went. A companion that
		// genuinely needs to follow another plugin uses the
		// standalone `+gen:out plugin=<name>` form, which says so.
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Interface{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: "mg",
						KV:   map[string]string{"out": "mocks/"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		mocked := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		companion := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMockTest", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGenWithDirective{
				layoutGen: layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
					Name: "", Path: "example.com/users",
					Structs: []*emit.Struct{mocked},
				}},
				schema: directive.NewSchema("mg").Build(),
			}).
			WithGenerator(&layoutGen{name: "mgt", suffix: "_mock_test.go", pkg: &emit.Package{
				Name: "", Path: "example.com/users",
				Structs: []*emit.Struct{companion},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		want := filepath.Join("internal", "users", "mocks")
		if got := mocked.Target.Dir; got != want {
			t.Fatalf("mg Target.Dir = %q, want %q (directive owner)", got, want)
		}
		// The companion owns no routing directive, so it falls back
		// to alongside-source rather than inheriting the override.
		notWant := filepath.Join("internal", "users")
		if got := companion.Target.Dir; got != notWant {
			t.Fatalf("companion Target.Dir = %q, want %q (not the owner's override)", got, notWant)
		}
	})
}

// TestLayout_PerDirective_TagScope pins the form-3 `tag=` key on
// an emitter-owned directive: when the directive carries `tag=`,
// the override applies only to the directive's owning plugin's
// matching output and does not propagate to companion plugins
// (tag values are plugin-scoped). Companion-aware propagation of
// `out=` / `pkg=` continues to apply to companion plugins for
// the unscoped path.
func TestLayout_PerDirective_TagScope(t *testing.T) {
	t.Parallel()

	t.Run("tag= scopes the form-3 override to the emitter's matching output only", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Interface{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: "mg",
						KV:   map[string]string{"out": "tests/", "tag": "test"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		primary := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserPrimary", Package: "example.com/users",
		}
		tagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "UserTagged", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGenWithDirective{
				layoutGen: layoutGen{
					name: "mg",
					outputs: []plugin.Output{
						{Suffix: "_mock.go"},
						{Tag: "test", Suffix: "_mock_test.go"},
					},
					pkg: &emit.Package{
						Name: "", Path: "example.com/users",
						Structs: []*emit.Struct{primary, tagged},
					},
				},
				schema: directive.NewSchema("mg").Build(),
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := primary.Target.Dir, "internal/users"; got != want {
			t.Errorf("primary Target.Dir = %q, want %q (override should not apply to primary)", got, want)
		}
		if got, want := tagged.Target.Dir, filepath.Join("internal", "users", "tests"); got != want {
			t.Errorf("tagged Target.Dir = %q, want %q (override applies to matching tag)", got, want)
		}
	})

	t.Run("tag= on form-3 directive does not propagate to companion plugins", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Interface{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/users/user.go"},
				DirectiveList: []*directive.Directive{
					{
						Name: "mg",
						KV:   map[string]string{"out": "tests/", "tag": "test"},
					},
				},
			},
			Name: "User", Package: "example.com/users",
		}
		mgTagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "MgTagged", Package: "example.com/users",
		}
		companionTagged := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, OutputTagName: "test"},
			Name:     "CompanionTagged", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGenWithDirective{
				layoutGen: layoutGen{
					name: "mg",
					outputs: []plugin.Output{
						{Suffix: "_mock.go"},
						{Tag: "test", Suffix: "_mock_test.go"},
					},
					pkg: &emit.Package{
						Name: "", Path: "example.com/users",
						Structs: []*emit.Struct{mgTagged},
					},
				},
				schema: directive.NewSchema("mg").Build(),
			}).
			WithGenerator(&layoutGen{
				name: "companion",
				outputs: []plugin.Output{
					{Suffix: "_comp.go"},
					{Tag: "test", Suffix: "_comp_test.go"},
				},
				pkg: &emit.Package{
					Name: "", Path: "example.com/users",
					Structs: []*emit.Struct{companionTagged},
				},
			}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := mgTagged.Target.Dir, filepath.Join("internal", "users", "tests"); got != want {
			t.Errorf("mg tagged Target.Dir = %q, want %q (form-3 tag override applies)", got, want)
		}
		if got, want := companionTagged.Target.Dir, "internal/users"; got != want {
			t.Errorf("companion tagged Target.Dir = %q, want %q (tag= does not propagate)", got, want)
		}
	})
}

// TestLayout_TestShift_NoDoubleShift pins the
// already-ends-in-_test guard on the framework's `_test.go`
// package-name shift: a plugin that already names its emit.Package
// `<pkg>_test` (the `reference/mockgen` convention) must not see
// the framework append another `_test` suffix on top. Without the
// guard, mockgen-style emitters would produce
// `package <pkg>_test_test` declarations.
func TestLayout_TestShift_NoDoubleShift(t *testing.T) {
	t.Parallel()

	t.Run("plugin-set <pkg>_test name is left untouched", func(t *testing.T) {
		t.Parallel()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Interface{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users_test",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock_test.go", pkg: &emit.Package{
				Name: "users_test", Path: "example.com/users_test", // explicit <pkg>_test
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		if got, want := s.Target.Package, "users_test"; got != want {
			t.Fatalf("Target.Package = %q, want %q (no double-shift)", got, want)
		}
	})
}

// outputPackageSetterItem is a plugin-defined emit kind implementing
// [emit.OutputPackageSetter]. It records what the Layout phase hands it so
// the dispatch contract — right paths, exactly once — is observable
// from a test rather than inferred from rendered output.
type outputPackageSetterItem struct {
	emit.BaseEmit

	// got is a copy of the most recent map received. Copied rather
	// than retained because the contract forbids holding the
	// caller's map, and a test that broke that rule would not notice
	// if the caller later reused the allocation.
	got map[string]string

	// calls counts dispatches. The contract says at most one; a
	// second would mean the walk reached the same value from both
	// the package root and the file root without the seen-set
	// catching it.
	calls int
}

// Kind returns the fixture's plugin-defined kind.
func (*outputPackageSetterItem) Kind() kind.Kind { return "test.outputaware" }

// SetOutputPackages records the dispatch.
func (i *outputPackageSetterItem) SetOutputPackages(byTag map[string]string) {
	i.calls++
	i.got = maps.Clone(byTag)
}

// outputPackageSetterGen queues one plain slot contribution against its
// plugin's primary output and one [outputPackageSetterItem] against its
// tagged output — the shape of a generator emitting a type plus a
// companion that references it.
type outputPackageSetterGen struct {
	origin node.Node
	item   *outputPackageSetterItem

	// dual additionally parks the same item pointer in a slot on a
	// plugin-created [emit.File] registered on an [emit.Package].
	// The dispatch walks packages and files as separate roots, so
	// this is the shape that reaches one value twice — the case the
	// seen-set exists for.
	dual bool
}

// Name returns the fixture plugin identifier.
func (*outputPackageSetterGen) Name() string { return "aware" }

// Outputs declares a primary and a `_test.go`-suffixed companion,
// so the companion's resolved package takes the external-test shift
// and the two outputs land on genuinely different import paths.
func (*outputPackageSetterGen) Outputs(_ string) []plugin.Output {
	return []plugin.Output{
		{Suffix: "_aware.go"},
		{Tag: "test", Suffix: "_aware_test.go"},
	}
}

// Generate queues both contributions against the same origin.
func (g *outputPackageSetterGen) Generate(ctx *plugin.GeneratorContext) error {
	prov := emit.Provenance{SetBy: "aware"}
	primary := &emit.Struct{
		BaseEmit: emit.BaseEmit{SetByName: "aware", OriginNode: g.origin},
		Name:     "Thing", Package: "users",
	}
	if err := ctx.Store.Emit().AppendOriginSlot(g.origin, "top", primary, prov); err != nil {
		return err
	}
	g.item = &outputPackageSetterItem{BaseEmit: emit.BaseEmit{
		SetByName:     "aware",
		OriginNode:    g.origin,
		OutputTagName: "test",
	}}
	if err := ctx.Store.Emit().AppendOriginSlot(g.origin, "top", g.item, prov); err != nil {
		return err
	}
	if !g.dual {
		return nil
	}
	f := &emit.File{
		BaseEmit: emit.BaseEmit{SetByName: "aware", OriginNode: g.origin},
		Name:     "extra.go",
		Package:  "example.com/users",
	}
	if err := f.Slot("top").Append(g.item, prov); err != nil {
		return err
	}
	return ctx.Store.Emit().AddPackage(&emit.Package{
		Name: "users", Path: "example.com/users",
		Files: []*emit.File{f},
	})
}

// TestLayout_DispatchesOutputPackages pins the [emit.OutputPackageSetter]
// contract: a generator that emits two outputs can learn where the
// Layout phase routed each of them, which is the one fact it cannot
// derive during Generate.
func TestLayout_DispatchesOutputPackages(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, dual bool) *outputPackageSetterItem {
		t.Helper()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		gen := &outputPackageSetterGen{origin: origin, dual: dual}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(gen).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
		return gen.item
	}

	t.Run("the primary output's import path arrives under the empty tag", func(t *testing.T) {
		t.Parallel()
		// The empty tag is the framework's name for a plugin's
		// primary output, so this is the lookup a companion uses to
		// qualify a reference back to the type it exercises.
		if got, want := run(t, false).got[""], "example.com/users"; got != want {
			t.Fatalf("byTag[\"\"] = %q, want %q", got, want)
		}
	})

	t.Run("each declared tag arrives under its own key", func(t *testing.T) {
		t.Parallel()
		// The companion's own path carries the external-test shift,
		// which is what makes the two outputs distinguishable at all
		// — equal paths would hide a dispatch that ignored the tag.
		if got, want := run(t, false).got["test"], "example.com/users_test"; got != want {
			t.Fatalf("byTag[\"test\"] = %q, want %q", got, want)
		}
	})

	t.Run("dispatch happens exactly once per value", func(t *testing.T) {
		t.Parallel()
		// The fixture parks one item pointer in both roots the
		// dispatch walks — a package-attached File and the File
		// Layout created for the pending slot — so a missing
		// seen-set shows up here as two calls.
		if got := run(t, true).calls; got != 1 {
			t.Fatalf("SetOutputPackages called %d times, want 1", got)
		}
	})

	t.Run("no tag beyond the plugin's own outputs is reported", func(t *testing.T) {
		t.Parallel()
		// Scoping is per (origin, plugin): a plugin must not observe
		// where some other plugin routed its output for the same
		// source node.
		if got := len(run(t, false).got); got != 2 {
			t.Fatalf("byTag has %d entries, want 2", got)
		}
	})
}

// TestLayout_ConflictingRoutingIsReported pins that two routing
// directives of equal scope, both applying to the same output, are
// reported rather than silently resolved by declaration order.
//
// Nothing in the precedence rules breaks that tie, so whichever the
// author happened to write first would win — and the loser would
// vanish without trace, which is exactly how an explicit `out=` came
// to be ignored before per-directive keys were scoped to their
// owner.
func TestLayout_ConflictingRoutingIsReported(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T, dirs []*directive.Directive) *diag.Sink {
		t.Helper()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos:     position.Pos{File: "internal/users/user.go"},
				DirectiveList: dirs,
			},
			Name: "User", Package: "example.com/users",
		}
		s := &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin},
			Name:     "UserMock", Package: "example.com/users",
		}
		d := diag.New()
		p, err := pipeline.New().
			WithDiag(d).
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&layoutGen{name: "mg", suffix: "_mock.go", pkg: &emit.Package{
				Name: "", Path: "example.com/users",
				Structs: []*emit.Struct{s},
			}}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		// Run reports a non-nil error when the run produced error
		// diagnostics, which is the outcome under test here — so the
		// sink, not the error, is what each case asserts on.
		_ = p.Run(t.Context(), "x")
		return d
	}

	out := func(path, pkg string) *directive.Directive {
		return &directive.Directive{
			Name: pipeline.OutDirective,
			Args: []string{path},
			KV:   map[string]string{"pkg": pkg},
		}
	}

	t.Run("two unscoped +gen:out directives conflict", func(t *testing.T) {
		t.Parallel()
		d := run(t, []*directive.Directive{
			out("mocks/", "usermocks"),
			out("testkit/", "usertest"),
		})
		if !d.HasErrors() {
			t.Fatalf("two equally-scoped routing directives must be reported")
		}
	})

	t.Run("an identical duplicate is not a conflict", func(t *testing.T) {
		t.Parallel()
		// Both resolve to the same place, so declaration order
		// decides nothing and there is no ambiguity to report.
		// Warning here would punish a harmless copy-paste.
		d := run(t, []*directive.Directive{
			out("mocks/", "usermocks"),
			out("mocks/", "usermocks"),
		})
		if d.HasErrors() {
			t.Fatalf("identical duplicates must not be reported: %+v", d.Diagnostics())
		}
	})

	t.Run("a single directive is not a conflict", func(t *testing.T) {
		t.Parallel()
		d := run(t, []*directive.Directive{out("mocks/", "usermocks")})
		if d.HasErrors() {
			t.Fatalf("one directive must not be reported: %+v", d.Diagnostics())
		}
	})
}

// panickingSetter is a plugin-defined emit kind whose
// SetOutputPackages panics — the shape the Layout phase invokes
// directly via dispatchOutputPackages.
type panickingSetter struct {
	emit.BaseEmit
}

// Kind returns the fixture's plugin-defined kind.
func (*panickingSetter) Kind() kind.Kind { return "test.panicsetter" }

// SetOutputPackages panics, standing in for any plugin-authored
// code the Layout phase runs.
func (*panickingSetter) SetOutputPackages(map[string]string) {
	panic("plugin exploded during output-package dispatch")
}

// panickingSetterGen queues the panicking value against its origin.
type panickingSetterGen struct{ origin node.Node }

// Name returns the fixture plugin identifier.
func (*panickingSetterGen) Name() string { return "boom" }

// Outputs declares a primary and a tagged companion so dispatch has
// something to report.
func (*panickingSetterGen) Outputs(_ string) []plugin.Output {
	return []plugin.Output{{Suffix: "_boom.go"}, {Tag: "test", Suffix: "_boom_test.go"}}
}

// Generate queues a well-formed contribution plus the panicking one.
func (g *panickingSetterGen) Generate(ctx *plugin.GeneratorContext) error {
	prov := emit.Provenance{SetBy: "boom"}
	ok := &emit.Struct{
		BaseEmit: emit.BaseEmit{SetByName: "boom", OriginNode: g.origin},
		Name:     "Thing", Package: "users",
	}
	if err := ctx.Store.Emit().AppendOriginSlot(g.origin, "top", ok, prov); err != nil {
		return err
	}
	bad := &panickingSetter{BaseEmit: emit.BaseEmit{
		SetByName: "boom", OriginNode: g.origin, OutputTagName: "test",
	}}
	return ctx.Store.Emit().AppendOriginSlot(g.origin, "top", bad, prov)
}

// TestLayout_PanicIsContained pins that plugin code panicking inside
// the Layout phase becomes a diagnostic rather than killing the
// process.
//
// Run's docblock states the containment contract without
// qualification, but the RecoverAs guard was installed only at the
// frontend, annotator, generator and backend boundaries. Layout
// calls SetOutputPackages on every implementing emit value and walks
// plugin-defined kinds to find them, so it runs plugin-authored code
// with no guard at all.
func TestLayout_PanicIsContained(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T) (*diag.Sink, error) {
		t.Helper()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		d := diag.New()
		p, err := pipeline.New().
			WithDiag(d).
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&panickingSetterGen{origin: origin}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		return d, p.Run(t.Context(), "x")
	}

	t.Run("Run returns instead of taking the process down", func(t *testing.T) {
		t.Parallel()
		// Reaching the assertion at all is the proof: an unguarded
		// panic ends the test binary rather than failing this case.
		if _, err := run(t); err == nil {
			t.Fatalf("a contained panic must still fail the run")
		}
	})

	t.Run("the panic surfaces as a diagnostic naming it", func(t *testing.T) {
		t.Parallel()
		// Containment alone is not enough — swallowing the panic
		// silently would pass the case above while hiding the cause.
		d, _ := run(t)
		for _, g := range d.Diagnostics() {
			if strings.Contains(g.Message, "plugin exploded during output-package dispatch") {
				return
			}
		}
		t.Fatalf("panic was not reported as a diagnostic; diags=%+v", d.Diagnostics())
	})
}

// noOutputsGen is a generator that emits a routable decl while
// declaring no outputs for the active language — the shape that
// trips ErrMissingFilenameProvider.
type noOutputsGen struct {
	origin node.Node
	decl   *emit.Struct
}

// Name returns the fixture plugin identifier.
func (*noOutputsGen) Name() string { return "noout" }

// Outputs declares nothing, so the decl cannot be routed.
func (*noOutputsGen) Outputs(_ string) []plugin.Output { return nil }

// Generate registers a package holding the routable decl.
func (g *noOutputsGen) Generate(ctx *plugin.GeneratorContext) error {
	return ctx.Store.Emit().AddPackage(&emit.Package{
		Name: "users", Path: "example.com/users",
		Structs: []*emit.Struct{g.decl},
	})
}

// TestLayout_SentinelsAreMatchable pins that a Layout-phase failure
// is classifiable with errors.Is.
//
// The four Layout sentinels are declared beside twelve Build-time
// ones, in the same file and the same voice, with docblocks telling
// consumers to compare with errors.Is. Build-time sentinels are
// returned as values and matched; Layout-time ones reached the
// diagnostic sink through Error(), which flattened them to text and
// severed the chain. A host wanting to special-case "a plugin forgot
// FilenameProvider" had to substring-match a message, which
// CONTRIBUTING.md forbids.
func TestLayout_SentinelsAreMatchable(t *testing.T) {
	t.Parallel()

	run := func(t *testing.T) error {
		t.Helper()
		nodePkg := &node.Package{Name: "users", Path: "example.com/users"}
		origin := &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: "internal/users/user.go"}},
			Name:     "User", Package: "example.com/users",
		}
		decl := &emit.Struct{
			BaseEmit: emit.BaseEmit{SetByName: "noout", OriginNode: origin},
			Name:     "UserThing", Package: "example.com/users",
		}
		p, err := pipeline.New().
			WithFrontend(&nodePackageFE{name: "fe", pkg: nodePkg}).
			WithGenerator(&noOutputsGen{origin: origin, decl: decl}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		assertNoError(t, err)
		return p.Run(t.Context(), "x")
	}

	t.Run("the specific sentinel matches", func(t *testing.T) {
		t.Parallel()
		if err := run(t); !errors.Is(err, pipeline.ErrMissingFilenameProvider) {
			t.Fatalf("Run error = %v, want it to match ErrMissingFilenameProvider", err)
		}
	})

	t.Run("ErrRunHadErrors still matches", func(t *testing.T) {
		t.Parallel()
		// Callers matching only the general failure predate this and
		// must keep working.
		if err := run(t); !errors.Is(err, pipeline.ErrRunHadErrors) {
			t.Fatalf("Run error = %v, want it to match ErrRunHadErrors", err)
		}
	})

	t.Run("an unrelated sentinel does not match", func(t *testing.T) {
		t.Parallel()
		// Joining every sentinel unconditionally would satisfy the
		// first case while making the classification meaningless.
		if err := run(t); errors.Is(err, pipeline.ErrUnknownOutputTag) {
			t.Fatalf("Run error = %v matched a sentinel that did not fire", err)
		}
	})
}

// TestLayout_OneFileOnePackage_TwoConflicts pins the diagnostic's
// file prefix across two independent conflicting files.
//
// The grouping key used to be the joined "dir/filename" string and is
// now a comparable struct, so the message reconstructs the path
// rather than printing the key it grouped by. That reconstruction is
// the one user-visible surface this change can move, and a
// single-conflict fixture cannot show that the batched clear still
// reaches every conflicting file.
//
// The emitted order is not asserted: the report loop ranges a Go map,
// so with two or more conflicts the sequence is already unstable.
// That predates this change and is not cured by it.
func TestLayout_OneFileOnePackage_TwoConflicts(t *testing.T) {
	t.Parallel()

	// Two files, each with two origins from different source
	// packages: internal/x/first.go and internal/x/second.go.
	origin := func(dir, out, pkgPath, name string) *node.Struct {
		return &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: dir + "/src.go"},
				DirectiveList: []*directive.Directive{
					{Name: pipeline.OutDirective, Args: []string{out}},
				},
			},
			Name: name, Package: pkgPath,
		}
	}
	mk := func(o *node.Struct, name, pkg string) *emit.Struct {
		return &emit.Struct{BaseEmit: emit.BaseEmit{OriginNode: o}, Name: name, Package: pkg}
	}

	a1 := mk(origin("internal/x", "first.go", "example.com/x", "A1"), "A1", "x")
	a2 := mk(origin("internal/x", "first.go", "example.com/y", "A2"), "A2", "y")
	b1 := mk(origin("internal/x", "second.go", "example.com/x", "B1"), "B1", "x")
	b2 := mk(origin("internal/x", "second.go", "example.com/y", "B2"), "B2", "y")

	pkgX := &node.Package{Name: "x", Path: "example.com/x"}
	pkgY := &node.Package{Name: "y", Path: "example.com/y"}

	d := diag.New()
	p, err := pipeline.New().
		WithFrontend(&multiNodePackageFE{name: "fe", pkgs: []*node.Package{pkgX, pkgY}}).
		WithGenerator(&layoutGen{name: "rg", suffix: "_gen.go", pkg: &emit.Package{
			Name: "x", Path: "example.com/x",
			Structs: []*emit.Struct{a1, a2, b1, b2},
		}}).
		WithBackend(&stubBE{name: "be"}).
		WithSink(sink.NewMemory()).
		WithDiag(d).
		Build()
	assertNoError(t, err)
	if runErr := p.Run(t.Context(), "x"); !errors.Is(runErr, pipeline.ErrRunHadErrors) {
		t.Fatalf("Run = %v, want ErrRunHadErrors", runErr)
	}

	t.Run("both conflicting files are named with their dir prefix", func(t *testing.T) {
		t.Parallel()
		for _, want := range []string{
			"one-file-one-package violation at internal/x/first.go",
			"one-file-one-package violation at internal/x/second.go",
		} {
			if !hasDiagContaining(d, want) {
				t.Fatalf("missing %q in %+v", want, d.Diagnostics())
			}
		}
	})

	t.Run("one clearing pass reaches every conflicting file", func(t *testing.T) {
		t.Parallel()
		// The batched clear walks the buckets once for the whole
		// conflicting set; a membership test that only carried the
		// last group would leave the other file's decls routed.
		for name, s := range map[string]*emit.Struct{"A1": a1, "A2": a2, "B1": b1, "B2": b2} {
			if s.Target != (emit.Target{}) {
				t.Fatalf("%s kept Target %+v", name, s.Target)
			}
		}
	})
}
