// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape

// ParamKind says how the refinement resolver treats a directive
// parameter's value.
//
// The kind is a property of the key, so it is declared beside the key
// rather than in a parallel list. An earlier form carried the keys in
// one slice and each resolvable subset in another, which meant every
// key was declared twice and nothing checked the two agreed: a key
// named as a sibling but omitted from the key list resolved correctly
// and stayed invisible to every [Contract.Validate] and
// [Mixin.Validate] hook, because those read the key list alone.
type ParamKind uint8

const (
	// KindOpaque leaves the value verbatim. The right kind for
	// anything with no declaration to resolve against — a literal, a
	// quantity, an encoded graph. A field of the host's value type is
	// [KindValueField] and a parameter of the host is [KindParam];
	// both were opaque before those kinds existed, which is why a
	// key here named a member the resolver never checked.
	KindOpaque ParamKind = iota

	// KindCallable names a sibling callable, resolved through the
	// host's own scope: an owner's method set for a method, the
	// package's functions for a function.
	KindCallable

	// KindVar names a package-level var, resolved through the
	// package. A sentinel is not declared on the receiver, so this
	// scope is the package's for a method host as much as a function
	// one.
	KindVar

	// KindMember names a member of the type the host answers — a
	// method on the handle a role's callable returns. Resolved
	// through that type's own declaration, which the resolver can
	// only see when the run loaded it.
	KindMember

	// KindValueField names a field of the host's value type — the
	// ordering stamp a session guarantee reads, the field a
	// compare-and-swap guards.
	//
	// # The scope rule, stated
	//
	// Stated rather than left to the implementation, because at
	// least one consumer has to reproduce it to use the stamp at
	// all, and a rule that is only implemented gets re-derived by
	// hand until the two disagree:
	//
	//   - The value type is the host's first non-error result.
	//   - A host answering nothing resolves against each
	//     non-context parameter's type in declaration order, taking
	//     the first that declares the field — the written value.
	//   - Both positions are pointer-stripped before lookup.
	//
	// The field must be exported, since every consumer reaches it
	// from outside the declaring package, and promoted fields count:
	// an embedded base carrying the version stamp is exactly the
	// shape this kind names, and a flat match would report a correct
	// directive as wrong.
	//
	// A field, never a method — the distinction from [KindMember],
	// and the reason this is its own constant: a stamp is assigned
	// as well as read, and no method form can sit on the left of
	// that assignment. When the run did not load the value type's
	// declaration the param stamps unvalidated, the same rule
	// KindMember documents.
	KindValueField

	// KindParam names a parameter of the host callable itself — the
	// axis a partition check varies, the key that pins a sticky
	// instance.
	//
	// Validated against the host's own signature rather than
	// resolved into a qualified name: a parameter has no
	// package-level spelling, so the stamp stays as the author wrote
	// it and the check is that the name is genuinely in the
	// signature. Before this kind existed the same check lived in
	// per-mixin Validate hooks, where forgetting it was
	// indistinguishable from the param being free-form.
	KindParam
)

// String renders the kind for diagnostics.
func (k ParamKind) String() string {
	switch k {
	case KindOpaque:
		return "opaque"
	case KindCallable:
		return "callable"
	case KindVar:
		return "var"
	case KindMember:
		return "member"
	case KindValueField:
		return "value-field"
	case KindParam:
		return "param"
	default:
		return "unknown"
	}
}

// scopeNoun renders what a kind's value must name, for the resolver's
// "names no <noun>" diagnostic. Distinct from [ParamKind.String],
// which is the bare token a reader wants when the kind is the subject
// rather than the object of a sentence.
func (k ParamKind) scopeNoun() string {
	switch k {
	case KindCallable:
		return "callable in scope"
	case KindVar:
		return "package-level var in scope"
	case KindMember:
		return "member of the type it answers"
	case KindValueField:
		return "field of its value type"
	case KindParam:
		return "parameter of the annotated callable"
	case KindOpaque:
		return "resolvable declaration"
	default:
		return "resolvable declaration"
	}
}

