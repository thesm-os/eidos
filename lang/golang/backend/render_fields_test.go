// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
)

// TestRenderFields_Embeds covers struct embedding via the
// emit.struct template's renderEmbeds invocation.
func TestRenderFields_Embeds(t *testing.T) {
	t.Parallel()

	t.Run("struct with single embed renders the embedded type", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Structs: []*emit.Struct{{
				Name: "Wrapper", Package: "x", Target: target,
				Embeds: []*emit.Embed{
					{Type: emit.External("io", "Reader")},
				},
				Fields: []*emit.Field{{Name: "Closed", Type: emit.Builtin("bool")}},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "io.Reader\n\tClosed bool") {
			t.Fatalf("embed should precede fields; got:\n%s", body)
		}
	})

	t.Run("struct with multiple embeds renders each on its own line", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Structs: []*emit.Struct{{
				Name: "Combined", Package: "x", Target: target,
				Embeds: []*emit.Embed{
					{Type: emit.External("io", "Reader")},
					{Type: emit.External("io", "Closer")},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "io.Reader\n\tio.Closer\n") {
			t.Fatalf("embeds should render adjacent on separate lines; got:\n%s", body)
		}
	})
}

// TestRenderFields_TypeParams covers generic-decl rendering via the
// renderTypeParams helper across struct / interface / alias.
func TestRenderFields_TypeParams(t *testing.T) {
	t.Parallel()

	t.Run("generic struct with 'any' constraint renders [T any]", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Structs: []*emit.Struct{{
				Name: "Box", Package: "x", Target: target,
				TypeParams: []*emit.TypeParam{{Name: "T"}},
				Fields:     []*emit.Field{{Name: "Value", Type: emit.Builtin("int")}},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "type Box[T any] struct {") {
			t.Fatalf("generic struct mismatched; got:\n%s", body)
		}
	})

	t.Run("generic struct with builtin constraint", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Structs: []*emit.Struct{{
				Name: "Box", Package: "x", Target: target,
				TypeParams: []*emit.TypeParam{{
					Name: "T",
					Constraint: &emit.Constraint{
						Embedded: []emit.Ref{emit.Builtin("comparable")},
					},
				}},
				Fields: []*emit.Field{{Name: "Value", Type: emit.Builtin("int")}},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "type Box[T comparable] struct {") {
			t.Fatalf("constrained generic mismatched; got:\n%s", body)
		}
	})

	t.Run("generic interface renders bracketed params after the name", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Interfaces: []*emit.Interface{{
				Name: "Container", Package: "x", Target: target,
				TypeParams: []*emit.TypeParam{{Name: "T"}, {Name: "U"}},
				Methods:    []*emit.Method{{Name: "Add"}},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "type Container[T any, U any] interface {") {
			t.Fatalf("multi-param generic interface mismatched; got:\n%s", body)
		}
	})

	t.Run("generic alias renders params after the name", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Aliases: []*emit.Alias{{
				Name: "List", Package: "x", File: target,
				TypeParams: []*emit.TypeParam{{Name: "T"}},
				Target:     emit.Builtin("int"),
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "type List[T any] int") {
			t.Fatalf("generic alias mismatched; got:\n%s", body)
		}
	})

	t.Run("multi-embed constraint renders inline interface intersection", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Structs: []*emit.Struct{{
				Name: "Pair", Package: "x", Target: target,
				TypeParams: []*emit.TypeParam{{
					Name: "T",
					Constraint: &emit.Constraint{Embedded: []emit.Ref{
						emit.External("io", "Reader"),
						emit.External("io", "Closer"),
					}},
				}},
				Fields: []*emit.Field{{Name: "V", Type: emit.Builtin("int")}},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		// Multi-embed renders the type parameter's bound as an
		// inline-interface intersection. gofmt expands the
		// single-line `interface { A; B }` into a multi-line block,
		// so the assertion checks fragments rather than a one-line
		// substring.
		if !strings.Contains(string(body), "type Pair[T interface {") {
			t.Fatalf("multi-embed constraint should open inline interface; got:\n%s", body)
		}
		if !strings.Contains(string(body), "io.Reader") || !strings.Contains(string(body), "io.Closer") {
			t.Fatalf("expected both embeds io.Reader + io.Closer; got:\n%s", body)
		}
	})
}

// TestRenderFields_TagAggregation covers the renderFields tag-blob
// aggregation: [emit.Field.Tag] (base) unions with `*emit.Tag`
// entries appended to [emit.Field.Tags] into one backtick blob.
func TestRenderFields_TagAggregation(t *testing.T) {
	t.Parallel()

	t.Run("base Tag alone renders inside backticks", func(t *testing.T) {
		t.Parallel()
		body := renderTagsForStruct(t, &emit.Field{
			Name: "ID",
			Type: emit.Builtin("int"),
			Tag:  `json:"id"`,
		})
		if !strings.Contains(body, "`json:\"id\"`") {
			t.Fatalf("base tag should appear in backticks; got:\n%s", body)
		}
	})

	t.Run("slot Tag entries append after base Tag in order", func(t *testing.T) {
		t.Parallel()
		f := &emit.Field{
			Name: "ID",
			Type: emit.Builtin("int"),
			Tag:  `json:"id"`,
		}
		if err := f.Tags().Append(&emit.Tag{Key: "db", Value: "id"}, emit.Provenance{}); err != nil {
			t.Fatalf("Append db tag: %v", err)
		}
		if err := f.Tags().Append(&emit.Tag{Key: "yaml", Value: "id"}, emit.Provenance{}); err != nil {
			t.Fatalf("Append yaml tag: %v", err)
		}
		body := renderTagsForStruct(t, f)
		if !strings.Contains(body, "`json:\"id\" db:\"id\" yaml:\"id\"`") {
			t.Fatalf("aggregated tag blob mismatched; got:\n%s", body)
		}
	})

	t.Run("slot Tag values are Go-string-escaped", func(t *testing.T) {
		t.Parallel()
		f := &emit.Field{Name: "X", Type: emit.Builtin("string")}
		if err := f.Tags().Append(&emit.Tag{Key: "msg", Value: `with "quotes"`}, emit.Provenance{}); err != nil {
			t.Fatalf("Append msg tag: %v", err)
		}
		body := renderTagsForStruct(t, f)
		if !strings.Contains(body, `msg:"with \"quotes\""`) {
			t.Fatalf("tag value should be Go-string-escaped; got:\n%s", body)
		}
	})

	t.Run("field with neither base nor slot tag renders no backticks", func(t *testing.T) {
		t.Parallel()
		body := renderTagsForStruct(t, &emit.Field{Name: "Plain", Type: emit.Builtin("int")})
		if strings.Contains(body, "`") {
			t.Fatalf("empty tag should not produce backticks; got:\n%s", body)
		}
	})
}

// renderTagsForStruct builds a single-field struct with the
// supplied field, renders it, and returns the rendered file body.
// Centralised so per-tag-shape tests stay terse.
func renderTagsForStruct(t *testing.T, f *emit.Field) string {
	t.Helper()
	ctx, mem, d := newBackendContext(t)
	target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
	addEmitPackage(t, ctx, &emit.Package{
		Name: "x", Path: "x",
		Structs: []*emit.Struct{{
			Name: "S", Package: "x", Target: target,
			Fields: []*emit.Field{f},
		}},
	})
	body := assertRenderSucceeds(t, ctx, mem, d, target)
	return string(body)
}
