// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
)

// TestGoCallable pins the type-assert helper that lets detectors
// destructure a [node.Node] into a (params, returns) pair without
// re-implementing the kind switch in every detector body.
func TestGoCallable(t *testing.T) {
	t.Parallel()

	t.Run("returns params and returns for a Function", func(t *testing.T) {
		t.Parallel()
		fn := &node.Function{
			Params:  []*node.Param{{Name: "a"}},
			Returns: node.AnonReturns(&node.TypeRef{Name: "error"}),
		}
		p, r := shape.GoCallable(fn)
		if len(p) != 1 || len(r) != 1 {
			t.Fatalf("GoCallable(fn) = %v, %v; want 1 param + 1 return", p, r)
		}
	})

	t.Run("returns params and returns for a Method", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{
			Params:  []*node.Param{{Name: "a"}, {Name: "b"}},
			Returns: node.AnonReturns(&node.TypeRef{Name: "string"}, &node.TypeRef{Name: "error"}),
		}
		p, r := shape.GoCallable(m)
		if len(p) != 2 || len(r) != 2 {
			t.Fatalf("GoCallable(m) = %v, %v; want 2 params + 2 returns", p, r)
		}
	})

	t.Run("returns nil, nil for any other node kind", func(t *testing.T) {
		t.Parallel()
		p, r := shape.GoCallable(&node.Struct{Name: "X"})
		if p != nil || r != nil {
			t.Fatalf("GoCallable(struct) = %v, %v; want nil, nil", p, r)
		}
	})
}

// TestGoContextHelpers covers the context-parameter recognition
// pair: [shape.GoHasContext] reports presence; [shape.GoStripContext]
// drops the leading context.Context when present and is a no-op
// when absent.
func TestGoContextHelpers(t *testing.T) {
	t.Parallel()

	ctxParam := &node.Param{Type: &node.TypeRef{Name: "Context", Package: "context"}}
	other := &node.Param{Type: &node.TypeRef{Name: "string"}}

	t.Run("HasContext: leading context.Context returns true", func(t *testing.T) {
		t.Parallel()
		if !shape.GoHasContext([]*node.Param{ctxParam, other}) {
			t.Fatalf("expected leading context.Context to be recognised")
		}
	})

	t.Run("HasContext: non-context leading param returns false", func(t *testing.T) {
		t.Parallel()
		if shape.GoHasContext([]*node.Param{other}) {
			t.Fatalf("non-context param should not be recognised as context")
		}
	})

	t.Run("HasContext: empty params returns false", func(t *testing.T) {
		t.Parallel()
		if shape.GoHasContext(nil) {
			t.Fatalf("nil params should not be recognised as having context")
		}
	})

	t.Run("StripContext drops the leading context param", func(t *testing.T) {
		t.Parallel()
		got := shape.GoStripContext([]*node.Param{ctxParam, other})
		if len(got) != 1 || got[0] != other {
			t.Fatalf("GoStripContext = %v, want [other]", got)
		}
	})

	t.Run("StripContext is a no-op when no context is present", func(t *testing.T) {
		t.Parallel()
		in := []*node.Param{other}
		got := shape.GoStripContext(in)
		if len(got) != 1 || got[0] != other {
			t.Fatalf("GoStripContext(no-ctx) = %v, want unchanged", got)
		}
	})
}

// TestGoVariadicHelpers covers the trailing-variadic pair:
// [shape.GoTrailingVariadic] returns the trailing variadic param
// when present; [shape.GoStripVariadic] drops it.
func TestGoVariadicHelpers(t *testing.T) {
	t.Parallel()

	plain := &node.Param{Name: "a", Type: &node.TypeRef{Name: "string"}}
	varadic := &node.Param{Name: "opts", Type: &node.TypeRef{Name: "Option"}, Variadic: true}

	t.Run("TrailingVariadic returns the trailing variadic", func(t *testing.T) {
		t.Parallel()
		if got := shape.GoTrailingVariadic([]*node.Param{plain, varadic}); got != varadic {
			t.Fatalf("GoTrailingVariadic = %v, want variadic param", got)
		}
	})

	t.Run("TrailingVariadic returns nil for a non-variadic trailing param", func(t *testing.T) {
		t.Parallel()
		if got := shape.GoTrailingVariadic([]*node.Param{plain}); got != nil {
			t.Fatalf("GoTrailingVariadic = %v, want nil", got)
		}
	})

	t.Run("TrailingVariadic returns nil for empty params", func(t *testing.T) {
		t.Parallel()
		if got := shape.GoTrailingVariadic(nil); got != nil {
			t.Fatalf("GoTrailingVariadic(nil) = %v, want nil", got)
		}
	})

	t.Run("StripVariadic drops the trailing variadic", func(t *testing.T) {
		t.Parallel()
		got := shape.GoStripVariadic([]*node.Param{plain, varadic})
		if len(got) != 1 || got[0] != plain {
			t.Fatalf("GoStripVariadic = %v, want [plain]", got)
		}
	})

	t.Run("StripVariadic is a no-op when no variadic is present", func(t *testing.T) {
		t.Parallel()
		in := []*node.Param{plain}
		got := shape.GoStripVariadic(in)
		if len(got) != 1 || got[0] != plain {
			t.Fatalf("GoStripVariadic(no-variadic) = %v, want unchanged", got)
		}
	})
}

