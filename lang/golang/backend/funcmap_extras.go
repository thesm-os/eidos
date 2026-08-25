// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"fmt"
	"maps"
	"reflect"
	"strings"
	"unicode"

	"go.thesmos.sh/eidos/core/naming"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang"
)

// extrasFuncMap returns the overrideable funcmap categories — the
// Naming, Meta-read, String, and Debug helpers — that plugin
// templates may call directly and plugin authors may override
// through [plugin.TemplateProvider.TemplateOverrides]. Reserved
// canonical entries (dispatch, slot composition, `slot`,
// `provenance`, `imp`) are wired separately and may not be
// overridden.
func extrasFuncMap() map[string]any {
	out := map[string]any{
		// Naming — delegates to core/naming.
		"pascal":    naming.Pascal,
		"camel":     naming.Camel,
		"snake":     naming.Snake,
		"screaming": naming.ScreamingSnake,
		"exported":  exported,

		// Meta read — Bag-backed accessors.
		"meta":     metaValue,
		"metaBool": metaBool,
		"metaStr":  metaStr,
		"hasMeta":  hasMeta,
		"metaEq":   metaEq,

		// String — utility wrappers over strings.* with template-
		// friendly signatures.
		"join":     strJoin,
		"title":    naming.Title,
		"upper":    strings.ToUpper,
		"lower":    strings.ToLower,
		"trim":     strings.TrimSpace,
		"split":    strSplit,
		"default":  defaultValue,
		"coalesce": coalesce,

		// Debug — origin/explain helpers for plugin template
		// development. Cheap to call; intended for "where did
		// this come from?" diagnostic output.
		"origin":  origin,
		"explain": explain,
	}
	// Layer the whole shared Go vocabulary ([golang.AllFuncMap]) on
	// top so every plugin template targeting the Go backend can call
	// it without registering anything. One copy, registered here:
	// handing each plugin its own meant two plugins reaching for
	// `promotedMethods` registered the same helper twice and the
	// backend rejected the pair, which is what the per-plugin name
	// prefix existed to hide. Living under [extrasFuncMap] keeps them
	// overrideable — projects that need bridge-aware variants
	// (proto → Go `go.name` substitution, …) can supply replacements
	// through [plugin.TemplateProvider.TemplateOverrides].
	maps.Copy(out, map[string]any(golang.FuncMap()))
	maps.Copy(out, map[string]any(golang.AllFuncMap()))
	return out
}

// exported returns s with the first rune title-cased — the
// canonical "make this identifier exported in Go" transformation.
// Empty input returns the empty string; an already-exported
// identifier passes through unchanged.
func exported(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// metaValue returns the winning value at name on host's metadata
// bag, or nil when the key is unset or tombstoned. Templates that
// only need the raw value bypass the typed accessors:
//
//	{{ if hasMeta . "validate" }}validate enabled{{ end }}
func metaValue(host emit.Node, name string) any {
	if host == nil {
		return nil
	}
	v, ok := host.Meta().RawValue(name)
	if !ok {
		return nil
	}
	return v
}

// metaBool returns the bool value at name on host's metadata bag,
// or false when the key is unset or holds a non-bool value.
func metaBool(host emit.Node, name string) bool {
	if v, ok := metaValue(host, name).(bool); ok {
		return v
	}
	return false
}

// metaStr returns the string value at name on host's metadata bag,
// or the empty string when the key is unset or holds a non-string
// value.
func metaStr(host emit.Node, name string) string {
	if v, ok := metaValue(host, name).(string); ok {
		return v
	}
	return ""
}

// hasMeta reports whether host's metadata bag carries a live entry
// at name (set and not tombstoned).
func hasMeta(host emit.Node, name string) bool {
	if host == nil {
		return false
	}
	return host.Meta().Has(name)
}

// metaEq reports whether host's metadata bag carries name with a
// value equal to want (via [reflect.DeepEqual]). Useful for
// template conditionals: `{{ if metaEq . "kind" "primary" }}`.
func metaEq(host emit.Node, name string, want any) bool {
	got := metaValue(host, name)
	return reflect.DeepEqual(got, want)
}

// strJoin is the template-friendly wrapper around [strings.Join].
// Argument order matches the template idiom (`{{ join names ", " }}`)
// — slice first, then separator.
func strJoin(parts []string, sep string) string {
	return strings.Join(parts, sep)
}

// strSplit is the template-friendly wrapper around [strings.Split].
// Argument order matches the template idiom (`{{ split path "/" }}`).
func strSplit(s, sep string) []string {
	return strings.Split(s, sep)
}

// defaultValue returns value when it is non-zero / non-empty,
// otherwise fallback. The "zero" determination uses
// [reflect.Value.IsZero] — nil interfaces, empty strings, zero
// numerics, and zero structs all fall through to fallback.
func defaultValue(value, fallback any) any {
	if isZero(value) {
		return fallback
	}
	return value
}

// coalesce returns the first non-zero argument, or nil when every
// argument is zero. Multi-fallback variant of [defaultValue].
func coalesce(values ...any) any {
	for _, v := range values {
		if !isZero(v) {
			return v
		}
	}
	return nil
}

// isZero reports whether v reflects to a zero value (nil interface,
// empty string, zero numeric, …). Used by [defaultValue] and
// [coalesce] for the "fall through to fallback" decision.
func isZero(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return !rv.IsValid() || rv.IsZero()
}

// origin returns the "file:line" origin attribution for an emit
// value, or the empty string when no source-file path is
// available. Distinct from [provenance] in that origin produces
// only the position datum suitable for direct embedding in
// generated comments (`// origin: pkg/types.go:42`).
func origin(n emit.Node) string {
	if n == nil {
		return ""
	}
	node := n.Origin()
	if node == nil {
		return ""
	}
	pos := node.Pos()
	if pos.File == "" {
		return ""
	}
	if pos.Line > 0 {
		return fmt.Sprintf("%s:%d", pos.File, pos.Line)
	}
	return pos.File
}

// explain returns a human-readable multi-field summary of an
// emit value — kind + origin + directive count + meta-key count
// — for template-time debugging. The output is intentionally
// stable across runs (no timestamps) so it can be embedded in
// generated comments during plugin development without breaking
// byte-stability.
func explain(n emit.Node) string {
	if n == nil {
		return "(nil)"
	}
	parts := []string{string(n.Kind())}
	if o := origin(n); o != "" {
		parts = append(parts, "from="+o)
	}
	if dirs := n.Directives(); len(dirs) > 0 {
		parts = append(parts, fmt.Sprintf("directives=%d", len(dirs)))
	}
	if names := n.Meta().Names(); len(names) > 0 {
		parts = append(parts, fmt.Sprintf("meta=%d", len(names)))
	}
	return strings.Join(parts, " ")
}
