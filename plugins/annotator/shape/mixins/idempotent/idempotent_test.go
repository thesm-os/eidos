// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package idempotent_test

import (
	"reflect"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/idempotent"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = sdk.EnsureKey("frontend", sdk.StringParser)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	m := idempotent.Mixin()
	if m.Name != idempotent.Name {
		t.Fatalf("Mixin().Name = %q, want %q", m.Name, idempotent.Name)
	}
	if len(m.Params) != 0 {
		t.Fatalf("Mixin().Params = %v, want empty", m.Params)
	}
}

func TestMixin_DirectiveStamping(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Charge", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				{Name: shape.MixinDirectiveName, Args: []string{idempotent.Name}},
			},
		},
	}
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	p := shape.New().Mixins(idempotent.Mixin())
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}

	got := shape.Mixins(fn.Meta())
	want := []string{idempotent.Name}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Mixins = %v, want %v", got, want)
	}
}