// TestGoErrorHelpers covers the bare-error-return pair:
// [shape.GoErrorIndex] / [shape.GoHasError] find the error;
// [shape.GoStripError] removes it.
func TestGoErrorHelpers(t *testing.T) {
	t.Parallel()

	errRef := &node.TypeRef{Name: "error"}
	valRef := &node.TypeRef{Name: "Article", Package: "x"}

	t.Run("ErrorIndex returns the index of the bare error", func(t *testing.T) {
		t.Parallel()
		if got := shape.GoErrorIndex(node.AnonReturns(valRef, errRef)); got != 1 {
			t.Fatalf("GoErrorIndex = %d, want 1", got)
		}
	})

	t.Run("ErrorIndex returns -1 when no error is present", func(t *testing.T) {
		t.Parallel()
		if got := shape.GoErrorIndex(node.AnonReturns(valRef)); got != -1 {
			t.Fatalf("GoErrorIndex = %d, want -1", got)
		}
	})

	t.Run("HasError reports presence", func(t *testing.T) {
		t.Parallel()
		if !shape.GoHasError(node.AnonReturns(valRef, errRef)) {
			t.Fatalf("GoHasError should be true when error is present")
		}
		if shape.GoHasError(node.AnonReturns(valRef)) {
			t.Fatalf("GoHasError should be false when no error is present")
		}
	})

	t.Run("StripError drops the bare error return", func(t *testing.T) {
		t.Parallel()
		got := shape.GoStripError(node.AnonReturns(valRef, errRef))
		if len(got) != 1 || got[0] != valRef {
			t.Fatalf("GoStripError = %v, want [valRef]", got)
		}
	})

	t.Run("StripError is a no-op when no error is present", func(t *testing.T) {
		t.Parallel()
		in := node.AnonReturns(valRef)
		got := shape.GoStripError(in)
		if len(got) != 1 || got[0] != valRef {
			t.Fatalf("GoStripError(no-error) = %v, want the declared types unchanged", got)
		}
	})
}

// TestGoIterHelpers covers the iter.Seq / iter.Seq2 type-arg
// extractors detectors use to recognise the Go 1.23 range-over-func
// iterator shapes.
func TestGoIterHelpers(t *testing.T) {
	t.Parallel()

	t.Run("IterSeqElem returns the element type for iter.Seq[V]", func(t *testing.T) {
		t.Parallel()
		v := &node.TypeRef{Name: "Article", Package: "x"}
		ref := &node.TypeRef{
			Name: "Seq", Package: "iter",
			TypeArgs: []*node.TypeRef{v},
		}
		if got := shape.GoIterSeqElem(ref); got != v {
			t.Fatalf("GoIterSeqElem = %v, want %v", got, v)
		}
	})

	t.Run("IterSeqElem returns nil for non-iter.Seq", func(t *testing.T) {
		t.Parallel()
		if got := shape.GoIterSeqElem(&node.TypeRef{Name: "string"}); got != nil {
			t.Fatalf("GoIterSeqElem(string) = %v, want nil", got)
		}
	})

	t.Run("IterSeq2Args returns the (K, V) pair for iter.Seq2", func(t *testing.T) {
		t.Parallel()
		k := &node.TypeRef{Name: "string"}
		v := &node.TypeRef{Name: "Article", Package: "x"}
		ref := &node.TypeRef{
			Name: "Seq2", Package: "iter",
			TypeArgs: []*node.TypeRef{k, v},
		}
		gotK, gotV := shape.GoIterSeq2Args(ref)
		if gotK != k || gotV != v {
			t.Fatalf("GoIterSeq2Args = %v, %v; want %v, %v", gotK, gotV, k, v)
		}
	})

	t.Run("IterSeq2Args returns (nil, nil) for non-iter.Seq2", func(t *testing.T) {
		t.Parallel()
		gotK, gotV := shape.GoIterSeq2Args(&node.TypeRef{Name: "string"})
		if gotK != nil || gotV != nil {
			t.Fatalf("GoIterSeq2Args(string) = %v, %v; want nil, nil", gotK, gotV)
		}
	})
}

