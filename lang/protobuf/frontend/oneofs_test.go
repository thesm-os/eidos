// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/protobuf/frontend"
	"go.thesmos.sh/eidos/node"
)

// TestConvert_Oneofs_SourcePos covers the BaseNode.SourcePos
// contract for synthesized oneof interfaces: each
// `oneof <name> { ... }` block produces an interface whose
// Pos.File anchors back to the declaring proto file.
func TestConvert_Oneofs_SourcePos(t *testing.T) {
	t.Parallel()

	t.Run("synthesized oneof Interface carries the originating proto file path and line", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "messages", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkg := requireSinglePackage(t, env)
		iface := findInterface(pkg, "Container_choice")
		if iface == nil {
			t.Fatalf("Interface %q missing; got %+v", "Container_choice", interfaceNames(pkg))
		}
		pos := iface.Pos()
		if pos.File == "" {
			t.Errorf("synthesized Interface %q carries empty Pos.File", iface.Name)
		}
		if pos.Line == 0 {
			t.Errorf("synthesized Interface %q carries zero Pos.Line", iface.Name)
		}
	})
}

// TestConvert_OneofSynthesizesInterface covers the oneof → node.Interface
// rule: each user-declared oneof group on a message produces one
// node.Interface in the same package using the
// dot-for-nesting-underscore-between-host-and-oneof convention.
// The synthesized interface carries no methods and no embeds —
// variant identity flows through the meta linkage.
func TestConvert_OneofSynthesizesInterface(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)

	t.Run("top-level message oneof produces Container_choice interface", func(t *testing.T) {
		t.Parallel()
		iface := findInterface(pkg, "Container_choice")
		if iface == nil {
			t.Fatalf("Interface %q missing; got %+v", "Container_choice", interfaceNames(pkg))
		}
		if iface.Package != pkg.Path {
			t.Fatalf("Interface.Package = %q, want %q", iface.Package, pkg.Path)
		}
		if len(iface.Methods) != 0 {
			t.Fatalf("synthesized interface should carry no methods; got %d", len(iface.Methods))
		}
		if len(iface.Embeds) != 0 {
			t.Fatalf("synthesized interface should carry no embeds; got %d", len(iface.Embeds))
		}
	})

	t.Run("nested-message oneof produces User.Profile_contact interface (dot then underscore)", func(t *testing.T) {
		t.Parallel()
		iface := findInterface(pkg, "User.Profile_contact")
		if iface == nil {
			t.Fatalf("Interface %q missing; got %+v", "User.Profile_contact", interfaceNames(pkg))
		}
	})

	t.Run("synthetic oneofs from proto3 optional fields are skipped", func(t *testing.T) {
		t.Parallel()
		// User.optional_note uses the proto3 `optional` keyword,
		// which protocompile models as a synthetic one-element
		// oneof. The synthesizer skips synthetic oneofs entirely,
		// so no User_<oneof> interface should land for it.
		for _, i := range pkg.Interfaces {
			if i.Name == "User__optional_note" || i.Name == "User_optional_note" {
				t.Fatalf(
					"synthesized interface %q should not surface; synthetic oneofs are skipped",
					i.Name,
				)
			}
		}
	})
}

// TestConvert_OneofMetaLinkage covers the meta-driven linkage
// between variant fields and their synthesized interface:
// proto.oneof.message on the interface points at the host
// message; proto.oneof.interface on each variant field points
// back at the synthesized interface.
func TestConvert_OneofMetaLinkage(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)

	t.Run("proto.oneof.message on the interface names the host message qualifier", func(t *testing.T) {
		t.Parallel()
		iface := findInterface(pkg, "Container_choice")
		got, ok := frontend.MetaOneofMessage.Get(iface.Meta())
		if !ok {
			t.Fatalf("proto.oneof.message missing on Container_choice")
		}
		want := pkg.Path + ".Container"
		if got != want {
			t.Fatalf("proto.oneof.message = %q, want %q", got, want)
		}
	})

	t.Run("proto.oneof.interface on every variant field names the synthesized interface", func(t *testing.T) {
		t.Parallel()
		container := findStruct(pkg, "Container")
		want := pkg.Path + ".Container_choice"
		for _, name := range []string{"text", "number"} {
			f := container.FieldByName(name)
			if f == nil {
				t.Fatalf("field %q missing on Container", name)
			}
			got, ok := frontend.MetaOneofInterface.Get(f.Meta())
			if !ok {
				t.Fatalf("proto.oneof.interface missing on %q", name)
			}
			if got != want {
				t.Fatalf("proto.oneof.interface on %q = %q, want %q", name, got, want)
			}
		}
		// Non-oneof field stays unmarked.
		tag := container.FieldByName("tag")
		if _, ok := frontend.MetaOneofInterface.Get(tag.Meta()); ok {
			t.Fatalf("non-oneof field tag should not carry proto.oneof.interface")
		}
	})
}

// TestConvert_OneofDocsAndDirectives covers the doc-attachment
// surface on the synthesized interface: leading comments populate
// DocLines, `+gen:` directives parse onto DirectiveList, and the
// trailing same-line comment on the oneof's opening brace stamps
// the dedicated per-host-kind key [frontend.MetaOneofTrailingDoc].
func TestConvert_OneofDocsAndDirectives(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)
	iface := findInterface(pkg, "Container_choice")
	if iface == nil {
		t.Fatalf("Interface %q missing", "Container_choice")
	}

	t.Run("leading comment on the oneof lands on the synthesized interface's DocLines", func(t *testing.T) {
		t.Parallel()
		const want = "choice picks between a text and a numeric payload."
		if !containsLine(iface.DocLines, want) {
			t.Fatalf("Container_choice.DocLines = %+v, want a line containing %q", iface.DocLines, want)
		}
	})

	t.Run("+gen:debug on the oneof lands on DirectiveList", func(t *testing.T) {
		t.Parallel()
		var found bool
		for _, d := range iface.DirectiveList {
			if string(d.Name) == "debug" && !d.Negated {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf(
				"Container_choice.DirectiveList missing a positive `debug` directive; got %+v",
				iface.DirectiveList,
			)
		}
	})

	t.Run("trailing comment on the oneof block stamps proto.oneof.trailing_doc", func(t *testing.T) {
		t.Parallel()
		got, ok := frontend.MetaOneofTrailingDoc.Get(iface.Meta())
		if !ok {
			t.Fatalf("proto.oneof.trailing_doc missing on Container_choice")
		}
		const want = "pick-one"
		if got != "pick-one" && !containsLine([]string{got}, want) {
			t.Fatalf("proto.oneof.trailing_doc = %q, want a substring %q", got, want)
		}
	})
}

// interfaceNames returns the Name of every Interface on pkg.
// Used by the oneof tests for failure-output context.
func interfaceNames(pkg *node.Package) []string {
	out := make([]string, 0, len(pkg.Interfaces))
	for _, i := range pkg.Interfaces {
		out = append(out, i.Name)
	}
	return out
}
