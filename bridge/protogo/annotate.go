// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protogo

import (
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/naming"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/frontend/protobuf"
	"go.thesmos.sh/eidos/node"
)

// annotateStruct stamps per-field [MetaGoName] (PascalCase
// identifier) and routes each field's TypeRef through the
// field-aware stamping path so the optional-wrap rule (read off
// the field's `proto.field.optional` meta) lands correctly.
func annotateStruct(s *node.Struct) {
	for _, f := range s.Fields {
		stampFieldName(f)
		if f.Type != nil {
			stampFieldType(f)
		}
	}
}

// annotateInterface walks each method's params and returns and
// stamps the Go-side type meta on every reachable TypeRef. RPC
// param/return types don't carry the proto3-optional marker, so
// the field-aware stampFieldType isn't needed here; the simpler
// stampTypeRef path composes through the table + composite
// rules without the optional-wrap layer.
func annotateInterface(iface *node.Interface) {
	for _, m := range iface.Methods {
		for _, p := range m.Params {
			if p.Type != nil {
				stampTypeRef(p.Type)
			}
		}
		for _, r := range m.Returns {
			stampTypeRef(r.Type)
		}
	}
}

// stampFieldName records the Go-idiomatic PascalCase identifier
// for f under [MetaGoName]. Pre-stamped go.name meta wins —
// users who set go.name on a proto field by hand keep their
// override.
func stampFieldName(f *node.Field) {
	if _, ok := MetaGoName.Get(f.Meta()); ok {
		return
	}
	MetaGoName.Set(f.EnsureMeta(), naming.Pascal(f.Name), Name)
}

// stampFieldType records [MetaGoType] on f.Type using the
// composite composition rule plus the optional-wrap rule. The
// field is the iteration unit because optional-wrap requires
// reading `proto.field.optional` off the field's own meta bag,
// which the TypeRef itself doesn't carry. Pre-stamped go.type
// wins per the bridge idempotency contract.
//
// Well-known references additionally stamp the Go import path
// on the TypeRef so the render-site can register it on the
// host file's ImportSet without re-parsing the rendered type
// string.
func stampFieldType(f *node.Field) {
	if _, ok := MetaGoType.Get(f.Type.Meta()); ok {
		return
	}
	inner := composeGoType(f.Type)
	if inner == "" {
		return
	}
	if optional, _ := protobuf.MetaFieldOptional.Get(f.Meta()); optional {
		inner = "*" + inner
	}
	MetaGoType.Set(f.Type.EnsureMeta(), inner, Name)
	stampWellKnownImport(f.Type)
}

// stampTypeRef records [MetaGoType] on r using the composite
// composition rule: scalars and well-knowns flow through the
// translation tables; slice / map wrap the recursed element
// form; pointer wraps with `*`. Named message / enum refs aren't
// stamped — the backend's emit.External path renders the
// cross-package qualifier through the ImportSet. Pre-stamped
// go.type wins.
func stampTypeRef(r *node.TypeRef) {
	if _, ok := MetaGoType.Get(r.Meta()); ok {
		return
	}
	got := composeGoType(r)
	if got == "" {
		return
	}
	MetaGoType.Set(r.EnsureMeta(), got, Name)
	stampWellKnownImport(r)
}

// stampWellKnownImport records the canonical Go import path
// alongside [MetaGoType] when r references a well-known type.
// The render-site consults this key to register the import on
// the host file's ImportSet without parsing the rendered type
// string.
func stampWellKnownImport(r *node.TypeRef) {
	wk, ok := protobuf.MetaWellKnown.Get(r.Meta())
	if !ok {
		return
	}
	if path := wellKnownImport(wk); path != "" {
		MetaGoImport.Set(r.EnsureMeta(), path, Name)
	}
}

// composeGoType returns the Go-side rendered type expression
// for r, recursing into composite element types. Empty return
// means "no translation available" — the caller skips the stamp
// so the render-site falls back to TypeRef.Name verbatim.
func composeGoType(r *node.TypeRef) string {
	if r == nil {
		return ""
	}
	switch {
	case r.IsSlice():
		if elem := composeGoType(r.Elem); elem != "" {
			return "[]" + elem
		}
		return ""
	case r.IsPointer():
		if elem := composeGoType(r.Elem); elem != "" {
			return "*" + elem
		}
		return ""
	case r.IsMap():
		k := composeGoType(r.MapKey)
		v := composeGoType(r.MapValue)
		if k == "" || v == "" {
			return ""
		}
		return "map[" + k + "]" + v
	}
	if scalar := scalarGoType(r.Name); scalar != "" {
		return scalar
	}
	if wellKnown, ok := protobuf.MetaWellKnown.Get(r.Meta()); ok {
		if got := wellKnownGoType(wellKnown); got != "" {
			return got
		}
	}
	// Named message / enum refs are *not* stamped — the backend's
	// emit.External / ImportSet path already renders the
	// cross-package qualifier, and the dot-joined proto name
	// (Outer.Inner for nested messages) is not a valid Go
	// identifier. Letting the default rendering take over is
	// correct here; the empty return tells stampTypeRef to
	// skip the stamp.
	return ""
}

// stampPackageMeta records [MetaGoImport] and [MetaGoName] on
// pkg. The import path comes from the proto `go_package` option
// (the `proto.option.go_package` meta the frontend already
// stamps); the package clause derives from the same value's
// semicolon-suffix or last `/` segment. Falls back to the
// last dotted segment of the proto package qualifier when no
// option was set.
//
// Pre-stamped go.import / go.name win over the derived values
// per the bridge idempotency contract.
//
// A package missing the proto `go_package` option produces a
// positioned diag.Warn through ps so users see why their
// rendered Go output didn't pick up cross-package imports.
func stampPackageMeta(pkg *node.Package, ps *diag.PluginSink) {
	goPackageOption := meta.EnsureKey(protobuf.MetaOptionPrefix+"go_package", meta.StringParser)
	rawOption, optionPresent := goPackageOption.Get(pkg.Meta())
	if !optionPresent && ps != nil {
		ps.Warnf(
			position.Pos{File: firstFilePath(pkg)},
			"protogo: proto package %q has no `option go_package = ...;`; the rendered Go output will use the last dotted segment as the package clause and carry no import path for cross-package references",
			pkg.Path,
		)
	}
	if _, ok := MetaGoImport.Get(pkg.Meta()); !ok {
		if path := GoImportPath(rawOption); path != "" {
			MetaGoImport.Set(pkg.EnsureMeta(), path, Name)
		}
	}
	if _, ok := MetaGoName.Get(pkg.Meta()); !ok {
		name := GoPackageName(rawOption)
		if name == "" {
			name = pkg.Name
		}
		if name != "" {
			MetaGoName.Set(pkg.EnsureMeta(), name, Name)
		}
	}
}

// firstFilePath returns the path of the first source file
// contributing to pkg, or the package's path qualifier when no
// files are recorded. Used as the diagnostic-anchor position for
// the missing-go_package warn.
func firstFilePath(pkg *node.Package) string {
	if len(pkg.Files) > 0 {
		return pkg.Files[0].Path
	}
	return pkg.Path
}
