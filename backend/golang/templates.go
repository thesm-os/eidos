// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"embed"
	"errors"
	"maps"
	"text/template"

	"go.thesmos.sh/eidos/emit"
	langgo "go.thesmos.sh/eidos/lang/golang"
)

// templatesFS embeds the backend's core template files. Plugin-
// supplied templates are merged into the same set at render time
// via [plugin.TemplateProvider.Templates].
//
//go:embed templates/*.tmpl
var templatesFS embed.FS

// loadTemplates parses every core template file from [templatesFS]
// into a fresh template set and returns the root. Parse-time
// signature placeholders are registered for every funcmap entry
// the templates reference; each per-Target [renderState] clones
// this root and overrides the placeholders with closures binding
// the correct [writer.ImportSet] for that Target.
//
// Panics on parse failure: the templates are embedded at compile
// time, so a parse error indicates a developer-side syntax bug in
// the templates rather than a runtime condition.
func loadTemplates() *template.Template {
	return template.Must(
		template.New("eidos.golang").
			Funcs(parsePlaceholders).
			ParseFS(templatesFS, "templates/*.tmpl"),
	)
}

// parsePlaceholders satisfies the parser's name-and-arity check
// for funcmap entries referenced by core templates. The bodies
// return [errPlaceholderInvoked] — never executed in practice
// because every Target render clones the root and overrides them
// via [renderState.funcMap] before any ExecuteTemplate call. The
// placeholders match the signatures of the real implementations.
//
//nolint:gochecknoglobals // parse-time signature table; immutable.
var parsePlaceholders = func() map[string]any {
	out := map[string]any{
		"render":                 func(emit.Node) (string, error) { return "", errPlaceholderInvoked },
		"renderType":             func(emit.Ref) (string, error) { return "", errPlaceholderInvoked },
		"renderDocs":             func([]string) string { return "" },
		"renderTypeParams":       func([]*emit.TypeParam) (string, error) { return "", errPlaceholderInvoked },
		"renderParams":           func([]*emit.Param) (string, error) { return "", errPlaceholderInvoked },
		"renderReceiver":         func(*emit.Method) (string, error) { return "", errPlaceholderInvoked },
		"renderReturns":          func([]*emit.Return) (string, error) { return "", errPlaceholderInvoked },
		"renderExpr":             func(*emit.Expr) (string, error) { return "", errPlaceholderInvoked },
		"renderSample":           func(langgo.Sample) (string, error) { return "", errPlaceholderInvoked },
		"renderStmt":             func(*emit.Stmt) (string, error) { return "", errPlaceholderInvoked },
		"renderStructFields":     func(*emit.Struct) (string, error) { return "", errPlaceholderInvoked },
		"renderStructEmbeds":     func(*emit.Struct) (string, error) { return "", errPlaceholderInvoked },
		"renderStructMethods":    func(*emit.Struct) ([]*emit.Method, error) { return nil, errPlaceholderInvoked },
		"renderInterfaceEmbeds":  func(*emit.Interface) (string, error) { return "", errPlaceholderInvoked },
		"renderInterfaceMethods": func(*emit.Interface) ([]*emit.Method, error) { return nil, errPlaceholderInvoked },
		"renderEnumVariants":     func(*emit.Enum) (string, error) { return "", errPlaceholderInvoked },
		"renderFunctionBody":     func(*emit.Function) (string, error) { return "", errPlaceholderInvoked },
		"renderMethodBody":       func(*emit.Method) (string, error) { return "", errPlaceholderInvoked },
		"renderFunctionParams":   func(*emit.Function) (string, error) { return "", errPlaceholderInvoked },
		"renderMethodParams":     func(*emit.Method) (string, error) { return "", errPlaceholderInvoked },
		"renderFunctionReturns":  func(*emit.Function) (string, error) { return "", errPlaceholderInvoked },
		"renderMethodReturns":    func(*emit.Method) (string, error) { return "", errPlaceholderInvoked },
		"imp":                    func(string) (string, error) { return "", errPlaceholderInvoked },
		"external":               func(string, string) *emit.Expr { return nil },
		"slot":                   func(emit.Node, string) (*emit.Slot, error) { return nil, errPlaceholderInvoked },
		"provenance":             func(emit.Node) string { return "" },
	}
	// Layer the overrideable entries (Naming/Meta/String/Debug)
	// directly — their real implementations are stateless, so
	// using them as parse-time placeholders is harmless.
	maps.Copy(out, extrasFuncMap())
	return out
}()

// errPlaceholderInvoked surfaces if a placeholder closure is ever
// reached at execute time. Indicates a backend bug — execution
// without a per-Target clone of the template set.
var errPlaceholderInvoked = errors.New("golang: template func placeholder invoked; clone before execute")
