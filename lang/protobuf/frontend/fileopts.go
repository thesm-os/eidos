// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"fmt"

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// stampFileOptions walks fd's file-level options and stamps each
// set option on pkg's meta bag under the documented
// `proto.option.<full-name>` channel. Standard options (defined
// on google.protobuf.FileOptions) stamp under their short name —
// `proto.option.go_package`, `proto.option.deprecated`. Custom
// extension options stamp under the extension's proto FQN —
// `proto.option.acme.cfg.feature_flag`. Value types follow the
// documented value-type mapping.
//
// File-level options accumulate across multiple .proto files
// sharing one proto package: later files' options merge into the
// already-stamped package bag. The merge is order-deterministic
// because [convertFiles] sorts descriptors alphabetically by file
// path before invoking this helper. Same-key cross-file
// collisions on string- or bool-typed options surface as a Warn
// so users see the silent last-writer-wins behaviour rather than
// inheriting one file's value at random.
//
// Non-file hosts (message, field, oneof, enum, enum-variant,
// service, method) route through [stampHostOptions] instead —
// each descriptor owns its options at one site, so the
// cross-source overwrite-warn is file-specific. Both paths
// converge on [stampOptionOnBag] for the per-option stamping
// itself, so the value-type table and meta-key namespace are
// shared.
func stampFileOptions(ps *diag.PluginSink, pkg *node.Package, fd protoreflect.FileDescriptor) {
	opts := fd.Options()
	if opts == nil {
		return
	}
	pos := position.Pos{File: fd.Path()}
	opts.ProtoReflect().Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		stampFileOption(ps, pkg, fd, field, value, pos)
		return true
	})
}

// stampFileOption stamps one file-level option pair with the
// cross-file overwrite-warn guard layered on top of the generic
// path. Detects collisions for the common scalar forms (string,
// bool); other forms route through the same generic path
// without an overwrite check (the file-level collision risk is
// effectively scalar-shaped — every well-known FileOptions field
// is either a string, bool, or enum).
func stampFileOption(
	ps *diag.PluginSink, pkg *node.Package, fd protoreflect.FileDescriptor,
	field protoreflect.FieldDescriptor, value protoreflect.Value, pos position.Pos,
) {
	name := optionKeyName(field)
	switch field.Kind() { //nolint:exhaustive // overwrite warn covers scalar-shaped FileOptions kinds only
	case protoreflect.StringKind:
		warnIfFileOptionOverwrites(
			ps, pkg, name, value.String(), fd.Path(), pos, "%q",
		)
	case protoreflect.BoolKind:
		warnIfFileOptionOverwrites(
			ps, pkg, name, value.Bool(), fd.Path(), pos, "%t",
		)
	}
	stampOptionOnBag(ps, pkg.EnsureMeta(), field, value, pos)
}

// warnIfFileOptionOverwrites detects a same-key cross-file
// overwrite on a scalar option and surfaces a positioned Warn
// describing the prior/new pair. The generic format string lets
// the same helper handle string-quoted and bool-natural rendering
// without per-type duplication.
func warnIfFileOptionOverwrites[T comparable](
	ps *diag.PluginSink, pkg *node.Package,
	name string, value T, file string, pos position.Pos, valueFormat string,
) {
	key, ok := lookupScalarOptionKey[T](name)
	if !ok {
		return
	}
	prior, ok := key.Get(pkg.Meta())
	if !ok || prior == value {
		return
	}
	ps.Warnf(
		pos,
		fmt.Sprintf(
			"frontend: file option %%s = %s on %%s overwrites prior value %s from sibling file",
			valueFormat, valueFormat,
		),
		name, value, file, prior,
	)
}

// lookupScalarOptionKey returns the typed [meta.Key] for the
// option name under the [MetaOptionPrefix] namespace, dispatching
// the parser by T. Returns false for value types the file-level
// overwrite path doesn't track.
func lookupScalarOptionKey[T comparable](name string) (meta.Key[T], bool) {
	full := MetaOptionPrefix + name
	switch any(*new(T)).(type) {
	case string:
		k := meta.EnsureKey(full, meta.StringParser)
		return any(k).(meta.Key[T]), true
	case bool:
		k := meta.EnsureKey(full, meta.BoolParser)
		return any(k).(meta.Key[T]), true
	}
	return meta.Key[T]{}, false
}
