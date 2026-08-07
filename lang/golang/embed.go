// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strings"

	"go.thesmos.sh/eidos/node"
)

// Struct embedding: what an embedded type contributes to the type
// that embeds it.
//
// A generator reading `s.Fields` reads what the source typed, not
// what the struct has. `struct{ Base; Name string }` has every
// exported field of Base as well, reachable unqualified, and a
// builder that offers a setter per declared field silently offers
// none for them.
//
// # Go's promotion rules, and which of them are answerable here
//
// A field promotes when it is reachable at a shallower depth than
// any other of that name. Shallowest wins; a tie at equal depth
// promotes neither, and both become unreachable without an explicit
// qualifier. A declared field always shadows a promoted one,
// because depth zero beats everything.
//
// All three are implemented. What is not answerable without the
// graph is what an embedded type *is* — the model records a name
// and a package — so every function here that walks past the first
// level takes a [Resolver] and reports what it could not reach
// rather than guessing.

// maxEmbedDepth bounds the promotion walk.
//
// Go permits arbitrary embedding depth and real code rarely
// exceeds two. Eight is far past that and shallow enough that a
// cyclic graph — which no compiling source produces and a
// hand-built fixture can — terminates in a generation pass.
const maxEmbedDepth = 8

// EmbedIdent returns the field name an embedded type contributes,
// and whether it was embedded by pointer.
//
// An embed by pointer carries its name on the pointee rather than
// on the reference itself, so reading the reference's own name
// yields the empty string and the whole field is silently dropped —
// which is the bug this exists to prevent.
//
// A generic embed contributes the base name without its arguments:
// `Base[T]` embeds as the field `Base`.
func EmbedIdent(e *node.Embed) (name string, byPointer bool) {
	if e == nil || e.Type == nil {
		return "", false
	}
	t := e.Type
	if t.IsPointer() {
		if t.Elem == nil {
			return "", false
		}
		return t.Elem.Name, true
	}
	return t.Name, false
}

// EmbedTarget returns the type reference an embed names, with any
// pointer stripped.
//
// What a caller resolves to reach the embedded declaration: the
// pointer is a fact about the embedding, not about the type.
func EmbedTarget(e *node.Embed) *node.TypeRef {
	if e == nil {
		return nil
	}
	return Deref(e.Type)
}

// PromotedField is one field reachable through embedding, with the
// path that reaches it.
type PromotedField struct {
	// Field is the declaration itself, on whichever type declared
	// it.
	Field *node.Field

	// Depth is how many embeds were traversed: zero for a field the
	// struct declared, one for a field of a directly embedded type.
	Depth int

	// Path is the embedded field names traversed to reach it, outer
	// first — `[]string{"Base", "Meta"}` for a field of a type
	// embedded in a type embedded here. Empty at depth zero.
	//
	// Carried because a generator writing an explicit selector needs
	// it: promotion makes `v.Name` legal, but a composite literal
	// setting the same field has to write `v.Base.Meta.Name`.
	Path []string

	// ThroughPointer reports whether any embed on the path was by
	// pointer.
	//
	// Load-bearing for a composite literal: an embedded pointer is
	// nil until something allocates it, so a generated setter
	// writing through one panics unless it allocates first.
	ThroughPointer bool
}

// Selector renders the explicit path to the field — `Base.Meta.Name`
// — which is what a composite literal or an unambiguous read needs.
func (p PromotedField) Selector() string {
	if len(p.Path) == 0 {
		return p.Field.Name
	}
	var b strings.Builder
	for _, seg := range p.Path {
		b.WriteString(seg)
		b.WriteByte('.')
	}
	b.WriteString(p.Field.Name)
	return b.String()
}

