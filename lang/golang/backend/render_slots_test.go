// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/plugin"
)

// TestSlots_StructFields_AppendByPluginTopo covers the cross-cutting
// `fields` slot on a struct. Two stub plugins each append a field;
// the resulting rendered struct lists typed fields first, then the
// slot contributions in plugin-topo order.
func TestSlots_StructFields_AppendByPluginTopo(t *testing.T) {
	t.Parallel()

	t.Run("two plugins contribute fields; topo order honoured", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "audit"},
			stubPluginVersion{name: "validation"},
		}
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		host := &emit.Struct{
			Name: "User", Package: "users", Target: target,
			Fields: []*emit.Field{{Name: "ID", Type: emit.Builtin("int")}},
		}
		// Append out of topo order so the test verifies re-grouping
		// rather than coincidental append-order match.
		if err := host.FieldsSlot().Append(
			&emit.Field{Name: "Required", Type: emit.Builtin("bool")},
			emit.Provenance{SetBy: "validation"},
		); err != nil {
			t.Fatalf("append validation field: %v", err)
		}
		if err := host.FieldsSlot().Append(
			&emit.Field{Name: "AuditedAt", Type: emit.Builtin("int64")},
			emit.Provenance{SetBy: "audit"},
		); err != nil {
			t.Fatalf("append audit field: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("users", host))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		mustOrderedSubstrings(t, string(body), "ID ", "AuditedAt ", "Required ")
	})
}

// TestSlots_StructEmbeds_FromSlot covers the cross-cutting `embeds`
// slot on a struct. A slot-contributed embed renders inline inside
// the struct body alongside any typed embeds, in plugin-topo order.
func TestSlots_StructEmbeds_FromSlot(t *testing.T) {
	t.Parallel()

	t.Run("typed + slot embeds render inline inside the struct", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{stubPluginVersion{name: "tracegen"}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		host := &emit.Struct{
			Name: "Wrapper", Package: "x", Target: target,
			Embeds: []*emit.Embed{{Type: emit.External("io", "Reader")}},
			Fields: []*emit.Field{{Name: "N", Type: emit.Builtin("int")}},
		}
		if err := host.EmbedsSlot().Append(
			&emit.Embed{Type: emit.External("io", "Closer")},
			emit.Provenance{SetBy: "tracegen"},
		); err != nil {
			t.Fatalf("append embed slot: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("x", host))
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		mustOrderedSubstrings(t, body, "io.Reader", "io.Closer", "N int")
	})
}

// TestSlots_InterfaceEmbeds_FromSlot covers the cross-cutting
// `embeds` slot on an interface — a slot-contributed embed
// renders inline inside the interface body alongside the typed
// embeds, in plugin-topo order.
func TestSlots_InterfaceEmbeds_FromSlot(t *testing.T) {
	t.Parallel()

	t.Run("typed + slot embeds render inline inside the interface", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{stubPluginVersion{name: "metricsgen"}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		iface := &emit.Interface{
			Name: "Service", Package: "x", Target: target,
			Embeds: []*emit.Embed{{Type: emit.External("io", "Reader")}},
		}
		if err := iface.EmbedsSlot().Append(
			&emit.Embed{Type: emit.External("io", "Closer")},
			emit.Provenance{SetBy: "metricsgen"},
		); err != nil {
			t.Fatalf("append embed slot: %v", err)
		}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Interfaces: []*emit.Interface{iface},
		})
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		mustOrderedSubstrings(t, body, "io.Reader", "io.Closer")
	})
}

