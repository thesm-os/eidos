// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"slices"
	"strings"
	"testing"
)

func TestImportSetNamed(t *testing.T) {
	t.Parallel()

	t.Run("collects several names from one specifier", func(t *testing.T) {
		t.Parallel()
		// The reason this is not writer.ImportSet: one alias per path
		// cannot say `import { A, B } from './y'`.
		s := newImportSet("")
		s.Named("./y", "B", true)
		s.Named("./y", "A", true)

		got := s.Imports()
		if len(got) != 1 {
			t.Fatalf("statements = %d, want 1", len(got))
		}
		if got[0] != "import type { A, B } from './y';" {
			t.Fatalf("statement = %q", got[0])
		}
	})

	t.Run("a repeated name is imported once", func(t *testing.T) {
		t.Parallel()
		s := newImportSet("")
		s.Named("./y", "A", true)
		s.Named("./y", "A", true)

		if got := s.Imports()[0]; strings.Count(got, "A") != 1 {
			t.Fatalf("statement = %q, want A once", got)
		}
	})

	t.Run("a binding from the file's own module renders bare", func(t *testing.T) {
		t.Parallel()
		// A module cannot import itself; emitting the import would
		// make the file fail to load rather than merely look odd.
		s := newImportSet("./self")
		if got := s.Named("./self", "Local", true); got != "Local" {
			t.Fatalf("Named = %q, want the bare name", got)
		}
		if s.Len() != 0 {
			t.Fatal("a self-import was recorded")
		}
	})

	t.Run("an empty name or path records nothing", func(t *testing.T) {
		t.Parallel()
		s := newImportSet("")
		if got := s.Named("./y", "", true); got != "" {
			t.Errorf("Named with no name = %q, want empty", got)
		}
		if got := s.Named("", "A", true); got != "A" {
			t.Errorf("Named with no path = %q, want the bare name", got)
		}
		if s.Len() != 0 {
			t.Fatal("an incomplete import was recorded")
		}
	})
}

func TestImportSetTypeOnly(t *testing.T) {
	t.Parallel()

	t.Run("a specifier contributing only types is type-only", func(t *testing.T) {
		t.Parallel()
		s := newImportSet("")
		s.Named("./y", "A", true)

		if got := s.Imports()[0]; !strings.HasPrefix(got, "import type ") {
			t.Fatalf("statement = %q, want a type-only import", got)
		}
	})

	t.Run("one value binding makes the whole specifier a value import", func(t *testing.T) {
		t.Parallel()
		// Type-only erases at compile time. Marking a specifier that
		// also contributes a value would erase the value with it.
		s := newImportSet("")
		s.Named("./y", "T", true)
		s.Named("./y", "v", false)

		got := s.Imports()[0]
		if strings.HasPrefix(got, "import type ") {
			t.Fatalf("statement = %q, want a value import", got)
		}
		if !strings.Contains(got, "T") || !strings.Contains(got, "v") {
			t.Fatalf("statement = %q, want both bindings", got)
		}
	})

	t.Run("a default or namespace import is never type-only", func(t *testing.T) {
		t.Parallel()
		for _, record := range []func(*importSet){
			func(s *importSet) { s.Default("./y", "D") },
			func(s *importSet) { s.Namespace("./y", "NS") },
		} {
			s := newImportSet("")
			s.Named("./y", "T", true)
			record(s)
			if got := s.Imports()[0]; strings.HasPrefix(got, "import type ") {
				t.Errorf("statement = %q, want a value import", got)
			}
		}
	})
}

func TestImportSetForms(t *testing.T) {
	t.Parallel()

	t.Run("a default import binds the local name", func(t *testing.T) {
		t.Parallel()
		s := newImportSet("")
		if got := s.Default("./d", "D"); got != "D" {
			t.Fatalf("Default = %q", got)
		}
		if got := s.Imports()[0]; got != "import D from './d';" {
			t.Fatalf("statement = %q", got)
		}
	})

	t.Run("a second default keeps the first local name", func(t *testing.T) {
		t.Parallel()
		// A default export carries no name of its own, so two local
		// names would be two imports of one thing.
		s := newImportSet("")
		s.Default("./d", "First")
		if got := s.Default("./d", "Second"); got != "First" {
			t.Fatalf("Default = %q, want the first name", got)
		}
	})

	t.Run("a namespace import renders the star form", func(t *testing.T) {
		t.Parallel()
		s := newImportSet("")
		s.Namespace("./ns", "NS")
		if got := s.Imports()[0]; got != "import * as NS from './ns';" {
			t.Fatalf("statement = %q", got)
		}
	})

	t.Run("the three forms combine in one statement", func(t *testing.T) {
		t.Parallel()
		s := newImportSet("")
		s.Default("./m", "D")
		s.Named("./m", "A", false)
		if got := s.Imports()[0]; got != "import D, { A } from './m';" {
			t.Fatalf("statement = %q", got)
		}
	})

	t.Run("a self-referencing default or namespace records nothing", func(t *testing.T) {
		t.Parallel()
		s := newImportSet("./self")
		s.Default("./self", "D")
		s.Namespace("./self", "NS")
		if s.Len() != 0 {
			t.Fatal("a self-import was recorded")
		}
	})
}

func TestImportSetOrder(t *testing.T) {
	t.Parallel()

	t.Run("packages precede relative specifiers", func(t *testing.T) {
		t.Parallel()
		// The grouping every TypeScript style guide converges on, and
		// the one a reader scans for external dependencies.
		s := newImportSet("")
		s.Named("./local", "L", true)
		s.Named("pkg", "P", true)
		s.Named("../up", "U", true)
		s.Named("@scope/other", "O", true)

		got := s.Imports()
		if len(got) != 4 {
			t.Fatalf("statements = %d, want 4", len(got))
		}
		if !strings.Contains(got[0], "@scope/other") || !strings.Contains(got[1], "pkg") {
			t.Fatalf("packages did not sort first: %v", got)
		}
	})

	t.Run("the block does not depend on the order types were spelled", func(t *testing.T) {
		t.Parallel()
		// First-seen order depends on which declaration the renderer
		// reached first, so a reordering of the emit graph would
		// reorder the imports without changing what the file means.
		forward := newImportSet("")
		forward.Named("./a", "A", true)
		forward.Named("./b", "B", true)

		reverse := newImportSet("")
		reverse.Named("./b", "B", true)
		reverse.Named("./a", "A", true)

		if !slices.Equal(forward.Imports(), reverse.Imports()) {
			t.Fatalf("order reached the block:\n%v\n%v", forward.Imports(), reverse.Imports())
		}
	})

	t.Run("a specifier with no binding renders no statement", func(t *testing.T) {
		t.Parallel()
		s := newImportSet("")
		s.entry("./empty")
		if got := s.Imports(); len(got) != 0 {
			t.Fatalf("statements = %v, want none", got)
		}
	})
}
