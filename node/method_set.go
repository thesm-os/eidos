// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

// InterfaceResolver resolves an embedded type reference to the
// interface it names.
//
// The two results are distinct answers a caller acts on
// differently. (nil, false) means this run never loaded the
// reference — legitimate for a narrow run over one package, and not
// a fault in the source. (nil, true) means the reference resolved
// to something that is not an interface, which is a fault: an
// interface cannot embed a struct.
//
// Taken as a callback rather than a store handle because the method
// set is a fact about the model and [node] cannot import the store
// that holds the graph. [store.Reader.MethodSet] supplies the
// resolver for callers who have one.
type InterfaceResolver func(*TypeRef) (iface *Interface, found bool)

// MethodSetReason classifies why an embed contributed no methods.
//
// Separate reasons rather than one "failed" flag because the
// consequences differ: an unresolved embed is usually the caller's
// run being narrow, while a non-interface or cyclic embed is a
// defect in the source that no wider run will fix.
type MethodSetReason int

const (
	// ReasonUnresolved is an embed this run did not load. A
	// generator reporting it should say so rather than treat the
	// interface as complete — its double would be missing methods
	// the interface has.
	ReasonUnresolved MethodSetReason = iota

	// ReasonNonInterface is an embed that resolved to a declaration
	// that is not an interface.
	ReasonNonInterface

	// ReasonCyclic is an embed already being walked. Go rejects
	// embedding cycles, so reaching one means the graph was built
	// by hand or by a frontend that admitted invalid source; the
	// walk breaks rather than recursing forever.
	ReasonCyclic

	// ReasonGeneric is a parameterised embed. Resolving it needs
	// the type arguments substituted through the embedded
	// interface's method signatures, which the model does not carry
	// — so the methods are refused rather than reported with the
	// uninstantiated parameter names, which would render as
	// references to type parameters the embedding interface does
	// not declare.
	ReasonGeneric
)

// String renders the reason for a diagnostic.
func (r MethodSetReason) String() string {
	switch r {
	case ReasonUnresolved:
		return "not loaded by this run"
	case ReasonNonInterface:
		return "resolves to a declaration that is not an interface"
	case ReasonCyclic:
		return "embed cycle"
	case ReasonGeneric:
		return "parameterised embed"
	default:
		return "method_set_reason(?)"
	}
}

// MethodSetIssue is one embed that contributed no methods, and why.
type MethodSetIssue struct {
	// Embed is the embed that could not be walked.
	Embed *Embed

	// Reason classifies the failure.
	Reason MethodSetReason
}

// MethodSetResult is an interface's resolved method set plus every
// embed that could not be walked.
//
// Issues are reported rather than returned as an error because a
// partial answer is usually still useful: a generator can emit the
// methods it did resolve and report the rest, which beats emitting
// nothing. What it must not do is treat a partial answer as
// complete — hence [MethodSetResult.OK].
type MethodSetResult struct {
	// Methods is the resolved set: the interface's own declarations
	// first, then each embed's contribution in embed order.
	Methods []*Method

	// Issues holds every embed that contributed nothing.
	Issues []MethodSetIssue
}

// OK reports whether every embed resolved.
//
// A generator emitting a type that must satisfy the interface
// checks this: a double built from an incomplete set does not
// implement the interface it doubles, and the compiler reports that
// against the generated file rather than against the run that
// produced it.
func (r MethodSetResult) OK() bool { return len(r.Issues) == 0 }

// ByName returns the resolved method of that name, or nil.
//
// The whole reason to resolve a method set is to ask about it, and
// asking by name is what a caller composing a call expression
// needs.
func (r MethodSetResult) ByName(name string) *Method {
	for _, m := range r.Methods {
		if m != nil && m.Name == name {
			return m
		}
	}
	return nil
}