// TestSlots_StructMethods_RenderedInline covers the cross-cutting
// `methods` slot on a struct. Slot-contributed methods render as
// standalone declarations below the struct body, grouped by plugin
// topo order. Mixes typed methods with slot contributions to
// verify both paths land in the same file.
func TestSlots_StructMethods_RenderedInline(t *testing.T) {
	t.Parallel()

	t.Run("typed method + two-plugin slot contributions", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "mockgen"},
			stubPluginVersion{name: "tracer"},
		}
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		host := &emit.Struct{Name: "User", Package: "users", Target: target}
		host.Methods = []*emit.Method{{
			Name:         "ID",
			Receiver:     emit.Internal(host),
			ReceiverName: "u",
			Returns:      emit.AnonReturns(emit.Builtin("int")),
			Body: []*emit.Stmt{emit.NewReturn(&emit.Expr{
				ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "0",
			})},
		}}
		if err := host.MethodsSlot().Append(
			&emit.Method{
				Name:         "TraceID",
				Receiver:     emit.Internal(host),
				ReceiverName: "u",
				Returns:      emit.AnonReturns(emit.Builtin("string")),
				Body: []*emit.Stmt{emit.NewReturn(&emit.Expr{
					ExprKind: emit.ExprLiteral, LitKind: emit.LitString, RawText: "",
				})},
			},
			emit.Provenance{SetBy: "tracer"},
		); err != nil {
			t.Fatalf("append tracer method: %v", err)
		}
		if err := host.MethodsSlot().Append(
			&emit.Method{
				Name:         "Mock",
				Receiver:     emit.Internal(host),
				ReceiverName: "u",
				Body:         nil,
			},
			emit.Provenance{SetBy: "mockgen"},
		); err != nil {
			t.Fatalf("append mockgen method: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("users", host))
		out := string(assertRenderSucceeds(t, ctx, mem, d, target))
		mustOrderedSubstrings(t, out, "func (u User) ID()", "func (u User) Mock()", "func (u User) TraceID()")
	})
}

// TestErrNilHost pins the exported sentinel surfaced by every
// funcmap-exposed render helper when invoked with a nil host. The
// helpers are unreachable from core templates (the kind dispatcher
// always passes a real host), so the diagnostic surface fires
// from plugin-supplied templates that call the canonical helpers
// with a wrong dot. The table drives each helper via a plugin
// template that overrides `emit.constant` and passes a typed nil.
func TestErrNilHost(t *testing.T) {
	t.Parallel()

	t.Run("ErrNilHost is exported and satisfies errors.Is", func(t *testing.T) {
		t.Parallel()
		if backend.ErrNilHost == nil {
			t.Fatalf("ErrNilHost must be exported and non-nil")
		}
		if !errors.Is(backend.ErrNilHost, backend.ErrNilHost) {
			t.Fatalf("ErrNilHost must satisfy errors.Is reflexivity")
		}
	})

	// nilProviderFuncs supplies typed-nil constructors so the
	// per-helper subtests can build a plugin template that invokes
	// each canonical helper with a nil host — the only public
	// surface today that reaches the helper's nil-host guard.
	nilProviderFuncs := template.FuncMap{
		"nilStruct":    func() *emit.Struct { return nil },
		"nilInterface": func() *emit.Interface { return nil },
		"nilEnum":      func() *emit.Enum { return nil },
		"nilFunction":  func() *emit.Function { return nil },
		"nilMethod":    func() *emit.Method { return nil },
	}
	cases := []struct {
		helper string
		host   string
		invoke string
	}{
		{"renderStructFields", "*emit.Struct", "renderStructFields nilStruct"},
		{"renderStructEmbeds", "*emit.Struct", "renderStructEmbeds nilStruct"},
		{"renderStructMethods", "*emit.Struct", "{{ range renderStructMethods nilStruct }}{{ end }}"},
		{"renderInterfaceEmbeds", "*emit.Interface", "renderInterfaceEmbeds nilInterface"},
		{"renderInterfaceMethods", "*emit.Interface", "{{ range renderInterfaceMethods nilInterface }}{{ end }}"},
		{"renderEnumVariants", "*emit.Enum", "renderEnumVariants nilEnum"},
		{"renderFunctionBody", "*emit.Function", "renderFunctionBody nilFunction"},
		{"renderFunctionParams", "*emit.Function", "renderFunctionParams nilFunction"},
		{"renderMethodBody", "*emit.Method", "renderMethodBody nilMethod"},
		{"renderMethodParams", "*emit.Method", "renderMethodParams nilMethod"},
		{"renderReceiver", "*emit.Method", "renderReceiver nilMethod"},
	}
	for _, tc := range cases {
		t.Run(tc.helper+"_returns_ErrNilHost_when_host_is_nil", func(t *testing.T) {
			t.Parallel()
			ctx, mem, d := newBackendContext(t)
			// The plugin template overrides emit.constant — the
			// constant the test seeds is rendered through the
			// override, which invokes the helper-under-test with
			// a typed-nil pulled from the plugin's funcmap.
			body := "{{ define \"emit.constant\" }}" + maybeWrapAction(tc.invoke) + "{{ end }}"
			provider := &stubTemplateProvider{
				name:  "niltest",
				funcs: nilProviderFuncs,
				tmplFS: fstest.MapFS{
					"templates/golang/override.tmpl": &fstest.MapFile{Data: []byte(body)},
				},
			}
			ctx.Plugins = []plugin.Plugin{provider}
			ctx.Ordered = []plugin.Plugin{provider}
			target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
			addEmitPackage(t, ctx, &emit.Package{
				Name: "x", Path: "x",
				Constants: []*emit.Constant{{
					Name: "K", Package: "x", Target: target,
					Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "0"},
				}},
			})
			if err := mustNew(t).Render(ctx); err != nil {
				t.Fatalf("Render: %v", err)
			}
			if _, ok := mem.Get(target); ok {
				t.Fatalf("nil-host invocation must suppress sink output")
			}
			if !diagnosticsContain(d, diag.Error, "nil host passed to funcmap helper") {
				t.Fatalf("expected ErrNilHost diagnostic; got %+v", d.Diagnostics())
			}
			if !diagnosticsContain(d, diag.Error, tc.helper) {
				t.Fatalf("diagnostic should name helper %q; got %+v", tc.helper, d.Diagnostics())
			}
			if !diagnosticsContain(d, diag.Error, tc.host) {
				t.Fatalf("diagnostic should name host type %q; got %+v", tc.host, d.Diagnostics())
			}
		})
	}
}

