// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder

import (
	"fmt"

	"go.thesmos.sh/eidos/emit"
)

// AppendField appends f to host's "fields" slot, stamping
// [emit.Provenance.SetBy] with the Context's plugin identifier.
// Optional id is recorded in [emit.Provenance.ID] for later
// positional inserts via [emit.Slot.InsertBefore] /
// [emit.Slot.InsertAfter].
//
// host may be a *[emit.Struct] or a *[emit.Interface]; other kinds
// return [ErrUnsupportedHost]. The wrapped slot append surfaces
// [emit.ErrSlotElementType] when f.Kind() doesn't match the slot's
// declared element kind.
//
// An Interface accepts fields because in some languages that is what
// an interface mostly declares — a TypeScript interface is a
// structural type, not a method set.
func (c *Context) AppendField(host emit.Node, f *emit.Field, id ...string) error {
	if err := nilCheck(host, "field"); err != nil {
		return err
	}
	owner, slot, err := fieldHost(host, "AppendField")
	if err != nil {
		return err
	}
	f.Owner = owner
	return slot.Append(f, c.Provenance(id...))
}

// fieldHost resolves a field-slot host to its owner and slot.
//
// Shared by [Context.AppendField] and [Context.InsertField] so the
// set of kinds that accept a field is stated once; the two disagreeing
// would let a plugin append where it cannot insert.
func fieldHost(host emit.Node, op string) (emit.Node, *emit.Slot, error) {
	switch owner := host.(type) {
	case *emit.Struct:
		slot := owner.FieldsSlot()
		return owner, slot, nil
	case *emit.Interface:
		slot := owner.FieldsSlot()
		return owner, slot, nil
	default:
		return nil, nil, fmt.Errorf(
			"%w: %s expects *emit.Struct or *emit.Interface, got %T",
			ErrUnsupportedHost, op, host,
		)
	}
}

// AppendMethod appends m to host's "methods" slot, stamping
// provenance with the Context's plugin identifier.
//
// host may be a *[emit.Struct], *[emit.Interface], or *[emit.Alias];
// other kinds return [ErrUnsupportedHost].
func (c *Context) AppendMethod(host emit.Node, m *emit.Method, id ...string) error {
	if err := nilCheck(host, "method"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Struct:
		m.Owner = owner
		return owner.MethodsSlot().Append(m, c.Provenance(id...))
	case *emit.Interface:
		m.Owner = owner
		return owner.MethodsSlot().Append(m, c.Provenance(id...))
	case *emit.Alias:
		m.Owner = owner
		return owner.MethodsSlot().Append(m, c.Provenance(id...))
	default:
		return fmt.Errorf(
			"%w: AppendMethod expects *emit.Struct, *emit.Interface, or *emit.Alias, got %T",
			ErrUnsupportedHost, host,
		)
	}
}

// AppendEmbed appends e to host's "embeds" slot. host may be a
// *[emit.Struct] or *[emit.Interface].
func (c *Context) AppendEmbed(host emit.Node, e *emit.Embed, id ...string) error {
	if err := nilCheck(host, "embed"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Struct:
		e.Owner = owner
		return owner.EmbedsSlot().Append(e, c.Provenance(id...))
	case *emit.Interface:
		e.Owner = owner
		return owner.EmbedsSlot().Append(e, c.Provenance(id...))
	default:
		return fmt.Errorf("%w: AppendEmbed expects *emit.Struct or *emit.Interface, got %T", ErrUnsupportedHost, host)
	}
}

// AppendVariant appends v to host's "variants" slot. host must be
// a *[emit.Enum].
func (c *Context) AppendVariant(host *emit.Enum, v *emit.EnumVariant, id ...string) error {
	if host == nil {
		return fmt.Errorf("%w: variant host is nil", ErrNilHost)
	}
	v.Owner = host
	return host.VariantsSlot().Append(v, c.Provenance(id...))
}

// AppendParam appends p to host's "params" slot. host may be a
// *[emit.Function] or *[emit.Method].
func (c *Context) AppendParam(host emit.Node, p *emit.Param, id ...string) error {
	if err := nilCheck(host, "param"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Function:
		p.Owner = owner
		return owner.ParamsSlot().Append(p, c.Provenance(id...))
	case *emit.Method:
		p.Owner = owner
		return owner.ParamsSlot().Append(p, c.Provenance(id...))
	default:
		return fmt.Errorf("%w: AppendParam expects *emit.Function or *emit.Method, got %T", ErrUnsupportedHost, host)
	}
}

