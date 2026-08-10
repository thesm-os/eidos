// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"strings"

	"go.thesmos.sh/eidos/node"
)

// Embedding: what an embedded type contributes to the type that
// embeds it.
//
// A generator reading `s.Fields` reads what the source typed, not
// what the struct has. `struct{ Base; Name string }` has every
// exported field of Base as well, reachable unqualified, and a
// builder offering a setter per declared field silently offers none
// for them. The same holds one declaration kind over: an interface
// embedding two others has their methods too, and a generated double
// missing one does not satisfy the interface it doubles.
//
// # Go's promotion rules, and which of them are answerable here
//
// A member promotes when it is reachable at a shallower depth than
// any other of that name. Shallowest wins; a tie at equal depth
// promotes neither, and both become unreachable without an explicit
// qualifier. A declared member always shadows a promoted one,
// because depth zero beats everything. Fields and methods follow the
// same rules, and both walks apply them.
//
// Interface embedding has none of that. A method set is a set, Go
// admits overlapping embedded sets only where the signatures agree,
// and there is nothing to shadow — so [MethodSet] takes the first
// arrival and is right to.
//
// What is not answerable without the graph is what an embedded type
// *is*: the model records a name and a package. Every function here
// that walks past the first level takes a [Resolver] and reports
// what it could not reach rather than guessing.

// maxEmbedDepth bounds the promotion walk.
//
// Go permits arbitrary embedding depth and real code rarely
// exceeds two. Eight is far past that and shallow enough that a
// cyclic graph — which no compiling source produces and a
// hand-built fixture can — terminates in a generation pass.
const maxEmbedDepth = 8

// ResolveProblem classifies why a walk could not reach something.
//
// Shared by every walk here that resolves through a [Resolver] —
// [FieldSet] and [MethodSet] over embeds, [ComparableDeep] over field
// types — because the reasons are the same three facts about the run
// whatever is being resolved, and a caller switching on them should
// not have to learn a second spelling per walk.
type ResolveProblem string

const (
	// NoResolver reports that the caller supplied no
	// [Resolver], so nothing past the first level is reachable at
	// all. Distinct from [NotLoaded] because it is a fact about
	// the call rather than about the run: the same graph answers in
	// full once a resolver is passed.
	NoResolver ResolveProblem = "no-resolver"

	// NotLoaded reports that the run never read the embed's
	// package. Resolution is against what the invocation loaded, so
	// a run over one package cannot see a type declared in another,
	// and the same source answers differently under a wider one.
	NotLoaded ResolveProblem = "not-loaded"

	// GenericEmbed reports that the embed carries type arguments.
	// Its members are typed in that declaration's type parameters
	// rather than the embedder's, so copying them across produces
	// output naming identifiers that are not in scope.
	//
	// Reported rather than substituted: [SubstituteTypeParams] can
	// do the rewrite, but only the caller knows whether the
	// substituted form is what it means to emit.
	GenericEmbed ResolveProblem = "generic"

	// TooDeep reports that the chain exceeded [maxEmbedDepth].
	// No compiling source reaches it; a cyclic hand-built graph
	// does, and this is what stops the walk rather than the run.
	TooDeep ResolveProblem = "too-deep"
)

// UnresolvedEmbed names one embed a walk could not complete.
//
// Returned rather than reported, because severity is the caller's
// policy and not a language fact: a generator that must not emit a
// partial double treats [NotLoaded] as an error and refuses to
// write anything, while one filling a documentation table treats the
// same thing as a footnote. The package has no business importing a
// diagnostic sink either — it is a leaf over the two IRs by design.
//
// An empty slice means the answer is complete. That is the whole
// contract: a caller that ignores it emits against a set that is
// smaller than the truth rather than wrong, which is the failure
// mode worth having but not one to ship silently.
type UnresolvedEmbed struct {
	// Host is the QName of the declaration that wrote the embed,
	// which is not necessarily the one the caller asked about — an
	// embed three levels down is attributed to its own embedder, so
	// a diagnostic names the file the author would have to edit.
	Host string

	// Embed is the embed as written, carried for its position so a
	// caller's diagnostic can point at the source line.
	Embed *node.Embed

	// Written is the embed spelled the way the author would
	// recognise — `io.Closer` rather than the bare `Closer` the
	// reference carries, or the full import path nobody reads. Any
	// pointer is stripped, matching [EmbedTarget]: the pointer is a
	// fact about the embedding, not about the type.
	Written string

	// Reason classifies the failure.
	Reason ResolveProblem
}