// FieldSet returns every field reachable on s without an explicit
// qualifier, and whether the walk completed.
//
// Go's promotion rules applied in full: a declared field shadows a
// promoted one, a shallower promotion shadows a deeper one, and two
// promotions at equal depth cancel — neither is reachable, so
// neither appears.
//
// The second result is false when an embed named a type the
// resolver could not reach. The answer is then smaller rather than
// wrong, and a generator emitting against a partial field set
// produces output missing setters rather than output naming fields
// that do not exist — but it must not treat the partial answer as
// complete, which is what the flag is for.
//
// Order is declaration order at each level, outer levels first, so
// generated output is stable as an embedded type gains a field.
func FieldSet(s *node.Struct, r Resolver) ([]PromotedField, bool) {
	if s == nil {
		return nil, true
	}
	// byName accumulates every candidate for a name across depths;
	// the shadowing rules are applied once at the end, because a
	// shallower candidate may be found after a deeper one.
	byName := map[string][]PromotedField{}
	order := []string{}
	complete := collectFields(s, r, 0, nil, false, byName, &order, map[string]struct{}{})

	out := make([]PromotedField, 0, len(order))
	for _, name := range order {
		if winner, ok := shallowestUnique(byName[name]); ok {
			out = append(out, winner)
		}
	}
	return out, complete
}

// collectFields walks s and its embeds, recording every candidate
// for each field name.
func collectFields(
	s *node.Struct,
	r Resolver,
	depth int,
	path []string,
	throughPointer bool,
	byName map[string][]PromotedField,
	order *[]string,
	visited map[string]struct{},
) bool {
	if s == nil || depth > maxEmbedDepth {
		return false
	}
	// Guards a cycle. Illegal in Go — a struct cannot embed itself by
	// value — and reachable through a pointer embed or a malformed
	// graph, where it would otherwise not terminate.
	if s.QName() != "" {
		if _, looping := visited[s.QName()]; looping {
			return true
		}
		visited[s.QName()] = struct{}{}
	}

	for _, f := range s.Fields {
		if f == nil || f.Name == "" {
			continue
		}
		if _, seen := byName[f.Name]; !seen {
			*order = append(*order, f.Name)
		}
		byName[f.Name] = append(byName[f.Name], PromotedField{
			Field: f, Depth: depth, Path: path, ThroughPointer: throughPointer,
		})
	}

	complete := true
	for _, e := range s.Embeds {
		name, byPointer := EmbedIdent(e)
		if name == "" {
			continue
		}
		// The embedded field itself is reachable by its own name, and
		// shadows anything promoted through it.
		if _, seen := byName[name]; !seen {
			*order = append(*order, name)
		}
		byName[name] = append(byName[name], PromotedField{
			Field:          &node.Field{Name: name, Type: e.Type, Owner: s},
			Depth:          depth,
			Path:           path,
			ThroughPointer: throughPointer,
		})

		if r == nil {
			complete = false
			continue
		}
		target, found := r.Resolve(EmbedTarget(e))
		if !found {
			complete = false
			continue
		}
		inner, ok := target.(*node.Struct)
		if !ok {
			// An embedded interface contributes methods, not fields.
			// See [PromotedMethods].
			continue
		}
		next := make([]string, 0, len(path)+1)
		next = append(next, path...)
		next = append(next, name)
		if !collectFields(inner, r, depth+1, next, throughPointer || byPointer, byName, order, visited) {
			complete = false
		}
	}
	return complete
}

// shallowestUnique applies Go's promotion rules to one name's
// candidates.
//
// The shallowest wins; a tie at that depth promotes neither, since
// Go makes an ambiguous selector an error rather than a choice.
func shallowestUnique(candidates []PromotedField) (PromotedField, bool) {
	if len(candidates) == 0 {
		return PromotedField{}, false
	}
	best := candidates[0]
	ties := 1
	for _, c := range candidates[1:] {
		switch {
		case c.Depth < best.Depth:
			best, ties = c, 1
		case c.Depth == best.Depth:
			ties++
		}
	}
	if ties > 1 {
		return PromotedField{}, false
	}
	return best, true
}

// PromotedFields returns only the fields reached through embedding
// — [FieldSet] minus what the struct declared itself.
//
// For a generator that treats the two differently: a builder
// offering a setter per declared field and one whole setter for
// each embedded value needs to tell them apart, and a struct
// literal sets an embedded value as a unit.
func PromotedFields(s *node.Struct, r Resolver) ([]PromotedField, bool) {
	all, complete := FieldSet(s, r)
	out := make([]PromotedField, 0, len(all))
	for _, f := range all {
		if f.Depth > 0 {
			out = append(out, f)
		}
	}
	return out, complete
}

