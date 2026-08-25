// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"fmt"

	"go.thesmos.sh/eidos/emit"
)

// ErrSlotHostUnsupported is returned by the `slot` funcmap entry
// when the supplied value does not satisfy [emit.SlotHost]. The
// wrapped message names the offending type so a plugin template
// author can diagnose the misuse (typical cause: invoking
// `{{ slot . "name" }}` against a sub-element such as a Field
// that doesn't own slots).
var ErrSlotHostUnsupported = errors.New("backend: value does not own slots")

// slot is the canonical funcmap accessor for named slots on any
// emit value that satisfies [emit.SlotHost]. Plugin templates use
// it to read a host's slot generically:
//
//	{{ range (slot . "tags").Items }} … {{ end }}
//
// host must satisfy [emit.SlotHost]; values that don't (Field,
// Embed, Param, …) surface as [ErrSlotHostUnsupported].
//
// `slot` is one of the reserved canonical funcmap entries — plugin
// overrides are rejected at Build time.
func slot(host emit.Node, name string) (*emit.Slot, error) {
	owner, ok := host.(emit.SlotHost)
	if !ok {
		return nil, fmt.Errorf("%w: %T", ErrSlotHostUnsupported, host)
	}
	return owner.Slot(name), nil
}

// provenance is the canonical funcmap accessor for an emit value's
// attribution string. The returned text combines the entity's
// source-position (when available) with a human-readable kind
// label, suitable for diagnostic comments inside generated code:
//
//	{{ provenance . }}  →  "emit.struct from internal/users/user.go:42"
//	{{ provenance . }}  →  "emit.function (synthetic)"
//
// Returns "(nil)" for a nil node so the helper never blocks a
// template that lacks defensive nil-checking on the dot context.
//
// `provenance` is one of the reserved canonical funcmap entries —
// plugin overrides are rejected at Build time.
func provenance(n emit.Node) string {
	if n == nil {
		return "(nil)"
	}
	kind := string(n.Kind())
	origin := n.Origin()
	if origin == nil {
		return kind + " (synthetic)"
	}
	pos := origin.Pos()
	if pos.File == "" {
		return kind + " (synthetic)"
	}
	if pos.Line > 0 {
		return fmt.Sprintf("%s from %s:%d", kind, pos.File, pos.Line)
	}
	return fmt.Sprintf("%s from %s", kind, pos.File)
}