// unresolved records one embed the walk could not follow.
func unresolved(host string, e *node.Embed, reason ResolveProblem) UnresolvedEmbed {
	return UnresolvedEmbed{
		Host:    host,
		Embed:   e,
		Written: Display(EmbedTarget(e)),
		Reason:  reason,
	}
}

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

// resolveEmbed resolves an embed's target, falling back to the
// embedder's package for an in-package reference.
//
// The frontend records `type S struct{ Base }` with an empty package
// on the reference, because that is how the source reads. A resolver
// keyed by qualified name has nothing to look up until the gap is
// closed, and closing it here rather than in every resolver is what
// keeps a naive one correct.
//
// As-written is tried first, so a resolver that already handles the
// bare form keeps answering exactly as it did.
func resolveEmbed(e *node.Embed, hostPkg string, r Resolver) (node.Node, bool) {
	target := EmbedTarget(e)
	if target == nil || r == nil {
		return nil, false
	}
	if decl, found := r.Resolve(target); found {
		return decl, true
	}
	if target.Package != "" || hostPkg == "" {
		return nil, false
	}
	local := *target
	local.Package = hostPkg
	return r.Resolve(&local)
}

// descend resolves one embed for a walk, recording why it could not.
//
// The four refusals are shared by every walk here so that a caller
// reading [UnresolvedEmbed] gets the same vocabulary whichever
// function produced it, and so that a new walk cannot quietly omit
// one of them.
func descend(
	host, hostPkg string,
	e *node.Embed,
	depth int,
	r Resolver,
	problems *[]UnresolvedEmbed,
) (node.Node, bool) {
	switch {
	case depth+1 > maxEmbedDepth:
		*problems = append(*problems, unresolved(host, e, TooDeep))
		return nil, false
	case r == nil:
		*problems = append(*problems, unresolved(host, e, NoResolver))
		return nil, false
	case len(EmbedTarget(e).TypeArgs) > 0:
		*problems = append(*problems, unresolved(host, e, GenericEmbed))
		return nil, false
	}
	decl, found := resolveEmbed(e, hostPkg, r)
	if !found || nilDecl(decl) {
		*problems = append(*problems, unresolved(host, e, NotLoaded))
		return nil, false
	}
	return decl, true
}

// nilDecl reports whether a resolved declaration is a typed nil.
//
// A [Resolver] is consumer-supplied, and one answering with
// `(*node.Struct)(nil)` returns a node.Node that is not nil and
// passes every type assertion the walks make — so the first
// dereference panics inside the framework rather than in the code
// that got it wrong.
//
// Folded into [descend] rather than checked before each use: to a
// walk, a declaration it cannot read is indistinguishable from one
// the run never loaded, and both deserve the same report.
func nilDecl(n node.Node) bool {
	switch v := n.(type) {
	case *node.Struct:
		return v == nil
	case *node.Interface:
		return v == nil
	case *node.Enum:
		return v == nil
	case *node.Alias:
		return v == nil
	default:
		return n == nil
	}
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
	return selector(p.Path, p.Field.Name)
}

// PromotedMethod is one method reachable through embedding.
type PromotedMethod struct {
	// Method is the declaration itself, on whichever type declared
	// it.
	Method *node.Method

	// Depth is how many embeds were traversed to reach it. Always
	// at least one: a method the type declares itself is not
	// promoted, and shadows every promoted one of that name.
	Depth int

	// Path is the embedded field names traversed to reach it, outer
	// first.
	Path []string

	// ThroughPointer reports whether any embed on the path was by
	// pointer.
	//
	// Embedding `T` promotes T's value-receiver methods onto `S` and
	// its pointer-receiver methods onto `*S`; embedding `*T`
	// promotes both onto both. The distinction decides whether a
	// generated assertion may use the value form, and it is recorded
	// rather than resolved here because only the caller knows which
	// form it is about to emit.
	ThroughPointer bool
}

// Selector renders the explicit path to the method —
// `Base.Meta.Read` — which is what an unambiguous call needs when
// two embeds at equal depth cancelled the promoted name.
func (p PromotedMethod) Selector() string {
	return selector(p.Path, p.Method.Name)
}

