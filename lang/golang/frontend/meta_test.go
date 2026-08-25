// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang/frontend"
)

// TestMetaKeyNames verifies every documented `go.*` key carries the
// canonical name. The constants are programmer-facing identifiers
// the rest of the codebase pivots on (cache keys, --explain output,
// directive routing); a typo here would silently shadow the
// expected name without a build error.
//
// Table-driven because every row is the same (Key → expected name)
// uniform shape.
func TestMetaKeyNames(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		key  string
		want string
	}{
		{"MetaIsChannel", frontend.MetaIsChannel.Name(), "go.isChannel"},
		{"MetaChanDir", frontend.MetaChanDir.Name(), "go.chanDir"},
		{"MetaChanElem", frontend.MetaChanElem.Name(), "go.chanElem"},
		{"MetaIsContext", frontend.MetaIsContext.Name(), "go.isContext"},
		{"MetaIsError", frontend.MetaIsError.Name(), "go.isError"},
		{"MetaIsStringer", frontend.MetaIsStringer.Name(), "go.isStringer"},
		{"MetaIsComparable", frontend.MetaIsComparable.Name(), "go.isComparable"},
		{"MetaEmbedsInterface", frontend.MetaEmbedsInterface.Name(), "go.embedsInterface"},
		{"MetaIsEmptyInterface", frontend.MetaIsEmptyInterface.Name(), "go.isEmptyInterface"},
		{"MetaIsConstraintInterface", frontend.MetaIsConstraintInterface.Name(), "go.isConstraintInterface"},
		{"MetaUnderlyingKind", frontend.MetaUnderlyingKind.Name(), "go.underlyingKind"},
		{"MetaIsIterSeq", frontend.MetaIsIterSeq.Name(), "go.isIterSeq"},
		{"MetaIsIterSeq2", frontend.MetaIsIterSeq2.Name(), "go.isIterSeq2"},
		{"MetaIterKeyType", frontend.MetaIterKeyType.Name(), "go.iterKeyType"},
		{"MetaIterValueType", frontend.MetaIterValueType.Name(), "go.iterValueType"},
		{"MetaIotaValue", frontend.MetaIotaValue.Name(), "go.iotaValue"},
		{"MetaReceiverIsPointer", frontend.MetaReceiverIsPointer.Name(), "go.receiverIsPointer"},
		{"MetaConstraintTerms", frontend.MetaConstraintTerms.Name(), "go.constraintTerms"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.key != tc.want {
				t.Fatalf("%s name = %q, want %q", tc.name, tc.key, tc.want)
			}
		})
	}
}

// TestMetaFrontend covers the cross-frontend provenance-marker
// convention: every produced [node.Package] carries the bare
// `frontend` meta key whose value is the producing frontend's
// plugin name. The marker is the scope mechanism downstream bridge
// annotators and the cross-namespace audit step pivot on.
func TestMetaFrontend(t *testing.T) {
	t.Parallel()

	t.Run("key carries the documented bare 'frontend' name", func(t *testing.T) {
		t.Parallel()
		if got := frontend.MetaFrontend.Name(); got != "frontend" {
			t.Fatalf("MetaFrontend name = %q, want %q", got, "frontend")
		}
	})

	t.Run("converter stamps the marker on every produced node.Package", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n",
		})
		got, ok := frontend.MetaFrontend.Get(pkg.Meta())
		if !ok {
			t.Fatalf("MetaFrontend missing on go-frontend package %+v", pkg.Meta())
		}
		if got != frontend.FrontendName {
			t.Fatalf("MetaFrontend = %q, want %q", got, frontend.FrontendName)
		}
	})
}

// TestMetaTagPrefix verifies the documented namespace under which
// struct-tag entries are stamped.
func TestMetaTagPrefix(t *testing.T) {
	t.Parallel()
	t.Run("uses the documented go.tag. namespace", func(t *testing.T) {
		t.Parallel()
		if frontend.MetaTagPrefix != "go.tag." {
			t.Fatalf("MetaTagPrefix = %q, want %q", frontend.MetaTagPrefix, "go.tag.")
		}
	})
}

// TestStampChanMeta covers channel-direction metadata stamped onto
// every chan-typed reference produced by the converter.
func TestStampChanMeta(t *testing.T) {
	t.Parallel()
	t.Run("bidirectional channel records 'both'", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Holder struct { Ch chan int }\n",
		})
		s := pkg.StructByName("Holder")
		if s == nil {
			t.Fatalf("Holder missing")
		}
		f := s.FieldByName("Ch")
		if f == nil || f.Type == nil {
			t.Fatalf("Ch field type missing")
		}
		got, ok := frontend.MetaIsChannel.Get(f.Type.Meta())
		if !ok || !got {
			t.Fatalf("expected MetaIsChannel=true, got (%v, %v)", got, ok)
		}
		dir, ok := frontend.MetaChanDir.Get(f.Type.Meta())
		if !ok || dir != "both" {
			t.Fatalf("expected MetaChanDir=both, got (%q, %v)", dir, ok)
		}
		elem, ok := frontend.MetaChanElem.Get(f.Type.Meta())
		if !ok || elem != "int" {
			t.Fatalf("expected MetaChanElem=int, got (%q, %v)", elem, ok)
		}
	})

	t.Run("send-only channel records 'send'", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Holder struct { Out chan<- int }\n",
		})
		f := pkg.StructByName("Holder").FieldByName("Out")
		dir, _ := frontend.MetaChanDir.Get(f.Type.Meta())
		if dir != "send" {
			t.Fatalf("expected MetaChanDir=send, got %q", dir)
		}
	})

	t.Run("recv-only channel records 'recv'", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Holder struct { In <-chan int }\n",
		})
		f := pkg.StructByName("Holder").FieldByName("In")
		dir, _ := frontend.MetaChanDir.Get(f.Type.Meta())
		if dir != "recv" {
			t.Fatalf("expected MetaChanDir=recv, got %q", dir)
		}
	})
}
