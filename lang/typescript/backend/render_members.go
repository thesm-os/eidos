// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
)

// renderMembers spells the property list of a struct or interface,
// one per line, including any the fields slot contributed.
func (s *renderState) renderMembers(host emit.Node) (string, error) {
	fields, err := mergedFields(host)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, f := range fields {
		line, err := s.member(f)
		if err != nil {
			return "", err
		}
		b.WriteString(line)
	}
	return b.String(), nil
}

// member spells one property signature.
func (s *renderState) member(f *emit.Field) (string, error) {
	typ, err := s.renderType(f.Type)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString(renderDocs(f.Docs()))
	b.WriteString(modifiers(f))

	if ro, _ := typescript.MetaReadonly.Get(f.Meta()); ro {
		b.WriteString("readonly ")
	}
	b.WriteString(memberName(f, f.Name))

	// `?` and `| undefined` are different claims: an optional
	// property may be absent from an object entirely, where a
	// undefined-valued one must be present. Rendering optionality as
	// the union would fail under exactOptionalPropertyTypes.
	if opt, _ := typescript.MetaOptional.Get(f.Meta()); opt {
		b.WriteString("?")
	}

	if typ != "" {
		b.WriteString(": " + typ)
	}
	b.WriteString(";\n")
	return b.String(), nil
}

// renderMethods spells a host's method signatures, including any the
// methods slot contributed.
func (s *renderState) renderMethods(host emit.Node) (string, error) {
	methods, err := mergedMethods(host)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, m := range methods {
		line, err := s.method(m)
		if err != nil {
			return "", err
		}
		b.WriteString(line)
	}
	return b.String(), nil
}

// method spells one method signature.
func (s *renderState) method(m *emit.Method) (string, error) {
	params, err := s.renderParams(m.Params)
	if err != nil {
		return "", err
	}
	ret, err := s.renderReturn(m.Returns)
	if err != nil {
		return "", err
	}
	typeParams, err := s.renderTypeParams(m.TypeParams)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString(renderDocs(m.Docs()))

	// An overloaded callable renders its overload signatures and not
	// the derived one: the overloads are what a caller may use, and the
	// implementation signature exists only to cover them — which is a
	// body fact, and this backend renders declarations. TypeScript
	// resolves a call against the signatures top-down, so source order
	// is preserved.
	if overloads, ok := typescript.MetaOverloads.Get(m.Meta()); ok && len(overloads) > 0 {
		mods := modifiers(m)
		for _, o := range overloads {
			b.WriteString(mods + o.Text + ";\n")
		}
		return b.String(), nil
	}

	// MetaAsync is deliberately not rendered. `async` is illegal on an
	// interface method and in an ambient class alike — it describes how
	// a body produces its result, and a declaration has no body. The
	// Promise return type is the contract a caller sees either way.
	b.WriteString(modifiers(m))
	if accessor, ok := typescript.MetaAccessor.Get(m.Meta()); ok && accessor != "" {
		b.WriteString(accessor + " ")
	}
	b.WriteString(memberName(m, m.Name))
	if opt, _ := typescript.MetaOptional.Get(m.Meta()); opt {
		b.WriteString("?")
	}
	b.WriteString(typeParams)
	b.WriteString("(" + params + ")")
	if ret != "" {
		b.WriteString(": " + ret)
	}
	b.WriteString(";\n")
	return b.String(), nil
}

// modifiers spells the member keywords a declaration carries, in the
// order the grammar requires: accessibility, then static, then
// abstract.
//
// Visibility renders whatever was stamped, `public` included — the
// key's own contract is that absent and public are distinguishable
// precisely so a backend does not invent a keyword the author
// omitted, which means one that was stamped was written.
// [typescript.VisibilityHard] is not a keyword at all; the `#` rides
// the name, which [memberName] handles.
func modifiers(n emit.Node) string {
	var b strings.Builder
	v, ok := typescript.MetaVisibility.Get(n.Meta())
	if ok && v != "" && v != typescript.VisibilityHard {
		b.WriteString(v + " ")
	}
	if static, _ := typescript.MetaStatic.Get(n.Meta()); static {
		b.WriteString("static ")
	}
	if abstract, _ := typescript.MetaAbstract.Get(n.Meta()); abstract {
		b.WriteString("abstract ")
	}
	return b.String()
}

// memberName spells a member's name, carrying the `#` of a
// hard-private member.
//
// The marker is part of the name rather than a modifier, and it has
// to bypass [typescript.PropertyKey]: `#x` is not a well-formed
// identifier, so the key helper would quote it — and `'#x'` declares
// a public property whose name contains a hash.
func memberName(n emit.Node, name string) string {
	if v, ok := typescript.MetaVisibility.Get(n.Meta()); ok && v == typescript.VisibilityHard {
		return "#" + typescript.Ident(strings.TrimPrefix(name, "#"))
	}
	return typescript.PropertyKey(name)
}

