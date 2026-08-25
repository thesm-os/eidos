// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

// RewireOwners walks the given package and reconstructs every
// Owner back-pointer on its descendant nodes. The function is the
// post-load complement of JSON deserialization: encoding strips
// Owner pointers (to avoid the host → child cycle that JSON cannot
// otherwise represent), and RewireOwners restores them so the
// in-memory shape matches what a fresh frontend pass would produce.
//
// RewireOwners is idempotent — calling it on an already-wired
// package leaves the graph unchanged. It tolerates partially-wired
// inputs (a graph where some Owners are set and others are nil) by
// re-wiring every reachable node unconditionally.
//
// Callers typically invoke RewireOwners on a Package produced by
// json.Unmarshal before adding the package to the store. The store
// itself does not call RewireOwners — it accepts whatever the
// frontend (or test fixture) produced.
func RewireOwners(p *Package) {
	if p == nil {
		return
	}
	for _, f := range p.Files {
		f.Owner = p
		for _, imp := range f.Imports {
			imp.Owner = f
		}
	}
	for _, imp := range p.Imports {
		imp.Owner = p
	}
	for _, s := range p.Structs {
		rewireStruct(s)
	}
	for _, i := range p.Interfaces {
		rewireInterface(i)
	}
	for _, fn := range p.Functions {
		rewireFunction(fn)
	}
	for _, e := range p.Enums {
		rewireEnum(e)
	}
	for _, a := range p.Aliases {
		rewireAlias(a)
	}
	for _, v := range p.Variables {
		rewireTypeRef(v.Type)
	}
	for _, c := range p.Constants {
		rewireTypeRef(c.Type)
	}
}

// rewireStruct re-attaches every back-pointer reachable from s.
func rewireStruct(s *Struct) {
	for _, tp := range s.TypeParams {
		tp.Owner = s
		rewireConstraint(tp.Constraint)
	}
	for _, f := range s.Fields {
		f.Owner = s
		rewireTypeRef(f.Type)
	}
	for _, e := range s.Embeds {
		e.Owner = s
		rewireTypeRef(e.Type)
	}
	for _, m := range s.Methods {
		m.Owner = s
		rewireMethod(m)
	}
}

// rewireInterface re-attaches every back-pointer reachable from i.
func rewireInterface(i *Interface) {
	for _, tp := range i.TypeParams {
		tp.Owner = i
		rewireConstraint(tp.Constraint)
	}
	for _, f := range i.Fields {
		f.Owner = i
		rewireTypeRef(f.Type)
	}
	for _, m := range i.Methods {
		m.Owner = i
		rewireMethod(m)
	}
	for _, e := range i.Embeds {
		e.Owner = i
		rewireTypeRef(e.Type)
		if e.Resolved != nil {
			// The projection is a whole interface, so it re-wires as
			// one: its methods take it as their owner, which is what
			// keeps [PkgPathOf] answering the declaring package rather
			// than the embedding one after a round-trip.
			rewireInterface(e.Resolved)
		}
	}
}

// rewireMethod re-attaches every back-pointer reachable from m. The
// method's own Owner has already been set by the caller.
func rewireMethod(m *Method) {
	for _, tp := range m.TypeParams {
		tp.Owner = m
		rewireConstraint(tp.Constraint)
	}
	for _, p := range m.Params {
		p.Owner = m
		rewireTypeRef(p.Type)
	}
	for _, r := range m.Returns {
		r.Owner = m
		rewireTypeRef(r.Type)
	}
	rewireTypeRef(m.Receiver)
}

// rewireFunction re-attaches every back-pointer reachable from f.
func rewireFunction(f *Function) {
	for _, tp := range f.TypeParams {
		tp.Owner = f
		rewireConstraint(tp.Constraint)
	}
	for _, p := range f.Params {
		p.Owner = f
		rewireTypeRef(p.Type)
	}
	for _, r := range f.Returns {
		r.Owner = f
		rewireTypeRef(r.Type)
	}
}

// rewireEnum re-attaches every back-pointer reachable from e.
func rewireEnum(e *Enum) {
	rewireTypeRef(e.Underlying)
	for _, v := range e.Variants {
		v.Owner = e
	}
	// Mirrors rewireAlias: an enum owns its method set for the same
	// reason an alias does, so a deserialised enum has to arrive
	// wired the same way one built by the frontend does.
	for _, m := range e.Methods {
		m.Owner = e
		rewireMethod(m)
	}
}

// rewireAlias re-attaches every back-pointer reachable from a.
func rewireAlias(a *Alias) {
	for _, tp := range a.TypeParams {
		tp.Owner = a
		rewireConstraint(tp.Constraint)
	}
	rewireTypeRef(a.Target)
	for _, m := range a.Methods {
		m.Owner = a
		rewireMethod(m)
	}
}

// rewireTypeRef walks composite type references and the inline
// fields / methods / embeds of anonymous-type variants, re-attaching
// Owner pointers to the enclosing ref.
func rewireTypeRef(r *TypeRef) {
	if r == nil {
		return
	}
	switch r.TypeKind {
	case TypeRefNamed:
		for _, a := range r.TypeArgs {
			rewireTypeRef(a)
		}
	case TypeRefPointer, TypeRefSlice, TypeRefArray:
		rewireTypeRef(r.Elem)
	case TypeRefMap:
		rewireTypeRef(r.MapKey)
		rewireTypeRef(r.MapValue)
	case TypeRefFunc:
		for _, p := range r.FuncParams {
			rewireTypeRef(p)
		}
		for _, ret := range r.FuncReturns {
			rewireTypeRef(ret)
		}
	case TypeRefAnonStruct:
		for _, f := range r.Fields {
			f.Owner = r
			rewireTypeRef(f.Type)
		}
		for _, e := range r.Embeds {
			e.Owner = r
			rewireTypeRef(e.Type)
		}
	case TypeRefAnonInterface:
		for _, m := range r.Methods {
			m.Owner = r
			rewireMethod(m)
		}
		for _, e := range r.Embeds {
			e.Owner = r
			rewireTypeRef(e.Type)
		}
	case TypeRefTypeParam:
		// Leaf — no children to re-wire.
	}
}

// rewireConstraint walks a Constraint's embedded refs and re-attaches
// any back-pointers reachable from them. Constraints themselves are
// not Nodes and have no Owner of their own.
func rewireConstraint(c *Constraint) {
	if c == nil {
		return
	}
	for _, e := range c.Embedded {
		rewireTypeRef(e)
	}
}