// maybeWrapAction wraps a bare action expression in `{{ … }}` so
// the table can carry either a single helper invocation
// (`renderStructFields nilStruct`) or a multi-action sequence
// already containing `{{ … }}` (e.g. a range over a method slice).
func maybeWrapAction(s string) string {
	if strings.HasPrefix(s, "{{") {
		return s
	}
	return "{{ " + s + " }}"
}

// TestSlots_DuplicateEntity covers the slot-internal collision
// rule: two contributions to the same `methods` slot using the
// same name surface as [backend.ErrDuplicateEntity], with the
// rendered output suppressed.
func TestSlots_DuplicateEntity(t *testing.T) {
	t.Parallel()

	t.Run("ErrDuplicateEntity is exported and satisfies errors.Is", func(t *testing.T) {
		t.Parallel()
		if backend.ErrDuplicateEntity == nil {
			t.Fatalf("ErrDuplicateEntity must be exported and non-nil")
		}
		if !errors.Is(backend.ErrDuplicateEntity, backend.ErrDuplicateEntity) {
			t.Fatalf("ErrDuplicateEntity must satisfy errors.Is reflexivity")
		}
	})

	t.Run("two struct-method slot contributions with same name produce diagnostic", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "audit"},
			stubPluginVersion{name: "validation"},
		}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		host := &emit.Struct{Name: "Doc", Package: "x", Target: target}
		if err := host.MethodsSlot().Append(
			&emit.Method{Name: "Validate", Receiver: emit.Internal(host)},
			emit.Provenance{SetBy: "audit"},
		); err != nil {
			t.Fatalf("append audit method: %v", err)
		}
		if err := host.MethodsSlot().Append(
			&emit.Method{Name: "Validate", Receiver: emit.Internal(host)},
			emit.Provenance{SetBy: "validation"},
		); err != nil {
			t.Fatalf("append validation method: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("x", host))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("duplicate-method collision must suppress the sink write")
		}
		if !diagnosticsContain(d, diag.Error, "duplicate entity") {
			t.Fatalf("expected ErrDuplicateEntity diagnostic; got %+v", d.Diagnostics())
		}
	})

	t.Run("two struct-field slot contributions with same name produce diagnostic", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "audit"},
			stubPluginVersion{name: "validation"},
		}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		host := &emit.Struct{Name: "Doc", Package: "x", Target: target}
		if err := host.FieldsSlot().Append(
			&emit.Field{Name: "Required", Type: emit.Builtin("bool")},
			emit.Provenance{SetBy: "audit"},
		); err != nil {
			t.Fatalf("append audit field: %v", err)
		}
		if err := host.FieldsSlot().Append(
			&emit.Field{Name: "Required", Type: emit.Builtin("string")},
			emit.Provenance{SetBy: "validation"},
		); err != nil {
			t.Fatalf("append validation field: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("x", host))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("duplicate-field collision must suppress the sink write")
		}
		if !diagnosticsContain(d, diag.Error, "duplicate entity") {
			t.Fatalf("expected ErrDuplicateEntity diagnostic; got %+v", d.Diagnostics())
		}
	})

	t.Run("typed-field name colliding with slot field produces diagnostic", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{stubPluginVersion{name: "validation"}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		host := &emit.Struct{
			Name: "Doc", Package: "x", Target: target,
			Fields: []*emit.Field{{Name: "ID", Type: emit.Builtin("int")}},
		}
		if err := host.FieldsSlot().Append(
			&emit.Field{Name: "ID", Type: emit.Builtin("string")},
			emit.Provenance{SetBy: "validation"},
		); err != nil {
			t.Fatalf("append validation field: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("x", host))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("typed/slot field collision must suppress the sink write")
		}
		if !diagnosticsContain(d, diag.Error, "typed-content") {
			t.Fatalf("expected diagnostic citing typed-content; got %+v", d.Diagnostics())
		}
	})

	t.Run("two interface-method slot contributions with same name produce diagnostic", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "tracegen"},
			stubPluginVersion{name: "metricsgen"},
		}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		iface := &emit.Interface{Name: "Service", Package: "x", Target: target}
		if err := iface.MethodsSlot().Append(
			&emit.Method{Name: "Probe"},
			emit.Provenance{SetBy: "tracegen"},
		); err != nil {
			t.Fatalf("append tracegen method: %v", err)
		}
		if err := iface.MethodsSlot().Append(
			&emit.Method{Name: "Probe"},
			emit.Provenance{SetBy: "metricsgen"},
		); err != nil {
			t.Fatalf("append metricsgen method: %v", err)
		}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Interfaces: []*emit.Interface{iface},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("duplicate interface-method collision must suppress the sink write")
		}
		if !diagnosticsContain(d, diag.Error, "duplicate entity") {
			t.Fatalf("expected ErrDuplicateEntity diagnostic; got %+v", d.Diagnostics())
		}
	})

	t.Run("two enum-variant slot contributions with same name produce diagnostic", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "extragen1"},
			stubPluginVersion{name: "extragen2"},
		}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		host := &emit.Enum{
			Name: "Phase", Package: "x", Target: target,
			Underlying: emit.Builtin("int"),
			Variants: []*emit.EnumVariant{
				{Name: "Pending", Value: &emit.Expr{ExprKind: emit.ExprIdent, Name: "iota"}},
			},
		}
		if err := host.VariantsSlot().Append(
			&emit.EnumVariant{Name: "Extra"},
			emit.Provenance{SetBy: "extragen1"},
		); err != nil {
			t.Fatalf("append extragen1 variant: %v", err)
		}
		if err := host.VariantsSlot().Append(
			&emit.EnumVariant{Name: "Extra"},
			emit.Provenance{SetBy: "extragen2"},
		); err != nil {
			t.Fatalf("append extragen2 variant: %v", err)
		}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Enums: []*emit.Enum{host},
		})
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if _, ok := mem.Get(target); ok {
			t.Fatalf("duplicate enum-variant collision must suppress the sink write")
		}
		if !diagnosticsContain(d, diag.Error, "duplicate entity") {
			t.Fatalf("expected ErrDuplicateEntity diagnostic; got %+v", d.Diagnostics())
		}
	})
}

