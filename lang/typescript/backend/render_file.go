// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"cmp"
	"slices"
	"strings"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/store"
)

// targetGroup is everything one output file renders from.
type targetGroup struct {
	target emit.Target
	decls  []emit.Node
}

// groupByTarget buckets every renderable entity in the store by the
// file it belongs to.
//
// Declarations are sorted by qualified name within a file. Emit order
// is whatever order the generators ran in, so a file's contents would
// otherwise shuffle when an unrelated plugin was registered — the
// same input producing different bytes, which is the one thing the
// provenance hash exists to make impossible.
func groupByTarget(s *store.Store) []targetGroup {
	byTarget := map[emit.Target][]emit.Node{}
	for _, n := range renderableEntities(s) {
		t := targetOf(n)
		byTarget[t] = append(byTarget[t], n)
	}

	out := make([]targetGroup, 0, len(byTarget))
	for t, decls := range byTarget {
		slices.SortStableFunc(decls, func(a, b emit.Node) int {
			return cmp.Compare(qualifiedName(a), qualifiedName(b))
		})
		out = append(out, targetGroup{target: t, decls: decls})
	}
	// Targets themselves sort too, so the sink sees files in one
	// order however the map iterated.
	slices.SortFunc(out, func(a, b targetGroup) int {
		return cmp.Compare(a.target.JoinPath(), b.target.JoinPath())
	})
	return out
}

// renderableEntities collects every top-level entity the canonical
// template set can render.
func renderableEntities(s *store.Store) []emit.Node {
	var out []emit.Node
	view := s.Emit()

	view.Structs().Range(func(n *emit.Struct) bool { out = append(out, n); return true })
	view.Interfaces().Range(func(n *emit.Interface) bool { out = append(out, n); return true })
	view.Enums().Range(func(n *emit.Enum) bool { out = append(out, n); return true })
	view.Aliases().Range(func(n *emit.Alias) bool { out = append(out, n); return true })
	view.Functions().Range(func(n *emit.Function) bool { out = append(out, n); return true })
	view.Variables().Range(func(n *emit.Variable) bool { out = append(out, n); return true })
	view.Constants().Range(func(n *emit.Constant) bool { out = append(out, n); return true })
	return out
}

// targetOf returns the output file an entity belongs to.
func targetOf(n emit.Node) emit.Target {
	switch t := n.(type) {
	case *emit.Struct:
		return t.Target
	case *emit.Interface:
		return t.Target
	case *emit.Enum:
		return t.Target
	case *emit.Alias:
		return t.File
	case *emit.Function:
		return t.Target
	case *emit.Variable:
		return t.Target
	case *emit.Constant:
		return t.Target
	default:
		return emit.Target{}
	}
}

// qualifiedName returns an entity's sort key.
func qualifiedName(n emit.Node) string {
	type named interface{ QName() string }
	if q, ok := n.(named); ok {
		return q.QName()
	}
	return ""
}

// renderBody renders one target's declarations, then prepends the
// import block the render accumulated.
//
// Imports last, deliberately: which modules a file imports is not
// known until every type in it has been spelled, because spelling a
// type is what registers its import.
func (s *renderState) renderBody(decls []emit.Node) (string, error) {
	var body strings.Builder
	for _, d := range decls {
		if !renderable(d) {
			continue
		}
		got, err := s.render(d)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(got) == "" {
			continue
		}
		if body.Len() > 0 {
			body.WriteString("\n")
		}
		body.WriteString(strings.TrimRight(got, "\n"))
		body.WriteString("\n")
	}

	imports := s.imports.Imports()
	if len(imports) == 0 {
		return body.String(), nil
	}
	return strings.Join(imports, "\n") + "\n\n" + body.String(), nil
}
