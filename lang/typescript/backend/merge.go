// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"fmt"

	"go.thesmos.sh/eidos/emit"
)

// ErrDuplicateEntity reports two contributions declaring the same
// member on one host.
//
// Rejected rather than resolved: one of the two would be dropped, and
// which one is decided by plugin registration order — so a run would
// silently emit a member that a different ordering of the same
// plugins would not.
var ErrDuplicateEntity = errors.New("backend: duplicate member on one declaration")

// mergedFields returns a host's typed fields followed by the ones its
// fields slot received.
//
// Typed first, then contributions: what the generator declared
// outright is what the reader expects at the top, and a cross-cutting
// plugin adds below it.
func mergedFields(host emit.Node) ([]*emit.Field, error) {
	var out []*emit.Field
	switch h := host.(type) {
	case *emit.Struct:
		out = append(out, h.Fields...)
		out = append(out, slotFields(h.FieldsSlot())...)
	case *emit.Interface:
		out = append(out, h.Fields...)
		out = append(out, slotFields(h.FieldsSlot())...)
	default:
		return nil, nil
	}
	return out, rejectDuplicateFields(out)
}

// mergedMethods returns a host's typed methods followed by slot
// contributions.
func mergedMethods(host emit.Node) ([]*emit.Method, error) {
	var out []*emit.Method
	switch h := host.(type) {
	case *emit.Struct:
		out = append(out, h.Methods...)
		out = append(out, slotMethods(h.MethodsSlot())...)
	case *emit.Interface:
		out = append(out, h.Methods...)
		out = append(out, slotMethods(h.MethodsSlot())...)
	default:
		return nil, nil
	}
	return out, rejectDuplicateMethods(out)
}

// mergedEmbeds returns a host's typed embeds followed by slot
// contributions.
// Unlike fields and methods it reports no error: two identical
// embeds are legal — a class may implement an interface its base
// already implements — so there is nothing to reject.
func mergedEmbeds(host emit.Node) []*emit.Embed {
	var out []*emit.Embed
	switch h := host.(type) {
	case *emit.Struct:
		out = append(out, h.Embeds...)
		out = append(out, slotEmbeds(h.EmbedsSlot())...)
	case *emit.Interface:
		out = append(out, h.Embeds...)
		out = append(out, slotEmbeds(h.EmbedsSlot())...)
	}
	return out
}

// mergedVariants returns an enum's typed variants followed by slot
// contributions.
func mergedVariants(e *emit.Enum) ([]*emit.EnumVariant, error) {
	if e == nil {
		return nil, nil
	}
	out := append([]*emit.EnumVariant(nil), e.Variants...)
	for i := range e.VariantsSlot().Len() {
		if v, ok := e.VariantsSlot().At(i).(*emit.EnumVariant); ok {
			out = append(out, v)
		}
	}
	return out, rejectDuplicateVariants(out)
}

// slotFields projects a slot's items to fields, skipping anything
// else. The slot's element-kind constraint rejects a mismatch at
// append time, so a survivor here is a bug rather than user input.
func slotFields(s *emit.Slot) []*emit.Field {
	out := make([]*emit.Field, 0, s.Len())
	for i := range s.Len() {
		if f, ok := s.At(i).(*emit.Field); ok {
			out = append(out, f)
		}
	}
	return out
}

// slotMethods projects a slot's items to methods.
func slotMethods(s *emit.Slot) []*emit.Method {
	out := make([]*emit.Method, 0, s.Len())
	for i := range s.Len() {
		if m, ok := s.At(i).(*emit.Method); ok {
			out = append(out, m)
		}
	}
	return out
}

// slotEmbeds projects a slot's items to embeds.
func slotEmbeds(s *emit.Slot) []*emit.Embed {
	out := make([]*emit.Embed, 0, s.Len())
	for i := range s.Len() {
		if e, ok := s.At(i).(*emit.Embed); ok {
			out = append(out, e)
		}
	}
	return out
}

// rejectDuplicateFields reports two fields of one name.
func rejectDuplicateFields(fields []*emit.Field) error {
	seen := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		if _, dup := seen[f.Name]; dup {
			return fmt.Errorf("%w: property %q", ErrDuplicateEntity, f.Name)
		}
		seen[f.Name] = struct{}{}
	}
	return nil
}

// rejectDuplicateMethods reports two methods of one name.
func rejectDuplicateMethods(methods []*emit.Method) error {
	seen := make(map[string]struct{}, len(methods))
	for _, m := range methods {
		if _, dup := seen[m.Name]; dup {
			return fmt.Errorf("%w: method %q", ErrDuplicateEntity, m.Name)
		}
		seen[m.Name] = struct{}{}
	}
	return nil
}

// rejectDuplicateVariants reports two enum members of one name.
func rejectDuplicateVariants(variants []*emit.EnumVariant) error {
	seen := make(map[string]struct{}, len(variants))
	for _, v := range variants {
		if _, dup := seen[v.Name]; dup {
			return fmt.Errorf("%w: enum member %q", ErrDuplicateEntity, v.Name)
		}
		seen[v.Name] = struct{}{}
	}
	return nil
}
