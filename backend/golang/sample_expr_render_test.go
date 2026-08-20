// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"strings"
	"testing"

	langgo "go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// TestRenderExpr_SampleExprPayloads renders the exact expressions
// [langgo.SampleRefFor] produces for func, channel and constructor
// samples, end to end through the backend.
//
// The lang-side tests assert the expression tree; this one asserts
// the emitted Go, because a tree that is subtly wrong — a func
// literal whose anonymous params render with a stray name, a make
// whose channel type qualifies as `go.chan[T]` — compiles the
// assertion and breaks the consumer. A sample that cannot render is
// worse than a refusal: the refusal names itself.
func TestRenderExpr_SampleExprPayloads(t *testing.T) {
	t.Parallel()

	t.Run("a func sample renders as a no-op literal", func(t *testing.T) {
		t.Parallel()
		s, _ := langgo.SampleRefFor(&node.TypeRef{
			TypeKind:    node.TypeRefFunc,
			FuncParams:  []*node.TypeRef{{TypeKind: node.TypeRefNamed, Name: "string"}},
			FuncReturns: []*node.TypeRef{{TypeKind: node.TypeRefNamed, Name: "error"}},
		}, "fn", nil)
		if s.Expr == nil {
			t.Fatalf("sampler refused: %+v", s)
		}
		body := renderVariableInit(t, s.Expr)
		for _, want := range []string{"func(string) error {", "var r0 error", "return r0"} {
			if !strings.Contains(body, want) {
				t.Fatalf("rendered body should contain %q; got:\n%s", want, body)
			}
		}
	})

	t.Run("a channel sample renders as make of the element", func(t *testing.T) {
		t.Parallel()
		ch := &node.TypeRef{
			TypeKind: node.TypeRefNamed, Package: "go", Name: "chan",
			TypeArgs: []*node.TypeRef{{
				TypeKind: node.TypeRefNamed, Package: "example.com/bus", Name: "Value",
			}},
		}
		langgo.MetaIsChannel.Set(ch.EnsureMeta(), true, "test")
		langgo.MetaChanDir.Set(ch.EnsureMeta(), string(langgo.ChanRecv), "test")
		s, _ := langgo.SampleRefFor(ch, "ch", nil)
		if s.Expr == nil {
			t.Fatalf("sampler refused: %+v", s)
		}
		body := renderVariableInit(t, s.Expr)
		if !strings.Contains(body, "make(chan bus.Value)") {
			t.Fatalf(
				"rendered body should contain a bidirectional make with the import-qualified element; got:\n%s",
				body,
			)
		}
		if !strings.Contains(body, `"example.com/bus"`) {
			t.Fatalf("the element's import should be registered; got:\n%s", body)
		}
	})

	t.Run("a timestamp sample renders as the constructor call", func(t *testing.T) {
		t.Parallel()
		s, _ := langgo.SampleRefFor(&node.TypeRef{
			TypeKind: node.TypeRefNamed, Package: "time", Name: "Time",
		}, "at", nil)
		if s.Expr == nil {
			t.Fatalf("sampler refused: %+v", s)
		}
		body := renderVariableInit(t, s.Expr)
		if !strings.Contains(body, "time.Unix(42, 0)") {
			t.Fatalf("rendered body should contain the constructor call; got:\n%s", body)
		}
		if !strings.Contains(body, `"time"`) {
			t.Fatalf("the time import should be registered; got:\n%s", body)
		}
	})
}

// auditResolver answers the #48 repro struct — the corpus fixture
// whose whole-struct sample rendered `{CreatedAt: }` when the first
// samplable field carried an expression instead of text.
type auditResolver struct{}

func (auditResolver) Resolve(t *node.TypeRef) (node.Node, bool) {
	if t == nil || t.Name != "Audit" {
		return nil, false
	}
	return &node.Struct{
		Name: "Audit", Package: "example.com/audit",
		Fields: []*node.Field{
			{Name: "CreatedAt", Type: &node.TypeRef{
				TypeKind: node.TypeRefNamed, Package: "time", Name: "Time",
			}},
			{Name: "CreatedBy", Type: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"}},
		},
	}, true
}

// TestRenderExpr_CompositeSamplePayloads renders the #48 shapes end
// to end: the failure was only visible at format.Source, which is
// exactly the layer the lang-side structure tests cannot reach.
func TestRenderExpr_CompositeSamplePayloads(t *testing.T) {
	t.Parallel()

	t.Run("a struct with a timestamp field renders whole", func(t *testing.T) {
		t.Parallel()
		s, _ := langgo.SampleRefFor(&node.TypeRef{
			TypeKind: node.TypeRefNamed, Package: "example.com/audit", Name: "Audit",
		}, "a", auditResolver{})
		if s.Expr == nil {
			t.Fatalf("sampler refused: %+v", s)
		}
		body := renderVariableInit(t, s.Expr)
		if !strings.Contains(body, "audit.Audit{CreatedAt: time.Unix(42, 0)}") {
			t.Fatalf("rendered body should hold the composed literal; got:\n%s", body)
		}
		for _, imp := range []string{`"example.com/audit"`, `"time"`} {
			if !strings.Contains(body, imp) {
				t.Fatalf("import %s should be registered; got:\n%s", imp, body)
			}
		}
	})

	t.Run("a slice of timestamps renders whole", func(t *testing.T) {
		t.Parallel()
		s, _ := langgo.SampleRefFor(&node.TypeRef{
			TypeKind: node.TypeRefSlice,
			Elem:     &node.TypeRef{TypeKind: node.TypeRefNamed, Package: "time", Name: "Time"},
		}, "ts", nil)
		if s.Expr == nil {
			t.Fatalf("sampler refused: %+v", s)
		}
		body := renderVariableInit(t, s.Expr)
		if !strings.Contains(body, "[]time.Time{time.Unix(42, 0)}") {
			t.Fatalf("rendered body should hold the composed slice; got:\n%s", body)
		}
	})
}