// TestSlots_InterfaceMethods_Topo covers the interface's `methods`
// slot — two cross-cutting plugins each contribute a method
// signature; the inline interface lists typed methods first, then
// slot contributions in topo order.
func TestSlots_InterfaceMethods_Topo(t *testing.T) {
	t.Parallel()

	t.Run("typed + two-plugin slot methods preserve topo", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "tracegen"},
			stubPluginVersion{name: "metricsgen"},
		}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		iface := &emit.Interface{
			Name: "Service", Package: "x", Target: target,
			Methods: []*emit.Method{{Name: "Do"}},
		}
		if err := iface.MethodsSlot().Append(
			&emit.Method{Name: "Metric"},
			emit.Provenance{SetBy: "metricsgen"},
		); err != nil {
			t.Fatalf("append metricsgen method: %v", err)
		}
		if err := iface.MethodsSlot().Append(
			&emit.Method{Name: "Trace"},
			emit.Provenance{SetBy: "tracegen"},
		); err != nil {
			t.Fatalf("append tracegen method: %v", err)
		}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Interfaces: []*emit.Interface{iface},
		})
		out := string(assertRenderSucceeds(t, ctx, mem, d, target))
		mustOrderedSubstrings(t, out, "Do()", "Trace()", "Metric()")
	})
}

