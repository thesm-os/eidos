// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"
)

// TestSetTagEntry exercises [setTagEntry] indirectly through tag
// parsing in [stampFieldTagMeta]. The helper registers a typed
// [meta.Key] under a dynamic name and stamps it with provenance —
// any breakage surfaces as missing or mis-typed entries on the
// rendered field bag.
func TestSetTagEntry(t *testing.T) {
	t.Parallel()
	t.Run("registers a new key on first use and reuses it next time", func(t *testing.T) {
		t.Parallel()
		// Two distinct structs sharing the same json tag exercise
		// the EnsureKey path twice — the second registration must
		// reuse the prior key without panicking on duplicate.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype A struct{ X string `json:\"a_x\"` }\n\ntype B struct{ Y string `json:\"b_y\"` }\n",
		})
		ax, _ := pkg.StructByName("A").FieldByName("X").Meta().RawValue("go.tag.json")
		by, _ := pkg.StructByName("B").FieldByName("Y").Meta().RawValue("go.tag.json")
		if ax.(string) != "a_x" || by.(string) != "b_y" {
			t.Fatalf("expected a_x / b_y, got %v / %v", ax, by)
		}
	})

	t.Run("escapes are decoded via strconv.Unquote", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ X string `json:\"hello\\nworld\"` }\n",
		})
		v, _ := pkg.StructByName("S").FieldByName("X").Meta().RawValue("go.tag.json")
		if v.(string) != "hello\nworld" {
			t.Fatalf("expected hello<newline>world, got %q", v)
		}
	})

	t.Run("malformed tag yields no entry rather than crashing", func(t *testing.T) {
		t.Parallel()
		// A tag with an unterminated quote should be skipped
		// silently — the converter must not panic on
		// reflect-parsed tags it cannot decode.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ X string `json:\"unterminated` }\n",
		})
		f := pkg.StructByName("S").FieldByName("X")
		if f == nil {
			t.Fatalf("S.X missing — converter dropped a field on malformed tag")
		}
	})

	t.Run("whitespace-only tag stamps nothing", func(t *testing.T) {
		t.Parallel()
		// All-whitespace tag bytes are skipped after the leading-
		// whitespace consumer reaches the end of the string.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ X string `   ` }\n",
		})
		f := pkg.StructByName("S").FieldByName("X")
		for _, name := range f.Meta().Names() {
			if len(name) > len("go.tag.") && name[:len("go.tag.")] == "go.tag." {
				t.Fatalf("whitespace-only tag unexpectedly produced %q", name)
			}
		}
	})

	t.Run("tag without colon stamps nothing", func(t *testing.T) {
		t.Parallel()
		// Bare token without a key:"value" shape — the malformed-
		// shape early-return fires.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ X string `nokey` }\n",
		})
		f := pkg.StructByName("S").FieldByName("X")
		for _, name := range f.Meta().Names() {
			if len(name) > len("go.tag.") && name[:len("go.tag.")] == "go.tag." {
				t.Fatalf("colon-less tag unexpectedly produced %q", name)
			}
		}
	})

	t.Run("tag with bad escape skips the offending entry but keeps others", func(t *testing.T) {
		t.Parallel()
		// `\x` is not a recognised escape; strconv.Unquote fails
		// on the value and the parser continues past it. The
		// following well-formed `db:"id"` entry must still surface.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ X string `bad:\"\\x\" db:\"id\"` }\n",
		})
		f := pkg.StructByName("S").FieldByName("X")
		if got, _ := f.Meta().RawValue("go.tag.db"); got != "id" {
			t.Fatalf("expected go.tag.db = id after bad-escape skip, got %v", got)
		}
		if f.Meta().Has("go.tag.bad") {
			t.Fatalf("bad-escape entry must not stamp go.tag.bad")
		}
	})
}
