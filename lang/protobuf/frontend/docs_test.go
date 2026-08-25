// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/lang/protobuf/frontend"
)

// TestConvert_Comments_LeadingDocLines covers the
// leading-comment → DocLines attachment: leading `//` comments
// above a declaration land in the declaration's
// [node.BaseNode.DocLines] with comment markers stripped.
func TestConvert_Comments_LeadingDocLines(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)
	user := findStruct(pkg, "User")

	t.Run("leading comment lands on the field's DocLines without comment markers", func(t *testing.T) {
		t.Parallel()
		f := user.FieldByName("id")
		got := f.DocLines
		wantContains := "id is the account's stable unique identifier."
		if !containsLine(got, wantContains) {
			t.Fatalf("id.DocLines = %+v, want a line containing %q", got, wantContains)
		}
	})

	t.Run("leading comment lands on the message's DocLines", func(t *testing.T) {
		t.Parallel()
		got := user.DocLines
		wantContains := "User is the canonical account record."
		if !containsLine(got, wantContains) {
			t.Fatalf("User.DocLines = %+v, want a line containing %q", got, wantContains)
		}
	})
}

// TestConvert_Comments_Detached covers the detached-comment
// attachment rule: a comment block separated from the next
// declaration by a blank line attaches to the nearest following
// declaration. protocompile materialises the detached block on
// the SourceLocation's LeadingDetachedComments slice; the
// converter appends each block to the target's DocLines after
// the immediate leading comments.
func TestConvert_Comments_Detached(t *testing.T) {
	t.Parallel()

	t.Run("detached comment above a field attaches to the field's DocLines", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "messages", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkg := requireSinglePackage(t, env)
		container := findStruct(pkg, "Container")
		marker := container.FieldByName("marker")
		if marker == nil {
			t.Fatalf("field marker missing on Container")
		}
		wantPreamble := "detached preamble routed by protocompile"
		if !containsLine(marker.DocLines, wantPreamble) {
			t.Fatalf("marker.DocLines missing detached preamble; got %+v", marker.DocLines)
		}
		wantLeading := "marker exercises the detached-comment attachment rule"
		if !containsLine(marker.DocLines, wantLeading) {
			t.Fatalf("marker.DocLines missing immediate leading comment; got %+v", marker.DocLines)
		}
	})
}

// TestConvert_Comments_TrailingDoc covers the trailing-comment →
// `proto.<host>.trailing_doc` meta attachment for fields,
// messages, and enum variants. Trailing comments do NOT enter
// DocLines; they live as host-specific meta keys.
func TestConvert_Comments_TrailingDoc(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)

	t.Run("field trailing comment stamps proto.field.trailing_doc", func(t *testing.T) {
		t.Parallel()
		f := findStruct(pkg, "User").FieldByName("id")
		got, ok := frontend.MetaFieldTrailingDoc.Get(f.Meta())
		if !ok {
			t.Fatalf("proto.field.trailing_doc missing on id (DocLines=%+v)", f.DocLines)
		}
		const want = "primary key"
		if !strings.Contains(got, want) {
			t.Fatalf("proto.field.trailing_doc on id = %q, want a substring containing %q", got, want)
		}
	})

	t.Run("message trailing comment stamps proto.message.trailing_doc", func(t *testing.T) {
		t.Parallel()
		s := findStruct(pkg, "User")
		got, ok := frontend.MetaMessageTrailingDoc.Get(s.Meta())
		if !ok {
			t.Fatalf("proto.message.trailing_doc missing on User")
		}
		const want = "primary account"
		if !strings.Contains(got, want) {
			t.Fatalf("proto.message.trailing_doc on User = %q, want a substring containing %q", got, want)
		}
	})

	t.Run("enum variant trailing comment stamps proto.enum_variant.trailing_doc", func(t *testing.T) {
		t.Parallel()
		e := findEnum(pkg, "Status")
		if e == nil {
			t.Fatalf("Enum Status missing")
		}
		for _, v := range e.Variants {
			if v.Name != "STATUS_ACTIVE" {
				continue
			}
			got, ok := frontend.MetaEnumVariantTrailingDoc.Get(v.Meta())
			if !ok {
				t.Fatalf("proto.enum_variant.trailing_doc missing on STATUS_ACTIVE")
			}
			const want = "user is logged in"
			if !strings.Contains(got, want) {
				t.Fatalf("STATUS_ACTIVE trailing_doc = %q, want %q substring", got, want)
			}
			return
		}
		t.Fatalf("STATUS_ACTIVE variant not found")
	})
}

// containsLine reports whether any entry in lines contains needle
// — used by the comment tests because protocompile's
// SourceLocations may surface comment text with leading whitespace
// preserved; a substring match keeps the test resilient.
func containsLine(lines []string, needle string) bool {
	return slices.ContainsFunc(lines, func(l string) bool {
		return strings.Contains(l, needle)
	})
}