// ExportedFieldSet is [FieldSet] restricted to fields a generated
// file in another package can name.
//
// The set a builder, a mock or a fixture can actually set:
// unexported fields are visible to the declaring package and to
// nothing else, and a generator routed elsewhere that emitted a
// setter for one produces a file that does not compile.
func ExportedFieldSet(s *node.Struct, r Resolver) ([]PromotedField, bool) {
	all, complete := FieldSet(s, r)
	out := make([]PromotedField, 0, len(all))
	for _, f := range all {
		if IsExported(f.Field.Name) {
			out = append(out, f)
		}
	}
	return out, complete
}

// PromotedMethods returns the method set an embedded type
// contributes to s, and whether every embed resolved.
//
// A struct embedding `io.Reader` has `Read` and declares nothing,
// so a generator asking what a struct implements has to walk the
// embeds — which is the reason `go.embedsInterface` exists as a
// stamp and this exists as the answer behind it.
//
// # The pointer rule
//
// Embedding `T` promotes T's value-receiver methods onto `S` and
// its pointer-receiver methods onto `*S`. Embedding `*T` promotes
// both onto both. The distinction decides whether a generated
// assertion may use the value form, and it is recorded on each
// entry rather than resolved here, because only the caller knows
// which form it is about to emit.
func PromotedMethods(s *node.Struct, r Resolver) ([]PromotedMethod, bool) {
	if s == nil {
		return nil, true
	}
	seen := map[string]struct{}{}
	for _, m := range s.Methods {
		if m != nil {
			seen[m.Name] = struct{}{}
		}
	}
	out := []PromotedMethod{}
	complete := true
	for _, e := range s.Embeds {
		name, byPointer := EmbedIdent(e)
		if name == "" {
			continue
		}
		if r == nil {
			complete = false
			continue
		}
		target, found := r.Resolve(EmbedTarget(e))
		if !found {
			complete = false
			continue
		}
		for _, m := range methodsOfDecl(target) {
			if m == nil {
				continue
			}
			// A method the struct declares itself shadows the promoted
			// one at depth zero, and Go resolves the selector to it.
			if _, shadowed := seen[m.Name]; shadowed {
				continue
			}
			seen[m.Name] = struct{}{}
			out = append(out, PromotedMethod{
				Method: m, Through: name, ThroughPointer: byPointer,
			})
		}
	}
	return out, complete
}

// PromotedMethod is one method reached through embedding.
type PromotedMethod struct {
	// Method is the declaration itself.
	Method *node.Method

	// Through is the embedded field's name.
	Through string

	// ThroughPointer reports whether the embed was by pointer, which
	// decides whether the method reaches the value form as well as
	// the pointer form.
	ThroughPointer bool
}

// methodsOfDecl returns the methods a declaration carries.
//
// A type switch because the model puts the list on each kind rather
// than behind an accessor, and a caller resolving an embed does not
// know which kind it reached.
func methodsOfDecl(n node.Node) []*node.Method {
	switch v := n.(type) {
	case *node.Struct:
		return v.Methods
	case *node.Interface:
		return v.Methods
	case *node.Enum:
		return v.Methods
	case *node.Alias:
		return v.Methods
	default:
		return nil
	}
}

// EmbedsType reports whether s embeds the named type, directly or
// through another embed.
//
// Direct-only when r is nil, which is the honest partial answer: a
// caller without the graph can see the first level and no further.
func EmbedsType(s *node.Struct, qname string, r Resolver) bool {
	if s == nil {
		return false
	}
	for _, e := range s.Embeds {
		target := EmbedTarget(e)
		if QName(target) == qname {
			return true
		}
		if r == nil {
			continue
		}
		decl, found := r.Resolve(target)
		if !found {
			continue
		}
		if inner, ok := decl.(*node.Struct); ok && EmbedsType(inner, qname, r) {
			return true
		}
	}
	return false
}
