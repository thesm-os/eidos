// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package atomic_test

import (
	"reflect"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/atomic"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = sdk.EnsureKey("frontend", sdk.StringParser)

// TestMixin_Identity pins the constructor invariants: name + the
// zero-param mixin spec.
func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	m := atomic.Mixin()
	if m.Name != atomic.Name {
		t.Fatalf("Mixin().Name = %q, want %q", m.Name, atomic.Name)
	}
	if len(m.Params) != 0 {
		t.Fatalf("Mixin().Params = %v, want empty", m.Params)
	}
}

// TestMixin_DirectiveStamping pins the end-to-end stamping flow
// — registering the mixin and applying its directive lands the
// mixin name in [shape.MetaMixins].
func TestMixin_DirectiveStamping(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Save", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				{Name: shape.MixinDirectiveName, Args: []string{atomic.Name}},
			},
		},
	}
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	p := shape.New().Mixins(atomic.Mixin())
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}

	got := shape.Mixins(fn.Meta())
	want := []string{atomic.Name}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Mixins = %v, want %v", got, want)
	}
	if !slices.Contains(got, atomic.Name) {
		t.Fatalf("Mixins missing %q", atomic.Name)
	}
}