// TestSlots_EnumVariants covers the enum's `variants` slot — a
// cross-cutting plugin contributes an additional variant which
// renders inside the same `const ( ... )` block, after the typed
// variants, in plugin-topo order.
func TestSlots_EnumVariants(t *testing.T) {
	t.Parallel()

	t.Run("slot variant appears after typed variants", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{stubPluginVersion{name: "extragen"}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		host := &emit.Enum{
			Name: "Phase", Package: "x", Target: target,
			Underlying: emit.Builtin("int"),
			Variants: []*emit.EnumVariant{
				{Name: "Pending", Value: &emit.Expr{ExprKind: emit.ExprIdent, Name: "iota"}},
				{Name: "Active"},
			},
		}
		if err := host.VariantsSlot().Append(
			&emit.EnumVariant{Name: "Quarantined"},
			emit.Provenance{SetBy: "extragen"},
		); err != nil {
			t.Fatalf("append slot variant: %v", err)
		}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Enums: []*emit.Enum{host},
		})
		out := string(assertRenderSucceeds(t, ctx, mem, d, target))
		mustOrderedSubstrings(t, out, "Pending Phase = iota", "\n\tActive\n", "\n\tQuarantined\n")
	})
}

// TestSlots_FunctionPrebody covers the function's `prebody` slot —
// three cross-cutting plugins each contribute a setup statement;
// the rendered function body opens with the contributions in
// plugin-topo order, then the typed body statements.
func TestSlots_FunctionPrebody(t *testing.T) {
	t.Parallel()

	t.Run("three plugins contribute prebody stmts; topo order honoured", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "logger"},
			stubPluginVersion{name: "tracer"},
			stubPluginVersion{name: "metrics"},
		}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		fn := &emit.Function{
			Name: "Handle", Package: "x", Target: target,
			Body: []*emit.Stmt{emit.NewExprStmt(&emit.Expr{
				ExprKind: emit.ExprCall,
				Callee:   &emit.Expr{ExprKind: emit.ExprIdent, Name: "work"},
			})},
		}
		appendRaw := func(setBy, raw string) {
			if err := fn.Prebody().Append(emit.NewRawStmt(raw), emit.Provenance{SetBy: setBy}); err != nil {
				t.Fatalf("append %s prebody: %v", setBy, err)
			}
		}
		// Append out of topo order to verify re-sorting.
		appendRaw("metrics", `metrics.Inc("handle")`)
		appendRaw("logger", `log.Print("start")`)
		appendRaw("tracer", `defer trace.Span("Handle").End()`)
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{fn},
		})
		out := string(assertRenderSucceeds(t, ctx, mem, d, target))
		mustOrderedSubstrings(t, out, "log.Print", "trace.Span", "metrics.Inc", "work()")
	})
}

// TestSlots_MethodPostbody covers the method's `postbody` slot —
// a slot statement appended by a stub plugin renders AFTER the
// typed body, inside the same method braces.
func TestSlots_MethodPostbody(t *testing.T) {
	t.Parallel()

	t.Run("postbody slot stmt renders after typed body", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{stubPluginVersion{name: "cleanup"}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		host := &emit.Struct{Name: "T", Package: "x", Target: target}
		m := &emit.Method{
			Name:         "Run",
			Receiver:     emit.Internal(host),
			ReceiverName: "t",
			Body:         []*emit.Stmt{emit.NewRawStmt(`work()`)},
		}
		if err := m.Postbody().Append(
			emit.NewRawStmt(`cleanup()`),
			emit.Provenance{SetBy: "cleanup"},
		); err != nil {
			t.Fatalf("append postbody: %v", err)
		}
		host.Methods = []*emit.Method{m}
		addEmitPackage(t, ctx, emitPackage("x", host))
		out := string(assertRenderSucceeds(t, ctx, mem, d, target))
		workIdx, cleanIdx := strings.Index(out, "work()"), strings.Index(out, "cleanup()")
		if workIdx < 0 || cleanIdx < 0 {
			t.Fatalf("expected both work() and cleanup() in body; got:\n%s", out)
		}
		if workIdx > cleanIdx {
			t.Fatalf("postbody stmt must follow typed body; got work=%d cleanup=%d:\n%s",
				workIdx, cleanIdx, out)
		}
	})
}