// TestGoIsBool covers the bare-builtin-bool recognizer used by
// sentinel-bool-returning shape detectors.
func TestGoIsBool(t *testing.T) {
	t.Parallel()

	t.Run("bare bool returns true", func(t *testing.T) {
		t.Parallel()
		if !shape.GoIsBool(&node.TypeRef{Name: "bool"}) {
			t.Fatalf("bare bool should be recognised")
		}
	})

	t.Run("qualified bool returns false", func(t *testing.T) {
		t.Parallel()
		if shape.GoIsBool(&node.TypeRef{Name: "bool", Package: "x"}) {
			t.Fatalf("qualified bool must not be recognised as builtin")
		}
	})

	t.Run("nil ref returns false", func(t *testing.T) {
		t.Parallel()
		if shape.GoIsBool(nil) {
			t.Fatalf("nil ref must not be recognised")
		}
	})
}

// TestGoSliceElem covers the slice-element extractor used by
// detectors that recognise []T-shaped values.
func TestGoSliceElem(t *testing.T) {
	t.Parallel()

	t.Run("slice returns its element type", func(t *testing.T) {
		t.Parallel()
		v := &node.TypeRef{Name: "Article", Package: "x"}
		ref := &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: v}
		if got := shape.GoSliceElem(ref); got != v {
			t.Fatalf("GoSliceElem = %v, want %v", got, v)
		}
	})

	t.Run("non-slice returns nil", func(t *testing.T) {
		t.Parallel()
		if got := shape.GoSliceElem(&node.TypeRef{Name: "string"}); got != nil {
			t.Fatalf("GoSliceElem(non-slice) = %v, want nil", got)
		}
	})
}

// TestGoIsPointerReceiver covers the receiver-pointer-ness
// helper used by detectors that discriminate value- vs
// pointer-receiver methods.
func TestGoIsPointerReceiver(t *testing.T) {
	t.Parallel()

	t.Run("pointer receiver returns true", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{
			Receiver: &node.TypeRef{
				TypeKind: node.TypeRefPointer,
				Elem:     &node.TypeRef{Name: "Repo"},
			},
		}
		if !shape.GoIsPointerReceiver(m) {
			t.Fatalf("expected pointer-receiver method to be recognised")
		}
	})

	t.Run("value receiver returns false", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Receiver: &node.TypeRef{Name: "Repo"}}
		if shape.GoIsPointerReceiver(m) {
			t.Fatalf("value-receiver method must not be recognised as pointer")
		}
	})

	t.Run("interface method (nil receiver) returns false", func(t *testing.T) {
		t.Parallel()
		if shape.GoIsPointerReceiver(&node.Method{}) {
			t.Fatalf("interface-method (nil receiver) must not be recognised")
		}
	})

	t.Run("nil method returns false", func(t *testing.T) {
		t.Parallel()
		if shape.GoIsPointerReceiver(nil) {
			t.Fatalf("nil method must not be recognised")
		}
	})
}

// TestGoPointerElem covers the pointer-element extractor used by
// detectors that recognise *T-shaped values.
func TestGoPointerElem(t *testing.T) {
	t.Parallel()

	t.Run("pointer returns its element type", func(t *testing.T) {
		t.Parallel()
		v := &node.TypeRef{Name: "Article", Package: "x"}
		ref := &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: v}
		if got := shape.GoPointerElem(ref); got != v {
			t.Fatalf("GoPointerElem = %v, want %v", got, v)
		}
	})

	t.Run("non-pointer returns nil", func(t *testing.T) {
		t.Parallel()
		if got := shape.GoPointerElem(&node.TypeRef{Name: "string"}); got != nil {
			t.Fatalf("GoPointerElem(non-pointer) = %v, want nil", got)
		}
	})
}

// TestQName covers the qualified-spelling helper detectors use to
// stamp Match.KeyType / Match.ValueType in the canonical form.
func TestQName(t *testing.T) {
	t.Parallel()

	t.Run("qualified ref returns Package.Name", func(t *testing.T) {
		t.Parallel()
		got := shape.QName(&node.TypeRef{Name: "Article", Package: "x"})
		if got != "x.Article" {
			t.Fatalf("QName = %q, want %q", got, "x.Article")
		}
	})

	t.Run("unqualified ref returns Name only", func(t *testing.T) {
		t.Parallel()
		got := shape.QName(&node.TypeRef{Name: "string"})
		if got != "string" {
			t.Fatalf("QName = %q, want %q", got, "string")
		}
	})

	t.Run("nil ref returns empty string", func(t *testing.T) {
		t.Parallel()
		if got := shape.QName(nil); got != "" {
			t.Fatalf("QName(nil) = %q, want empty", got)
		}
	})
}