// renderParams spells a parameter list.
func (s *renderState) renderParams(params []*emit.Param) (string, error) {
	parts := make([]string, 0, len(params))
	for i, p := range params {
		typ, err := s.renderType(p.Type)
		if err != nil {
			return "", err
		}

		name := p.Name
		if name == "" {
			// TypeScript requires a name in a signature; an unnamed
			// parameter would declare one named after its type.
			name = positionalName(i)
		}

		var part strings.Builder
		if p.Variadic {
			part.WriteString("...")
		}
		part.WriteString(typescript.Ident(name))
		if opt, _ := typescript.MetaOptional.Get(p.Meta()); opt && !p.Variadic {
			part.WriteString("?")
		}
		if typ != "" {
			// A rest parameter's declared type is the element, per
			// node.Param's contract, and TypeScript annotates the
			// array — so the array is restored here.
			if p.Variadic {
				typ = arrayOf(typ)
			}
			part.WriteString(": " + typ)
		}
		parts = append(parts, part.String())
	}
	return strings.Join(parts, ", "), nil
}

// renderReturn spells a return clause.
//
// Several returns become a tuple: TypeScript has one return value,
// and a signature carrying more than one is spelled as the tuple that
// holds them.
func (s *renderState) renderReturn(returns []*emit.Return) (string, error) {
	switch len(returns) {
	case 0:
		return typescript.TypeVoid, nil
	case 1:
		return s.renderType(returns[0].Type)
	default:
		parts := make([]string, 0, len(returns))
		for _, r := range returns {
			got, err := s.renderType(r.Type)
			if err != nil {
				return "", err
			}
			parts = append(parts, got)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	}
}

// renderTypeParams spells a generic parameter list.
func (s *renderState) renderTypeParams(params []*emit.TypeParam) (string, error) {
	if len(params) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(params))
	for _, p := range params {
		part := typescript.Ident(p.Name)
		if bound, err := s.constraintOf(p); err != nil {
			return "", err
		} else if bound != "" {
			part += " extends " + bound
		}
		// The default is verbatim source text — the `= string` in
		// `<T = string>` — because a default is a type expression the
		// model holds no structure for, and the author's spelling is
		// the only faithful one.
		if def, ok := typescript.MetaTypeParamDefault.Get(p.Meta()); ok && def != "" {
			part += " = " + def
		}
		parts = append(parts, part)
	}
	return "<" + strings.Join(parts, ", ") + ">", nil
}

// renderHeritage spells the extends and implements clauses.
//
// An interface extends everything it embeds; a class extends at most
// one base and implements the rest. The heritage marker the frontend
// stamped decides which, and an embed with no marker is an extends —
// the reading that holds for an interface, which is where an
// unmarked embed comes from.
func (s *renderState) renderHeritage(host emit.Node) (string, error) {
	var extends, implements []string
	for _, e := range mergedEmbeds(host) {
		got, err := s.renderType(e.Type)
		if err != nil {
			return "", err
		}
		if got == "" {
			continue
		}
		kind, _ := typescript.MetaHeritage.Get(e.Meta())
		if kind == typescript.HeritageImplements {
			implements = append(implements, got)
			continue
		}
		extends = append(extends, got)
	}

	var b strings.Builder
	if len(extends) > 0 {
		b.WriteString(" extends " + strings.Join(extends, ", "))
	}
	if len(implements) > 0 {
		b.WriteString(" implements " + strings.Join(implements, ", "))
	}
	return b.String(), nil
}

// renderVariants spells an enum's members.
func (s *renderState) renderVariants(e *emit.Enum) (string, error) {
	variants, err := mergedVariants(e)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, v := range variants {
		b.WriteString(renderDocs(v.Docs()))
		b.WriteString(typescript.Ident(v.Name))
		if v.Value != nil {
			val, err := s.renderExpr(v.Value)
			if err != nil {
				return "", err
			}
			if val != "" {
				b.WriteString(" = " + val)
			}
		}
		// Trailing comma on every member, the last one included: a
		// later member then adds one line rather than touching two.
		b.WriteString(",\n")
	}
	return b.String(), nil
}

// constraintOf spells a type parameter's bound.
//
// Several bounds become an intersection. TypeScript's `extends` takes
// one type, and a parameter required to satisfy two interfaces is
// bounded by the type that is both — which is what `A & B` names.
func (s *renderState) constraintOf(p *emit.TypeParam) (string, error) {
	if p.Constraint == nil || len(p.Constraint.Embedded) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(p.Constraint.Embedded))
	for _, e := range p.Constraint.Embedded {
		got, err := s.renderType(e)
		if err != nil {
			return "", err
		}
		if got != "" {
			parts = append(parts, got)
		}
	}
	return strings.Join(parts, " & "), nil
}

// positionalName invents a parameter name for a signature that
// carries only types.
func positionalName(i int) string {
	return "arg" + strconv.Itoa(i)
}

// arrayOf spells the array form of an element type, parenthesising
// where a postfix `[]` would otherwise bind too tightly.
func arrayOf(elem string) string {
	if needsParens(elem) {
		return "(" + elem + ")[]"
	}
	return elem + "[]"
}