// AppendPrebody appends stmt to host's "prebody" slot — code that
// runs before the host's typed Body. host may be a *[emit.Function]
// or *[emit.Method].
func (c *Context) AppendPrebody(host emit.Node, stmt *emit.Stmt, id ...string) error {
	if err := nilCheck(host, "prebody"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Function:
		return owner.Prebody().Append(stmt, c.Provenance(id...))
	case *emit.Method:
		return owner.Prebody().Append(stmt, c.Provenance(id...))
	default:
		return fmt.Errorf("%w: AppendPrebody expects *emit.Function or *emit.Method, got %T", ErrUnsupportedHost, host)
	}
}

// AppendPostbody appends stmt to host's "postbody" slot — code that
// runs after the host's typed Body returns.
func (c *Context) AppendPostbody(host emit.Node, stmt *emit.Stmt, id ...string) error {
	if err := nilCheck(host, "postbody"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Function:
		return owner.Postbody().Append(stmt, c.Provenance(id...))
	case *emit.Method:
		return owner.Postbody().Append(stmt, c.Provenance(id...))
	default:
		return fmt.Errorf("%w: AppendPostbody expects *emit.Function or *emit.Method, got %T", ErrUnsupportedHost, host)
	}
}

// AppendReturn appends r to host's "returns" slot, stamping
// provenance with the Context's plugin identifier. host may be a
// *[emit.Function] or *[emit.Method]; other kinds return
// [ErrUnsupportedHost].
//
// Slot-contributed returns concatenate after the host's typed
// [emit.Function.Returns] / [emit.Method.Returns] at render time;
// the merged slice still has to obey Go's named-vs-anonymous
// uniformity rule, so the surrounding signature stays
// well-formed only when the contributor matches the host's
// existing naming mode.
func (c *Context) AppendReturn(host emit.Node, r *emit.Return, id ...string) error {
	if err := nilCheck(host, "return"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Function:
		return owner.ReturnsSlot().Append(r, c.Provenance(id...))
	case *emit.Method:
		return owner.ReturnsSlot().Append(r, c.Provenance(id...))
	default:
		return fmt.Errorf("%w: AppendReturn expects *emit.Function or *emit.Method, got %T", ErrUnsupportedHost, host)
	}
}

// AppendTag appends tag to host's "tags" slot — cross-cutting
// struct-tag entries on a single field. host must be a *[emit.Field].
//
// The tag arg is an [emit.Tag] (key + value); the renderer renders
// it as `key:"escaped value"` and joins it with the field's
// directly-declared [emit.Field.Tag].
func (c *Context) AppendTag(host *emit.Field, tag *emit.Tag, id ...string) error {
	if host == nil {
		return fmt.Errorf("%w: tag host is nil", ErrNilHost)
	}
	return host.Tags().Append(tag, c.Provenance(id...))
}

// AppendTop appends decl to file.Top() — the file-level slot
// rendered above free-floating declarations. decl is any [emit.Node]
// the file's top slot accepts (Struct, Interface, Function, etc.).
func (c *Context) AppendTop(file *emit.File, decl emit.Node, id ...string) error {
	if file == nil {
		return fmt.Errorf("%w: file is nil", ErrNilHost)
	}
	return file.Top().Append(decl, c.Provenance(id...))
}

// AppendBottom appends decl to file.Bottom() — the file-level slot
// rendered below free-floating declarations.
func (c *Context) AppendBottom(file *emit.File, decl emit.Node, id ...string) error {
	if file == nil {
		return fmt.Errorf("%w: file is nil", ErrNilHost)
	}
	return file.Bottom().Append(decl, c.Provenance(id...))
}

// AppendInit appends stmt to file.Init() — composed into the file's
// `func init()` body. Multiple plugins' contributions concatenate in
// capability-topological order.
func (c *Context) AppendInit(file *emit.File, stmt *emit.Stmt, id ...string) error {
	if file == nil {
		return fmt.Errorf("%w: file is nil", ErrNilHost)
	}
	return file.Init().Append(stmt, c.Provenance(id...))
}

