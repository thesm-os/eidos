// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest

import (
	"fmt"
	"go/ast"
	"os"
	"slices"
	"strings"
	"testing"
)

// API renders the file's exported surface: every exported
// declaration and its shape, sorted, without bodies, comments or
// formatting.
//
// # Why this exists beside a byte golden
//
// A byte golden answers "what does this file read like" and churns on
// every comment reflow, so a template change produces two hundred
// changed lines and the review becomes a formality. A substring
// assertion answers "is this one construct present" and says nothing
// about the two hundred lines around it.
//
// What a consumer actually depends on is the exported surface, and a
// golden over that changes only when their code would have to. Adding
// a method is one added line; renaming a parameter is nothing at all;
// changing a signature is one line replaced. That is a diff a
// reviewer reads rather than scrolls.
//
// Deliberately excluded: unexported declarations, doc comments,
// bodies, and field order within a struct — none of which a consumer
// in another package can observe. Struct fields and interface methods
// are sorted so a generator that reorders them without changing what
// it offers produces no diff.
func (s *Source) API() string {
	var lines []string
	for _, d := range s.file.Decls {
		lines = append(lines, apiLines(d)...)
	}
	slices.Sort(lines)
	lines = slices.Compact(lines)
	return "package " + s.file.Name.Name + "\n\n" + strings.Join(lines, "\n") + "\n"
}

// apiLines renders one declaration's exported surface.
func apiLines(d ast.Decl) []string {
	switch decl := d.(type) {
	case *ast.FuncDecl:
		return funcAPI(decl)
	case *ast.GenDecl:
		var out []string
		for _, spec := range decl.Specs {
			out = append(out, specAPI(spec)...)
		}
		return out
	default:
		return nil
	}
}

// funcAPI renders an exported function or method.
//
// A method on an unexported receiver is dropped: a consumer cannot
// name the type, so the method is not surface however it is spelled.
func funcAPI(fn *ast.FuncDecl) []string {
	if !ast.IsExported(fn.Name.Name) {
		return nil
	}
	sig := normalise(strings.TrimPrefix(render(fn.Type), "func"))
	if recv := receiverName(fn); recv != "" {
		if !ast.IsExported(recv) {
			return nil
		}
		return []string{fmt.Sprintf("func (%s%s) %s%s", pointerMark(fn), recv, fn.Name.Name, sig)}
	}
	return []string{"func " + fn.Name.Name + sig}
}

// pointerMark spells a method's receiver form, which is part of the
// surface: a value receiver puts the method on both forms and a
// pointer receiver on one.
func pointerMark(fn *ast.FuncDecl) string {
	if _, ptr := fn.Recv.List[0].Type.(*ast.StarExpr); ptr {
		return "*"
	}
	return ""
}

// specAPI renders an exported type, constant or variable.
func specAPI(spec ast.Spec) []string {
	switch sp := spec.(type) {
	case *ast.TypeSpec:
		if !ast.IsExported(sp.Name.Name) {
			return nil
		}
		return []string{"type " + sp.Name.Name + typeParams(sp) + " " + typeAPI(sp.Type)}
	case *ast.ValueSpec:
		var out []string
		for _, n := range sp.Names {
			if ast.IsExported(n.Name) {
				out = append(out, "var "+n.Name+valueType(sp))
			}
		}
		return out
	default:
		return nil
	}
}

// typeParams renders a generic type's parameter list, which is part
// of the surface because a consumer has to instantiate it.
//
// Composed field by field rather than printed: [printer.Fprint]
// renders a bare [ast.FieldList] as nothing, so a generic type would
// otherwise present the same surface as its non-generic namesake.
func typeParams(sp *ast.TypeSpec) string {
	if sp.TypeParams == nil || len(sp.TypeParams.List) == 0 {
		return ""
	}
	var parts []string
	for _, f := range sp.TypeParams.List {
		names := make([]string, 0, len(f.Names))
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
		parts = append(parts, strings.Join(names, ", ")+" "+exprString(f.Type))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// valueType renders a var's declared type, empty when it is inferred.
func valueType(sp *ast.ValueSpec) string {
	if sp.Type == nil {
		return ""
	}
	return " " + exprString(sp.Type)
}

// typeAPI renders a type's shape, expanding a struct or interface to
// its exported members and collapsing everything else to its
// canonical spelling.
func typeAPI(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StructType:
		return "struct{" + strings.Join(memberAPI(t.Fields, false), "; ") + "}"
	case *ast.InterfaceType:
		return "interface{" + strings.Join(memberAPI(t.Methods, true), "; ") + "}"
	default:
		return exprString(e)
	}
}

// memberAPI renders a struct's exported fields or an interface's
// method set, sorted.
//
// Sorted rather than in source order because a consumer naming a
// field by name cannot observe its position — only a struct literal
// without field names could, and generated code that produced one
// would break on any field addition anyway.
func memberAPI(fields *ast.FieldList, iface bool) []string {
	var out []string
	for _, f := range fields.List {
		rendered := exprString(f.Type)
		if len(f.Names) == 0 {
			// An embedded type in either position, or an interface's
			// embedded constraint: exported surface, since it carries
			// members through.
			out = append(out, rendered)
			continue
		}
		for _, n := range f.Names {
			if !ast.IsExported(n.Name) {
				continue
			}
			if iface {
				out = append(out, n.Name+normalise(strings.TrimPrefix(rendered, "func")))
				continue
			}
			out = append(out, n.Name+" "+rendered)
		}
	}
	slices.Sort(out)
	return out
}

// AssertAPIGolden compares the file's exported surface against a
// golden, writing it when the golden is absent.
//
// The review surface: a diff here is a list of what consumers would
// have to change, which is what a reviewer needs and what a byte
// golden buries. Keep both if you want a record of what the file
// reads like — they answer different questions — but this is the one
// worth reading.
//
// A missing golden is written rather than failed, so the first run
// after adding a fixture records the surface instead of demanding the
// author type it out. An existing one is never rewritten: regenerate
// by deleting it, which keeps the change visible in review.
func AssertAPIGolden(tb testing.TB, s *Source, goldenPath string) *Source {
	tb.Helper()
	got := s.API()

	want, err := os.ReadFile(goldenPath) //nolint:gosec // a test-supplied fixture path.
	if os.IsNotExist(err) {
		writeGolden(tb, goldenPath, []byte(got))
		return s
	}
	if err != nil {
		tb.Fatalf("golangtest: read %s: %v", goldenPath, err)
	}
	if string(want) != got {
		tb.Errorf("golangtest: %s exported surface changed; every line here is one a "+
			"consumer would have to react to\n--- %s ---\n%s\n--- got ---\n%s",
			s.path, goldenPath, want, got)
	}
	return s
}
