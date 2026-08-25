// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/node"
)

// TestConvert_DirectiveIntegration covers the comment → directive
// translation: a proto declaration carrying a `+gen:<name>` line
// in its leading comments has the parsed [directive.Directive]
// land on the produced node's [node.BaseNode.DirectiveList]. The
// pipeline's gate-fire step reads DirectiveList to decide whether
// a directive-gated plugin (buildergen, repogen, …) runs against
// the node; this test pins the per-node wiring without bringing
// the gated plugins themselves into the picture.
func TestConvert_DirectiveIntegration(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "messages", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := requireSinglePackage(t, env)

	t.Run("-gen:mock on a service parses with Negated = true", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "services", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkgs := collectPackages(t, env)
		var servicesPkg *node.Package
		for _, p := range pkgs {
			if p.Path == "eidos.protobuf.testdata.services" {
				servicesPkg = p
				break
			}
		}
		if servicesPkg == nil {
			t.Fatalf("services package missing")
		}
		var silent *node.Interface
		for _, i := range servicesPkg.Interfaces {
			if i.Name == "SilentService" {
				silent = i
				break
			}
		}
		if silent == nil {
			t.Fatalf("Interface SilentService missing")
		}
		var found *directive.Directive
		for _, d := range silent.DirectiveList {
			if string(d.Name) == "mock" {
				found = d
				break
			}
		}
		if found == nil {
			t.Fatalf("expected `mock` directive on SilentService; got %+v", directiveNames(silent.DirectiveList))
		}
		if !found.Negated {
			t.Fatalf("expected `mock` directive Negated = true; got false")
		}
	})

	t.Run("path-aware +gen:out with plugin + pkg KV-args parses through the proto comment surface", func(t *testing.T) {
		t.Parallel()
		user := findStruct(pkg, "User")
		if user == nil {
			t.Fatalf("Struct User missing")
		}
		var out *directive.Directive
		for _, d := range user.DirectiveList {
			if string(d.Name) == "out" {
				out = d
				break
			}
		}
		if out == nil {
			t.Fatalf("expected +gen:out directive on User; got %+v", directiveNames(user.DirectiveList))
		}
		if len(out.Args) == 0 || out.Args[0] != "user_extra.go" {
			t.Fatalf("+gen:out positional arg = %+v, want first arg \"user_extra.go\"", out.Args)
		}
		if got := out.KV["plugin"]; got != "buildergen" {
			t.Fatalf("+gen:out plugin=%q, want %q", got, "buildergen")
		}
		if got := out.KV["pkg"]; got != "usersx" {
			t.Fatalf("+gen:out pkg=%q, want %q", got, "usersx")
		}
	})

	t.Run("+gen:builder on a proto message lands on the Struct's DirectiveList", func(t *testing.T) {
		t.Parallel()
		user := findStruct(pkg, "User")
		if user == nil {
			t.Fatalf("Struct %q missing", "User")
		}
		var found bool
		for _, d := range user.DirectiveList {
			if string(d.Name) == "builder" && !d.Negated {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf(
				"User.DirectiveList missing a positive `builder` directive; got %+v",
				directiveNames(user.DirectiveList),
			)
		}
	})
}

// directiveNames returns one entry per directive — `+name` for
// positive, `-name` for negated. Used by the directive integration
// test's failure output so the human-readable list of parsed
// directives is visible at-a-glance.
func directiveNames(dl []*directive.Directive) []string {
	out := make([]string, 0, len(dl))
	for _, d := range dl {
		if d.Negated {
			out = append(out, "-"+string(d.Name))
			continue
		}
		out = append(out, "+"+string(d.Name))
	}
	return out
}