// AppendImport appends an explicit [emit.Import] to file.ImportsSlot()
// — the canonical path for cross-cutting plugins that need a file
// to carry an import the typed Imports list doesn't already declare
// (typically side-effect imports referenced by code the plugin
// contributes elsewhere).
func (c *Context) AppendImport(file *emit.File, imp *emit.Import, id ...string) error {
	if file == nil {
		return fmt.Errorf("%w: file is nil", ErrNilHost)
	}
	imp.Owner = file
	return file.ImportsSlot().Append(imp, c.Provenance(id...))
}

// InsertField places f at the supplied [InsertPos] in host's fields
// slot, stamping provenance with the Context's plugin identifier.
// host may be a *[emit.Struct] or a *[emit.Interface]; other kinds
// return [ErrUnsupportedHost]. The dedicated [Context.AppendField] is
// the common case; use InsertField for [Before] / [After] / [Prepend]
// positional intents.
func (c *Context) InsertField(host emit.Node, f *emit.Field, pos InsertPos, id ...string) error {
	if err := nilCheck(host, "field"); err != nil {
		return err
	}
	owner, slot, err := fieldHost(host, "InsertField")
	if err != nil {
		return err
	}
	f.Owner = owner
	return applyInsert(slot, f, c.Provenance(id...), pos)
}

// InsertMethod places m at the supplied [InsertPos] in host's
// methods slot. host may be a *[emit.Struct], *[emit.Interface], or
// *[emit.Alias]. Other kinds return [ErrUnsupportedHost].
func (c *Context) InsertMethod(host emit.Node, m *emit.Method, pos InsertPos, id ...string) error {
	if err := nilCheck(host, "method"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Struct:
		m.Owner = owner
		return applyInsert(owner.MethodsSlot(), m, c.Provenance(id...), pos)
	case *emit.Interface:
		m.Owner = owner
		return applyInsert(owner.MethodsSlot(), m, c.Provenance(id...), pos)
	case *emit.Alias:
		m.Owner = owner
		return applyInsert(owner.MethodsSlot(), m, c.Provenance(id...), pos)
	default:
		return fmt.Errorf(
			"%w: InsertMethod expects *emit.Struct, *emit.Interface, or *emit.Alias, got %T",
			ErrUnsupportedHost, host,
		)
	}
}

// InsertEmbed places e at the supplied [InsertPos] in host's embeds
// slot. host may be a *[emit.Struct] or *[emit.Interface].
func (c *Context) InsertEmbed(host emit.Node, e *emit.Embed, pos InsertPos, id ...string) error {
	if err := nilCheck(host, "embed"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Struct:
		e.Owner = owner
		return applyInsert(owner.EmbedsSlot(), e, c.Provenance(id...), pos)
	case *emit.Interface:
		e.Owner = owner
		return applyInsert(owner.EmbedsSlot(), e, c.Provenance(id...), pos)
	default:
		return fmt.Errorf("%w: InsertEmbed expects *emit.Struct or *emit.Interface, got %T", ErrUnsupportedHost, host)
	}
}

// InsertVariant places v at the supplied [InsertPos] in host's
// variants slot.
func (c *Context) InsertVariant(host *emit.Enum, v *emit.EnumVariant, pos InsertPos, id ...string) error {
	if host == nil {
		return fmt.Errorf("%w: variant host is nil", ErrNilHost)
	}
	v.Owner = host
	return applyInsert(host.VariantsSlot(), v, c.Provenance(id...), pos)
}

// InsertParam places p at the supplied [InsertPos] in host's params
// slot. host may be a *[emit.Function] or *[emit.Method].
func (c *Context) InsertParam(host emit.Node, p *emit.Param, pos InsertPos, id ...string) error {
	if err := nilCheck(host, "param"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Function:
		p.Owner = owner
		return applyInsert(owner.ParamsSlot(), p, c.Provenance(id...), pos)
	case *emit.Method:
		p.Owner = owner
		return applyInsert(owner.ParamsSlot(), p, c.Provenance(id...), pos)
	default:
		return fmt.Errorf("%w: InsertParam expects *emit.Function or *emit.Method, got %T", ErrUnsupportedHost, host)
	}
}

