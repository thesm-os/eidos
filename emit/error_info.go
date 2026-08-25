// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

// The vocabulary a generator carries when the declaration takes part
// in the language's error protocol.
//
// Every language has one and no two spell it alike: a method
// returning text, a class inheriting a base, an interface a value
// conforms to. What a generator asserting an error contract needs is
// the same in each — whether the value carries the protocol, whether
// it compares itself, whether it names a cause, and which of its
// members a message is expected to mention.

// ErrorInfo is what a language answers about one declaration that
// takes part in its error protocol.
type ErrorInfo struct {
	// Addressed reports that a value must be addressed before it
	// carries the contract, which decides how a check constructs its
	// subject.
	//
	// Go declares the protocol on either the value or its pointer,
	// and a check building the wrong one asserts against a value that
	// does not implement it — which the consumer's compiler reports,
	// against a file they did not write.
	Addressed bool

	// Compares reports that the declaration answers equality against
	// another error itself, rather than leaving it to identity.
	//
	// Earns a check, and its absence earns none: a type that does not
	// declare the comparison has nothing to assert about it beyond
	// what the language already guarantees.
	Compares bool

	// Unwraps reports that the declaration names the error it wraps.
	Unwraps bool

	// Cause is the member holding the wrapped error, empty when the
	// declaration carries none.
	//
	// Wider than [ErrorInfo.Members]: a check assigns the cause
	// through whatever selector reaches it, which promotion makes
	// legal at any depth — while a literal's key is not a selector,
	// so a promoted member cannot be set that way. The asymmetry is
	// what lets a family of errors sharing one embedded base be
	// checked at all.
	Cause string

	// Members are the members a message is expected to mention, in
	// declaration order.
	//
	// Narrower than the declaration's full member set: only the ones
	// a literal in the generated file can set directly.
	Members []ErrorMember

	// Unresolved names the parts of the declaration the run could not
	// reach, empty when the walk completed.
	//
	// A non-empty list means the projection is smaller than the
	// truth, so a generator asserting against it quietly claims a
	// contract the declaration may not have — or omits one it does.
	// The usual cause is a run narrower than the declaration, which
	// the author can fix and the generator cannot.
	Unresolved []string
}

// ErrorMember is one member of an error declaration, with what a
// check can write into it.
type ErrorMember struct {
	// Name is the member's identifier.
	Name string

	// Sample is the value a check writes. Ask [Sample.OK] rather than
	// comparing against the zero value.
	Sample Sample

	// Verbatim reports that a message carries this value's text
	// unchanged, so a check may assert the text appears in it.
	//
	// False for a value a message formats: the width, base and
	// precision a format applies are not visible to the projection,
	// so asserting that `42` appears in the output fails against a
	// declaration that reports the same number perfectly well as
	// `042`.
	Verbatim bool
}

// Writable reports whether any member carries a value a check can
// write, which is what decides whether the message check is worth
// emitting.
func (e ErrorInfo) Writable() bool {
	for _, m := range e.Members {
		if m.Sample.OK() {
			return true
		}
	}
	return false
}
