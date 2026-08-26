// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
)

func TestFuncMapHelpers(t *testing.T) {
	t.Parallel()

	t.Run("renderDocs drops blank edges", func(t *testing.T) {
		t.Parallel()
		if got := renderDocs([]string{"", "Real.", ""}); got != "/** Real. */\n" {
			t.Fatalf("renderDocs = %q", got)
		}
	})

	t.Run("renderDocs on nothing renders nothing", func(t *testing.T) {
		t.Parallel()
		for _, in := range [][]string{nil, {}, {""}, {"", ""}} {
			if got := renderDocs(in); got != "" {
				t.Errorf("renderDocs(%v) = %q, want empty", in, got)
			}
		}
	})

	t.Run("indentBlock leaves a blank line empty", func(t *testing.T) {
		t.Parallel()
		// Trailing whitespace is what the normaliser strips and what
		// every formatter treats as an error.
		got := indentBlock("a\n\nb")
		for line := range strings.SplitSeq(got, "\n") {
			if strings.TrimSpace(line) == "" && line != "" {
				t.Fatalf("blank line carries whitespace: %q", got)
			}
		}
		if !strings.HasPrefix(got, indent+"a") {
			t.Fatalf("indentBlock = %q, want the first line indented", got)
		}
	})

	t.Run("indentBlock on empty input renders nothing", func(t *testing.T) {
		t.Parallel()
		if got := indentBlock(""); got != "" {
			t.Fatalf("indentBlock(empty) = %q", got)
		}
	})

	t.Run("exportPrefix defaults to exported", func(t *testing.T) {
		t.Parallel()
		// A generated type nothing can import is a type nothing can
		// use, so absence means exported.
		if got := exportPrefix(nil); got != "export " {
			t.Errorf("exportPrefix(nil) = %q, want export", got)
		}
		if got := exportPrefix(&emit.Interface{Name: "A"}); got != "export " {
			t.Errorf("exportPrefix(unmarked) = %q, want export", got)
		}
	})

	t.Run("metaString and metaBool tolerate nil and absence", func(t *testing.T) {
		t.Parallel()
		if got := metaString(nil, "ts.anything"); got != "" {
			t.Errorf("metaString(nil) = %q", got)
		}
		if metaBool(nil, "ts.anything") {
			t.Error("metaBool(nil) reported true")
		}
		iface := &emit.Interface{Name: "A"}
		if got := metaString(iface, "ts.absent"); got != "" {
			t.Errorf("metaString(absent) = %q", got)
		}
		if metaBool(iface, "ts.absent") {
			t.Error("metaBool(absent) reported true")
		}
	})

	t.Run("metaBool reads a stamped key", func(t *testing.T) {
		t.Parallel()
		iface := &emit.Interface{Name: "A"}
		typescript.MetaExported.Set(iface.EnsureMeta(), true, "test")
		if !metaBool(iface, "ts.exported") {
			t.Fatal("metaBool did not read a stamped key")
		}
	})
}

func TestMetaStringReadsAStampedKey(t *testing.T) {
	t.Parallel()

	t.Run("reads a string-valued key", func(t *testing.T) {
		t.Parallel()
		iface := &emit.Interface{Name: "A"}
		typescript.MetaVisibility.Set(iface.EnsureMeta(), typescript.VisibilityPublic, "test")
		if got := metaString(iface, "ts.visibility"); got != typescript.VisibilityPublic {
			t.Fatalf("metaString = %q", got)
		}
	})

	t.Run("a key of another type reads as empty", func(t *testing.T) {
		t.Parallel()
		// Templates read string-keyed because templates are text; a
		// bool key asked for as a string is a template mistake, and
		// empty is what a template renders as nothing.
		iface := &emit.Interface{Name: "A"}
		typescript.MetaExported.Set(iface.EnsureMeta(), true, "test")
		if got := metaString(iface, "ts.exported"); got != "" {
			t.Fatalf("metaString on a bool key = %q, want empty", got)
		}
	})

	t.Run("metaBool on a string key reads as false", func(t *testing.T) {
		t.Parallel()
		iface := &emit.Interface{Name: "A"}
		typescript.MetaVisibility.Set(iface.EnsureMeta(), "public", "test")
		if metaBool(iface, "ts.visibility") {
			t.Fatal("metaBool on a string key reported true")
		}
	})
}