// InsertPrebody places stmt at the supplied [InsertPos] in host's
// prebody slot. host may be a *[emit.Function] or *[emit.Method].
func (c *Context) InsertPrebody(host emit.Node, stmt *emit.Stmt, pos InsertPos, id ...string) error {
	if err := nilCheck(host, "prebody"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Function:
		return applyInsert(owner.Prebody(), stmt, c.Provenance(id...), pos)
	case *emit.Method:
		return applyInsert(owner.Prebody(), stmt, c.Provenance(id...), pos)
	default:
		return fmt.Errorf("%w: InsertPrebody expects *emit.Function or *emit.Method, got %T", ErrUnsupportedHost, host)
	}
}

// InsertPostbody places stmt at the supplied [InsertPos] in host's
// postbody slot. host may be a *[emit.Function] or *[emit.Method].
func (c *Context) InsertPostbody(host emit.Node, stmt *emit.Stmt, pos InsertPos, id ...string) error {
	if err := nilCheck(host, "postbody"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Function:
		return applyInsert(owner.Postbody(), stmt, c.Provenance(id...), pos)
	case *emit.Method:
		return applyInsert(owner.Postbody(), stmt, c.Provenance(id...), pos)
	default:
		return fmt.Errorf("%w: InsertPostbody expects *emit.Function or *emit.Method, got %T", ErrUnsupportedHost, host)
	}
}

// InsertReturn places r at the supplied [InsertPos] in host's
// returns slot. host may be a *[emit.Function] or *[emit.Method].
func (c *Context) InsertReturn(host emit.Node, r *emit.Return, pos InsertPos, id ...string) error {
	if err := nilCheck(host, "return"); err != nil {
		return err
	}
	switch owner := host.(type) {
	case *emit.Function:
		return applyInsert(owner.ReturnsSlot(), r, c.Provenance(id...), pos)
	case *emit.Method:
		return applyInsert(owner.ReturnsSlot(), r, c.Provenance(id...), pos)
	default:
		return fmt.Errorf("%w: InsertReturn expects *emit.Function or *emit.Method, got %T", ErrUnsupportedHost, host)
	}
}

// InsertTag places tag at the supplied [InsertPos] in host's tags
// slot. host must be a *[emit.Field].
func (c *Context) InsertTag(host *emit.Field, tag *emit.Tag, pos InsertPos, id ...string) error {
	if host == nil {
		return fmt.Errorf("%w: tag host is nil", ErrNilHost)
	}
	return applyInsert(host.Tags(), tag, c.Provenance(id...), pos)
}

// InsertTop places decl at the supplied [InsertPos] in file.Top().
func (c *Context) InsertTop(file *emit.File, decl emit.Node, pos InsertPos, id ...string) error {
	if file == nil {
		return fmt.Errorf("%w: file is nil", ErrNilHost)
	}
	return applyInsert(file.Top(), decl, c.Provenance(id...), pos)
}

// InsertBottom places decl at the supplied [InsertPos] in
// file.Bottom().
func (c *Context) InsertBottom(file *emit.File, decl emit.Node, pos InsertPos, id ...string) error {
	if file == nil {
		return fmt.Errorf("%w: file is nil", ErrNilHost)
	}
	return applyInsert(file.Bottom(), decl, c.Provenance(id...), pos)
}

// InsertInit places stmt at the supplied [InsertPos] in
// file.Init().
func (c *Context) InsertInit(file *emit.File, stmt *emit.Stmt, pos InsertPos, id ...string) error {
	if file == nil {
		return fmt.Errorf("%w: file is nil", ErrNilHost)
	}
	return applyInsert(file.Init(), stmt, c.Provenance(id...), pos)
}

// InsertImport places imp at the supplied [InsertPos] in
// file.ImportsSlot().
func (c *Context) InsertImport(file *emit.File, imp *emit.Import, pos InsertPos, id ...string) error {
	if file == nil {
		return fmt.Errorf("%w: file is nil", ErrNilHost)
	}
	imp.Owner = file
	return applyInsert(file.ImportsSlot(), imp, c.Provenance(id...), pos)
}