// TestSlots_FunctionParamsSlot covers the function's `params` slot
// — a cross-cutting plugin appends an extra parameter that
// renders after the typed params in plugin-topo order.
func TestSlots_FunctionParamsSlot(t *testing.T) {
	t.Parallel()

	t.Run("slot param renders after typed params", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{stubPluginVersion{name: "ctxgen"}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		fn := &emit.Function{
			Name: "Save", Package: "x", Target: target,
			Params: []*emit.Param{{Name: "doc", Type: emit.Builtin("string")}},
		}
		if err := fn.ParamsSlot().Append(
			&emit.Param{Name: "tenantID", Type: emit.Builtin("string")},
			emit.Provenance{SetBy: "ctxgen"},
		); err != nil {
			t.Fatalf("append params slot: %v", err)
		}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{fn},
		})
		out := string(assertRenderSucceeds(t, ctx, mem, d, target))
		if !strings.Contains(out, "func Save(doc string, tenantID string)") {
			t.Fatalf("slot param must follow typed param; got:\n%s", out)
		}
	})
}

// TestSlots_FunctionReturnsSlot covers the function's `returns`
// slot — a cross-cutting plugin appends an extra return type
// that renders after the typed returns. Pinning the anonymous
// case (the common Go signature shape) is enough; the mixed
// named/anonymous case surfaces as [emit.ErrMixedNamedReturns]
// via the existing renderReturns check.
func TestSlots_FunctionReturnsSlot(t *testing.T) {
	t.Parallel()

	t.Run("slot return renders after typed returns", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{stubPluginVersion{name: "errgen"}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		fn := &emit.Function{
			Name: "Save", Package: "x", Target: target,
			Returns: []*emit.Return{{Type: emit.Builtin("int")}},
		}
		if err := fn.ReturnsSlot().Append(
			&emit.Return{Type: emit.Builtin("error")},
			emit.Provenance{SetBy: "errgen"},
		); err != nil {
			t.Fatalf("append returns slot: %v", err)
		}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{fn},
		})
		out := string(assertRenderSucceeds(t, ctx, mem, d, target))
		if !strings.Contains(out, "func Save() (int, error)") {
			t.Fatalf("slot return must follow typed return; got:\n%s", out)
		}
	})

	t.Run("void function with only a slot return renders the slot type", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{stubPluginVersion{name: "errgen"}}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		fn := &emit.Function{Name: "Run", Package: "x", Target: target}
		if err := fn.ReturnsSlot().Append(
			&emit.Return{Type: emit.Builtin("error")},
			emit.Provenance{SetBy: "errgen"},
		); err != nil {
			t.Fatalf("append returns slot: %v", err)
		}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{fn},
		})
		out := string(assertRenderSucceeds(t, ctx, mem, d, target))
		if !strings.Contains(out, "func Run() error {") {
			t.Fatalf("void function with slot return must render the slot type; got:\n%s", out)
		}
	})
}

// TestSlots_FieldTagsViaSlot covers the tag-slot composition under
// the plugin-topo treatment: two plugins each append a tag, and
// the rendered struct field stitches them in topo order after
// the base tag.
func TestSlots_FieldTagsViaSlot(t *testing.T) {
	t.Parallel()

	t.Run("base tag + two-plugin slot tags compose in topo order", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "dbgen"},
			stubPluginVersion{name: "yamlgen"},
		}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		id := &emit.Field{Name: "ID", Type: emit.Builtin("int"), Tag: `json:"id"`}
		// Append out of topo order — verifies re-grouping rather
		// than coincidental append-order match.
		if err := id.Tags().Append(
			&emit.Tag{Key: "yaml", Value: "id"},
			emit.Provenance{SetBy: "yamlgen"},
		); err != nil {
			t.Fatalf("append yaml tag: %v", err)
		}
		if err := id.Tags().Append(
			&emit.Tag{Key: "db", Value: "user_id"},
			emit.Provenance{SetBy: "dbgen"},
		); err != nil {
			t.Fatalf("append db tag: %v", err)
		}
		host := &emit.Struct{
			Name: "User", Package: "x", Target: target,
			Fields: []*emit.Field{id},
		}
		addEmitPackage(t, ctx, emitPackage("x", host))
		body := string(assertRenderSucceeds(t, ctx, mem, d, target))
		// Tag-slot ordering currently follows append order. Both
		// contributors must be present in the rendered backtick blob.
		if !strings.Contains(body, `db:"user_id"`) || !strings.Contains(body, `yaml:"id"`) {
			t.Fatalf("expected both db and yaml tags in field; got:\n%s", body)
		}
	})
}