// MethodSet returns i's full method set, walking embedded
// interfaces transitively.
//
// Reading [Interface.Methods] alone reads what the source typed,
// not what the interface has. An interface embedding two others has
// their methods too, and the difference is invisible: a generated
// double missing one does not satisfy the interface it doubles, and
// a generated suite missing one asserts about a different interface
// than the one a consumer implements.
//
// # Ordering
//
// Declared methods come before embedded ones, and embeds contribute
// in declaration order, depth first. Order is part of the contract
// because generators derive field order from it, and a set that
// reordered between runs would change generated output without the
// source changing.
//
// # Duplicates
//
// A name already in the set is skipped. Go admits overlapping
// embedded sets only where the signatures agree, so the first
// occurrence is as good as any later one; taking the first means a
// declared method always wins over an embedded one of the same
// name, which matches Go's own resolution.
//
// # Constraint interfaces
//
// An interface used as a generic bound carries union terms in
// [Interface.Embeds], and those are not interfaces — they report as
// [ReasonNonInterface]. Callers walking a possibly-constraint
// interface check [IsConstraint] first; a constraint has no method
// set to resolve.
//
// A nil resolver walks nothing and reports every embed as
// [ReasonUnresolved], which is the honest answer for a caller that
// supplied no way to look one up.
func MethodSet(i *Interface, resolve InterfaceResolver) MethodSetResult {
	if i == nil {
		return MethodSetResult{}
	}
	var out MethodSetResult
	seen := map[string]struct{}{}
	visiting := map[*Interface]struct{}{}
	collect(i, resolve, seen, visiting, &out)
	return out
}

// collect appends i's declarations then walks its embeds,
// short-circuiting on an interface already on the current path.
func collect(
	i *Interface,
	resolve InterfaceResolver,
	seen map[string]struct{},
	visiting map[*Interface]struct{},
	out *MethodSetResult,
) {
	visiting[i] = struct{}{}
	defer delete(visiting, i)

	for _, m := range i.Methods {
		if m == nil || m.Name == "" {
			continue
		}
		if _, dup := seen[m.Name]; dup {
			continue
		}
		seen[m.Name] = struct{}{}
		out.Methods = append(out.Methods, m)
	}

	for _, e := range i.Embeds {
		if e == nil || e.Type == nil {
			continue
		}
		if len(e.Type.TypeArgs) > 0 {
			out.Issues = append(out.Issues, MethodSetIssue{Embed: e, Reason: ReasonGeneric})
			continue
		}
		if resolve == nil {
			out.Issues = append(out.Issues, MethodSetIssue{Embed: e, Reason: ReasonUnresolved})
			continue
		}
		embedded, found := resolve(e.Type)
		switch {
		case embedded == nil && !found:
			out.Issues = append(out.Issues, MethodSetIssue{Embed: e, Reason: ReasonUnresolved})
		case embedded == nil:
			out.Issues = append(out.Issues, MethodSetIssue{Embed: e, Reason: ReasonNonInterface})
		default:
			if _, cyclic := visiting[embedded]; cyclic {
				out.Issues = append(out.Issues, MethodSetIssue{Embed: e, Reason: ReasonCyclic})
				continue
			}
			collect(embedded, resolve, seen, visiting, out)
		}
	}
}

// IsConstraint reports whether i is a generic constraint rather
// than a method-set contract.
//
// A constraint declares type-set terms — a union of types, or an
// approximation — in the position an ordinary interface uses for
// embeds. It has no method set to double, and a generator treating
// one as an interface emits a type asserting nothing about
// anything.
//
// Detected structurally: a constraint's embeds name types rather
// than interfaces. Frontends that can tell definitively stamp their
// own metadata; this is the model-level answer available without
// one.
func IsConstraint(i *Interface) bool {
	if i == nil || len(i.Methods) > 0 {
		return false
	}
	for _, e := range i.Embeds {
		if e != nil && e.Type != nil && e.Type.IsBuiltin() {
			return true
		}
	}
	return false
}