// Through returns the embed the source author wrote to reach this
// member — the first hop, which is the one they can see in their own
// file. Empty only for a zero value.
func (p PromotedMethod) Through() string {
	if len(p.Path) == 0 {
		return ""
	}
	return p.Path[0]
}

// selector joins an embed path and a member name into the explicit
// Go selector that reaches it.
func selector(path []string, name string) string {
	if len(path) == 0 {
		return name
	}
	var b strings.Builder
	for _, seg := range path {
		b.WriteString(seg)
		b.WriteByte('.')
	}
	b.WriteString(name)
	return b.String()
}

// FieldSet returns every field reachable on s without an explicit
// qualifier, and every embed the walk could not complete.
//
// Go's promotion rules applied in full: a declared field shadows a
// promoted one, a shallower promotion shadows a deeper one, and two
// promotions at equal depth cancel — neither is reachable, so
// neither appears.
//
// A non-empty second result means the set is smaller than the truth.
// A generator emitting against a partial field set produces output
// missing setters rather than output naming fields that do not
// exist — but it must not treat the partial answer as complete,
// which is what the slice is for. See [UnresolvedEmbed].
//
// Order is declaration order at each level, outer levels first, so
// generated output is stable as an embedded type gains a field.
func FieldSet(s *node.Struct, r Resolver) ([]PromotedField, []UnresolvedEmbed) {
	if s == nil {
		return nil, nil
	}
	// byName accumulates every candidate for a name across depths;
	// the shadowing rules are applied once at the end, because a
	// shallower candidate may be found after a deeper one.
	byName := map[string][]PromotedField{}
	order := []string{}
	problems := []UnresolvedEmbed{}
	collectFields(s, r, 0, nil, false, byName, &order, map[string]struct{}{}, &problems)

	out := make([]PromotedField, 0, len(order))
	for _, name := range order {
		if winner, ok := shallowestUnique(byName[name], fieldDepth); ok {
			out = append(out, winner)
		}
	}
	return out, problems
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
	problems *[]UnresolvedEmbed,
) {
	// Guards a cycle. Illegal in Go — a struct cannot embed itself by
	// value — and reachable through a pointer embed or a malformed
	// graph, where it would otherwise not terminate.
	if s.QName() != "" {
		if _, looping := visited[s.QName()]; looping {
			return
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

	for _, e := range s.Embeds {
		name, byPointer := EmbedIdent(e)
		if name == "" {
			continue
		}
		// The embedded field itself is reachable by its own name, and
		// shadows anything promoted through it. Recorded before the
		// descent, so it survives an embed that cannot be followed.
		if _, seen := byName[name]; !seen {
			*order = append(*order, name)
		}
		byName[name] = append(byName[name], PromotedField{
			Field:          &node.Field{Name: name, Type: e.Type, Owner: s},
			Depth:          depth,
			Path:           path,
			ThroughPointer: throughPointer,
		})

		target, ok := descend(s.QName(), s.Package, e, depth, r, problems)
		if !ok {
			continue
		}
		inner, isStruct := target.(*node.Struct)
		if !isStruct {
			// An embedded interface contributes methods, not fields.
			// See [PromotedMethods].
			continue
		}
		collectFields(
			inner, r, depth+1, extend(path, name),
			throughPointer || byPointer, byName, order, visited, problems,
		)
	}
}

// PromotedMethods returns the method set embedding contributes to s,
// and every embed the walk could not complete.
//
// A struct embedding `io.Reader` has `Read` and declares nothing, so
// a generator asking what a struct implements has to walk the embeds
// — which is the reason `go.embedsInterface` exists as a stamp and
// this exists as the answer behind it.
//
// Promoted only: a method s declares itself is not in the result and
// shadows every promoted one of its name, because depth zero beats
// everything. Beyond that the same rules as [FieldSet] apply —
// shallowest wins, equal depth cancels — so a name reachable through
// two embeds at the same depth appears in neither Go's selector nor
// this result.
//
// An embedded interface contributes its *flattened* set, so a struct
// embedding `io.ReadCloser` has `Read` and `Close` even though
// ReadCloser declares neither. That walk is [MethodSet], and its
// unresolved embeds surface here too.
func PromotedMethods(s *node.Struct, r Resolver) ([]PromotedMethod, []UnresolvedEmbed) {
	if s == nil {
		return nil, nil
	}
	declared := map[string]struct{}{}
	for _, m := range s.Methods {
		if m != nil {
			declared[m.Name] = struct{}{}
		}
	}
	byName := map[string][]PromotedMethod{}
	order := []string{}
	problems := []UnresolvedEmbed{}
	collectMethods(
		s, r, 0, nil, false, declared, byName, &order,
		map[string]struct{}{}, &problems,
	)

	out := make([]PromotedMethod, 0, len(order))
	for _, name := range order {
		if winner, ok := shallowestUnique(byName[name], methodDepth); ok {
			out = append(out, winner)
		}
	}
	return out, problems
}

// collectMethods walks s and its embeds, recording every candidate
// for each method name.
func collectMethods(
	s *node.Struct,
	r Resolver,
	depth int,
	path []string,
	throughPointer bool,
	declared map[string]struct{},
	byName map[string][]PromotedMethod,
	order *[]string,
	visited map[string]struct{},
	problems *[]UnresolvedEmbed,
) {
	if s.QName() != "" {
		if _, looping := visited[s.QName()]; looping {
			return
		}
		visited[s.QName()] = struct{}{}
	}

	for _, e := range s.Embeds {
		name, byPointer := EmbedIdent(e)
		if name == "" {
			continue
		}
		target, ok := descend(s.QName(), s.Package, e, depth, r, problems)
		if !ok {
			continue
		}
		next := extend(path, name)
		through := throughPointer || byPointer

		record := func(m *node.Method) {
			if m == nil || m.Name == "" {
				return
			}
			// A method the struct declares itself shadows every promoted
			// one, so a candidate for that name is never a winner and
			// recording it would only make the tie count wrong.
			if _, shadowed := declared[m.Name]; shadowed {
				return
			}
			if _, seen := byName[m.Name]; !seen {
				*order = append(*order, m.Name)
			}
			byName[m.Name] = append(byName[m.Name], PromotedMethod{
				Method: m, Depth: depth + 1, Path: next, ThroughPointer: through,
			})
		}

		if iface, isIface := target.(*node.Interface); isIface {
			// An interface's own Methods are what it declared, not what
			// it has; its embeds contribute too, and a struct embedding
			// it gets the whole set.
			//
			// Through [node.MethodSet] rather than a walk of this
			// package's own. Interface embedding has no shadowing and no
			// depth rule — a method set is a set — so the second walker
			// this package used to carry shared nothing with the
			// promotion rules around it except a cycle guard, and the
			// two guards disagreed about a type-set term.
			set := node.MethodSet(iface, interfaceResolver(r))
			for _, iss := range set.Issues {
				*problems = append(*problems, fromIssue(iface, iss))
			}
			for _, m := range set.Methods {
				record(m)
			}
			continue
		}

		for _, m := range methodsOfDecl(target) {
			record(m)
		}
		if inner, isStruct := target.(*node.Struct); isStruct {
			collectMethods(
				inner, r, depth+1, next, through, declared,
				byName, order, visited, problems,
			)
		}
	}
}

// interfaceResolver adapts this package's [Resolver] to the callback
// [node.MethodSet] takes.
//
// The model's walk asks a narrower question than a general resolver
// answers — "is this an interface, and did you find it" — and the two
// nil-and-false combinations it distinguishes are what let it tell a
// struct in embed position from a package this run never read.
func interfaceResolver(r Resolver) node.InterfaceResolver {
	return func(t *node.TypeRef) (*node.Interface, bool) {
		if r == nil {
			return nil, false
		}
		decl, found := r.Resolve(t)
		if !found {
			return nil, false
		}
		iface, isIface := decl.(*node.Interface)
		if !isIface || iface == nil {
			// Found, and not an interface: the model's walk reads the
			// pair as ReasonNonInterface, which is a defect in the
			// source rather than a narrow run.
			return nil, true
		}
		return iface, true
	}
}

// fromIssue translates the model's vocabulary into this package's.
//
// Two vocabularies for one fact is what let the type-set workaround
// diverge between them; this is the single crossing, so a reason
// added upstream surfaces here rather than silently mapping to the
// nearest existing one.
func fromIssue(host *node.Interface, iss node.MethodSetIssue) UnresolvedEmbed {
	reason := NotLoaded
	switch iss.Reason {
	case node.ReasonGeneric:
		reason = GenericEmbed
	case node.ReasonCyclic:
		reason = TooDeep
	case node.ReasonUnresolved, node.ReasonNonInterface:
		reason = NotLoaded
	}
	return UnresolvedEmbed{
		Host:    host.QName(),
		Embed:   iss.Embed,
		Written: Display(EmbedTarget(iss.Embed)),
		Reason:  reason,
	}
}

// PromotedFields returns only the fields reached through embedding
// — [FieldSet] minus what the struct declared itself.
//
// For a generator that treats the two differently: a builder
// offering a setter per declared field and one whole setter for
// each embedded value needs to tell them apart, and a struct
// literal sets an embedded value as a unit.
func PromotedFields(s *node.Struct, r Resolver) ([]PromotedField, []UnresolvedEmbed) {
	all, problems := FieldSet(s, r)
	out := make([]PromotedField, 0, len(all))
	for _, f := range all {
		if f.Depth > 0 {
			out = append(out, f)
		}
	}
	return out, problems
}

// ExportedFieldSet is [FieldSet] restricted to fields a generated
// file in another package can name.
//
// The set a builder, a mock or a fixture can actually set:
// unexported fields are visible to the declaring package and to
// nothing else, and a generator routed elsewhere that emitted a
// setter for one produces a file that does not compile.
func ExportedFieldSet(s *node.Struct, r Resolver) ([]PromotedField, []UnresolvedEmbed) {
	all, problems := FieldSet(s, r)
	out := make([]PromotedField, 0, len(all))
	for _, f := range all {
		if IsExported(f.Field.Name) {
			out = append(out, f)
		}
	}
	return out, problems
}

// shallowestUnique applies Go's promotion rules to one name's
// candidates.
//
// The shallowest wins; a tie at that depth promotes neither, since
// Go makes an ambiguous selector an error rather than a choice.
//
// Generic over the candidate type because the rule turns on depth
// alone, and fields and methods obey it identically — a second copy
// keyed to the other type is a second chance to get the tie wrong.
//
// Requires at least one candidate. Both callers iterate an order
// slice that only gains a name when the candidate map gains its
// first entry for it, so an empty list cannot arise and a guard for
// one would be a line no reader can account for.
func shallowestUnique[T any](candidates []T, depthOf func(T) int) (T, bool) {
	var zero T
	best := candidates[0]
	ties := 1
	for _, c := range candidates[1:] {
		switch {
		case depthOf(c) < depthOf(best):
			best, ties = c, 1
		case depthOf(c) == depthOf(best):
			ties++
		}
	}
	if ties > 1 {
		return zero, false
	}
	return best, true
}

// fieldDepth and methodDepth are the depth accessors
// [shallowestUnique] is keyed on.
func fieldDepth(p PromotedField) int   { return p.Depth }
func methodDepth(p PromotedMethod) int { return p.Depth }

// extend appends one segment to an embed path without aliasing the
// caller's slice.
//
// # Allocation
//
// Always allocates. The walk hands the same path to every candidate
// it records at a level, so a shared backing array would let a
// sibling embed's append overwrite a path already stored on a
// result.
func extend(path []string, segment string) []string {
	out := make([]string, 0, len(path)+1)
	out = append(out, path...)
	return append(out, segment)
}

// methodsOfDecl returns the methods a declaration carries.
//
// A type switch because the model puts the list on each kind rather
// than behind an accessor, and a caller resolving an embed does not
// know which kind it reached.
//
// No interface case: an interface's own Methods are what it declared
// rather than what it has, so [collectMethods] routes one through
// [MethodSet] before reaching here.
func methodsOfDecl(n node.Node) []*node.Method {
	switch v := n.(type) {
	case *node.Struct:
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
	return embedsType(s, qname, r, map[string]struct{}{})
}

// embedsType is [EmbedsType] with the cycle guard threaded through,
// so a malformed graph terminates the search rather than the run.
func embedsType(s *node.Struct, qname string, r Resolver, visited map[string]struct{}) bool {
	if s == nil {
		return false
	}
	if s.QName() != "" {
		if _, looping := visited[s.QName()]; looping {
			return false
		}
		visited[s.QName()] = struct{}{}
	}
	for _, e := range s.Embeds {
		if QName(EmbedTarget(e)) == qname {
			return true
		}
		decl, found := resolveEmbed(e, s.Package, r)
		if !found {
			continue
		}
		inner, isStruct := decl.(*node.Struct)
		if isStruct && embedsType(inner, qname, r, visited) {
			return true
		}
	}
	return false
}