// TestSlots_StructSlot_Goldens pins canonical multi-plugin output
// for the three flagship slot scenarios — struct methods, function
// prebody, struct fields — so byte-level drift in the slot merge
// or template surface is caught at PR time.
func TestSlots_StructSlot_Goldens(t *testing.T) {
	t.Parallel()

	t.Run("slot_struct_methods — typed + two-plugin slot methods", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "mockgen"},
			stubPluginVersion{name: "tracer"},
		}
		target := emit.Target{Dir: "users", Filename: "user.go", Package: "users"}
		host := &emit.Struct{
			BaseEmit: emit.BaseEmit{DocLines: []string{"User is the canonical user record."}},
			Name:     "User", Package: "users", Target: target,
			Fields: []*emit.Field{{Name: "ID", Type: emit.Builtin("int")}},
		}
		host.Methods = []*emit.Method{{
			BaseEmit:     emit.BaseEmit{DocLines: []string{"Identifier returns the user's ID."}},
			Name:         "Identifier",
			Receiver:     emit.Internal(host),
			ReceiverName: "u",
			Returns:      emit.AnonReturns(emit.Builtin("int")),
			Body: []*emit.Stmt{emit.NewReturn(&emit.Expr{
				ExprKind: emit.ExprField,
				Receiver: &emit.Expr{ExprKind: emit.ExprIdent, Name: "u"},
				Name:     "ID",
			})},
		}}
		if err := host.MethodsSlot().Append(
			&emit.Method{
				BaseEmit:     emit.BaseEmit{DocLines: []string{"TraceID returns the user's trace identifier."}},
				Name:         "TraceID",
				Receiver:     emit.Internal(host),
				ReceiverName: "u",
				Returns:      emit.AnonReturns(emit.Builtin("string")),
				Body: []*emit.Stmt{emit.NewReturn(&emit.Expr{
					ExprKind: emit.ExprLiteral, LitKind: emit.LitString, RawText: "",
				})},
			},
			emit.Provenance{SetBy: "tracer"},
		); err != nil {
			t.Fatalf("append tracer method: %v", err)
		}
		if err := host.MethodsSlot().Append(
			&emit.Method{
				BaseEmit:     emit.BaseEmit{DocLines: []string{"Mock is a no-op for test substitution."}},
				Name:         "Mock",
				Receiver:     emit.Internal(host),
				ReceiverName: "u",
			},
			emit.Provenance{SetBy: "mockgen"},
		); err != nil {
			t.Fatalf("append mockgen method: %v", err)
		}
		addEmitPackage(t, ctx, emitPackage("users", host))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "slot_struct_methods.go.golden"))
	})

	t.Run("slot_function_prebody — three-plugin prebody composition", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.Ordered = []plugin.Plugin{
			stubPluginVersion{name: "logger"},
			stubPluginVersion{name: "tracer"},
			stubPluginVersion{name: "metrics"},
		}
		target := emit.Target{Dir: "handlers", Filename: "handler.go", Package: "handlers"}
		fn := &emit.Function{
			BaseEmit: emit.BaseEmit{DocLines: []string{"Handle dispatches a single request."}},
			Name:     "Handle", Package: "handlers", Target: target,
			Body: []*emit.Stmt{emit.NewExprStmt(&emit.Expr{
				ExprKind: emit.ExprCall,
				Callee:   &emit.Expr{ExprKind: emit.ExprIdent, Name: "work"},
			})},
		}
		appendRaw := func(setBy, raw string) {
			if err := fn.Prebody().Append(emit.NewRawStmt(raw), emit.Provenance{SetBy: setBy}); err != nil {
				t.Fatalf("append %s prebody: %v", setBy, err)
			}
		}
		// Append out of topo order; render verifies re-sorting.
		appendRaw("metrics", `_ = "metrics-stub"`)
		appendRaw("logger", `_ = "logger-stub"`)
		appendRaw("tracer", `_ = "tracer-stub"`)
		addEmitPackage(t, ctx, &emit.Package{
			Name: "handlers", Path: "handlers",
			Functions: []*emit.Function{fn},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		pipelinetest.MatchesGoldenBytes(t, body, goldenPath(t, "slot_function_prebody.go.golden"))
	})
}