// Param is one KV key a directive accepts, with how its value
// resolves.
//
// Declaring the kind here rather than in a parallel list is what makes
// the key's treatment single-sourced: a reader of the declaration sees
// what the value must name, and a fifth kind costs a constant rather
// than a field on two schemas.
type Param struct {
	// Key is the KV key as written in the directive.
	Key string

	// Kind says what the value names and therefore how it resolves.
	// The zero value is [KindOpaque], so a param needing no
	// resolution may leave it unset.
	Kind ParamKind

	// Role scopes the param to directives hosted by that role. The
	// zero value applies it to every role, which is what a param
	// declared before this field existed means.
	//
	// The field exists because one key can carry two meanings in one
	// contract, chosen by the role hosting the directive. A cursor's
	// `next` is a role on the standalone arm — a sibling callable
	// beside Close — and a member of the handle on the producer arm,
	// where Scan answers a Cursor whose Next lives on the returned
	// type. Without the scope the two collide: the key is either
	// always a param, which breaks the standalone arm's partner
	// reference, or never one, which reports a correct producer
	// directive as naming a partner that is not in scope.
	//
	// Meaningless on a [Mixin], which has no role vocabulary. The
	// mixin test helper rejects a non-empty value rather than
	// ignoring it, because a scope that silently never applies is
	// the failure mode this field was added to remove.
	Role string

	// Required marks a key the directive must carry for the
	// classification to state its own sentence — the validator
	// reports a host whose folded stamps hold no value for it.
	//
	// A property of the key, declared beside the key, on the same
	// argument [ParamKind] makes above: a parallel required-list
	// would be the two-slices shape that has to be kept in
	// agreement by hand. It is not a value check — a key present
	// with a malformed value is [Mixin.Validate]'s to judge, as
	// `retrysucceeds` does — and an empty value counts as absent,
	// matching the stamping pass, which never stamps one.
	//
	// Enforced over the host's folded stamps rather than per
	// directive line, so a declaration split across lines — one
	// naming `fn=`, the next `unready=` — assembles one complete
	// attachment rather than two incomplete ones.
	//
	// Where [Param.Role] is set, required binds only on directives
	// that role hosts: a key the producer arm must carry says
	// nothing about the reader arm. The zero value keeps today's
	// behaviour — absence is legal, and whether a check is worth
	// emitting without the key belongs to the consumer.
	Required bool
}

// ParamsForRole returns the params that apply to a directive hosted
// by role, in declaration order.
//
// The single home for "which params apply". Two passes need the
// answer — the stamping pass deciding whether a KV key is a param or
// a partner reference, and the resolver choosing a scope for it — and
// a contract whose key means different things under different roles
// is exactly the case where two implementations of the predicate
// would agree only until someone declared the disagreement.
//
// An empty role matches only the unscoped params: a directive with no
// role stamps nothing role-specific, so nothing role-scoped applies.
func ParamsForRole(params []Param, role string) []Param {
	var out []Param
	for _, p := range params {
		if p.Role == "" || p.Role == role {
			out = append(out, p)
		}
	}
	return out
}

// ParamKeys returns the declared keys in declaration order.
//
// The order is the declaration's, not sorted: a diagnostic listing
// accepted keys reads best in the order the author wrote them, and
// resolution visits them in the same order so two runs report the same
// failure first.
func ParamKeys(params []Param) []string {
	if len(params) == 0 {
		return nil
	}
	out := make([]string, 0, len(params))
	for _, p := range params {
		out = append(out, p.Key)
	}
	return out
}

// ParamKeysOfKind returns the declared keys of one kind, in
// declaration order.
func ParamKeysOfKind(params []Param, kind ParamKind) []string {
	var out []string
	for _, p := range params {
		if p.Kind == kind {
			out = append(out, p.Key)
		}
	}
	return out
}
