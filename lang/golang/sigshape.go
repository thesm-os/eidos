// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"slices"

	"go.thesmos.sh/eidos/node"
)

// The well-known Go method shapes — the signatures the standard
// library and its conventions give meaning to.
//
// A generator asking "does this type implement error" is asking
// about a signature, not a name, and the check is three or four
// conditions with a nil guard on each. Written out per generator it
// is where the copies disagree: one that reads the return's binding
// name instead of its type classifies nothing at all and reports no
// diagnostic, because every real `Error() string` is written
// anonymously.

// Method name constants for the shapes below. Exported because a
// generator emitting one of these methods names it, and the name
// the emitter writes and the name the detector looks for have to be
// the same string.
const (
	MethodError           = "Error"
	MethodUnwrap          = "Unwrap"
	MethodIs              = "Is"
	MethodAs              = "As"
	MethodString          = "String"
	MethodWrite           = "Write"
	MethodRead            = "Read"
	MethodClose           = "Close"
	MethodMarshalText     = "MarshalText"
	MethodUnmarshalText   = "UnmarshalText"
	MethodMarshalJSON     = "MarshalJSON"
	MethodUnmarshalJSON   = "UnmarshalJSON"
	MethodMarshalBinary   = "MarshalBinary"
	MethodUnmarshalBinary = "UnmarshalBinary"
	MethodGobEncode       = "GobEncode"
	MethodGobDecode       = "GobDecode"

	MethodScan     = "Scan"
	MethodValue    = "Value"
	MethodLen      = "Len"
	MethodLess     = "Less"
	MethodSwap     = "Swap"
	MethodEqual    = "Equal"
	MethodCompare  = "Compare"
	MethodClone    = "Clone"
	MethodReset    = "Reset"
	MethodValidate = "Validate"
)

// IsErrorMethod reports whether m is `Error() string`.
func IsErrorMethod(m *node.Method) bool {
	return named(m, MethodError) && arity(m, 0, 1) && IsString(returnType(m, 0))
}

// IsUnwrapMethod reports whether m is `Unwrap() error` or
// `Unwrap() []error`.
//
// Both spellings, because Go 1.20 added the multi-error form and a
// type using it is unwrappable in exactly the sense a caller cares
// about. Recognising only the single form silently classifies a
// joined error as unwrappable-by-nothing.
func IsUnwrapMethod(m *node.Method) bool {
	if !named(m, MethodUnwrap) || !arity(m, 0, 1) {
		return false
	}
	r := returnType(m, 0)
	return IsError(r) || IsError(SliceElem(r))
}

// IsIsMethod reports whether m is `Is(error) bool` — the hook
// [errors.Is] consults.
func IsIsMethod(m *node.Method) bool {
	return named(m, MethodIs) && arity(m, 1, 1) &&
		IsError(paramType(m, 0)) && IsBool(returnType(m, 0))
}

// IsAsMethod reports whether m is `As(any) bool` — the hook
// [errors.As] consults.
func IsAsMethod(m *node.Method) bool {
	return named(m, MethodAs) && arity(m, 1, 1) &&
		IsAny(paramType(m, 0)) && IsBool(returnType(m, 0))
}

// IsStringMethod reports whether m is `String() string` —
// [fmt.Stringer].
//
// Distinct from [IsStringer], which reads the frontend's stamp on a
// type reference. This asks about a declaration a generator can
// see; that asks about a type it cannot resolve.
func IsStringMethod(m *node.Method) bool {
	return named(m, MethodString) && arity(m, 0, 1) && IsString(returnType(m, 0))
}

// IsWriteMethod reports whether m is `Write([]byte) (int, error)` —
// [io.Writer].
func IsWriteMethod(m *node.Method) bool {
	return named(m, MethodWrite) && byteCountErr(m)
}

// IsReadMethod reports whether m is `Read([]byte) (int, error)` —
// [io.Reader].
func IsReadMethod(m *node.Method) bool {
	return named(m, MethodRead) && byteCountErr(m)
}

// IsCloseMethod reports whether m is `Close() error` — [io.Closer].
func IsCloseMethod(m *node.Method) bool {
	return named(m, MethodClose) && arity(m, 0, 1) && IsError(returnType(m, 0))
}

// IsMarshalText reports whether m is
// `MarshalText() ([]byte, error)` — [encoding.TextMarshaler].
func IsMarshalText(m *node.Method) bool {
	return named(m, MethodMarshalText) && bytesErr(m)
}

// IsUnmarshalText reports whether m is `UnmarshalText([]byte) error`
// — [encoding.TextUnmarshaler].
func IsUnmarshalText(m *node.Method) bool {
	return named(m, MethodUnmarshalText) && bytesInErr(m)
}

// IsMarshalJSON reports whether m is
// `MarshalJSON() ([]byte, error)` — [json.Marshaler].
func IsMarshalJSON(m *node.Method) bool {
	return named(m, MethodMarshalJSON) && bytesErr(m)
}

// IsUnmarshalJSON reports whether m is `UnmarshalJSON([]byte) error`
// — [json.Unmarshaler].
func IsUnmarshalJSON(m *node.Method) bool {
	return named(m, MethodUnmarshalJSON) && bytesInErr(m)
}

// IsMarshalBinary reports whether m is
// `MarshalBinary() ([]byte, error)` — [encoding.BinaryMarshaler].
//
// The codec family Go's own `time.Time` and `net.IP` implement, and
// the one `gob` and `encoding/json` reach for after the text form.
// A generator asserting a type round-trips has to know which codecs
// it declares, and a table that stops at JSON reports a
// binary-only type as encoding nothing.
func IsMarshalBinary(m *node.Method) bool {
	return named(m, MethodMarshalBinary) && bytesErr(m)
}

// IsUnmarshalBinary reports whether m is
// `UnmarshalBinary([]byte) error` — [encoding.BinaryUnmarshaler].
func IsUnmarshalBinary(m *node.Method) bool {
	return named(m, MethodUnmarshalBinary) && bytesInErr(m)
}

// IsGobEncode reports whether m is `GobEncode() ([]byte, error)` —
// [gob.GobEncoder].
//
// Distinct from the binary codec despite the identical signature:
// `gob` prefers its own pair and falls back to the binary one, so a
// type declaring both encodes differently through each and a
// generator asserting a round trip has to name which it drove.
func IsGobEncode(m *node.Method) bool {
	return named(m, MethodGobEncode) && bytesErr(m)
}

// IsGobDecode reports whether m is `GobDecode([]byte) error` —
// [gob.GobDecoder].
func IsGobDecode(m *node.Method) bool {
	return named(m, MethodGobDecode) && bytesInErr(m)
}

// IsScanMethod reports whether m is `Scan(any) error` —
// [sql.Scanner].
//
// The database read half. Its partner [IsValuerMethod] cannot be
// matched on a builtin: `driver.Value` is a qualified alias, so the
// two halves of one interface pair are checked differently.
func IsScanMethod(m *node.Method) bool {
	return named(m, MethodScan) && arity(m, 1, 1) &&
		IsAny(paramType(m, 0)) && IsError(returnType(m, 0))
}

// IsValuerMethod reports whether m is
// `Value() (driver.Value, error)` — [driver.Valuer].
//
// Matched on the qualified type rather than on `any`, even though
// `driver.Value` is an alias for it: a frontend records the written
// spelling, and a type declaring `Value() (any, error)` is a
// different API that happens to be assignable.
func IsValuerMethod(m *node.Method) bool {
	if !named(m, MethodValue) || !arity(m, 0, 2) {
		return false
	}
	return isQualified(returnType(m, 0), "database/sql/driver", "Value") &&
		IsError(returnType(m, 1))
}

// IsLenMethod reports whether m is `Len() int` — the first third of
// [sort.Interface], and the shape a generated length assertion
// drives.
func IsLenMethod(m *node.Method) bool {
	return named(m, MethodLen) && arity(m, 0, 1) &&
		IsBuiltinNamed(returnType(m, 0), typeInt)
}

// IsLessMethod reports whether m is `Less(i, j int) bool`.
func IsLessMethod(m *node.Method) bool {
	return named(m, MethodLess) &&
		SignatureMatches(m, []string{typeInt, typeInt}, []string{typeBool})
}

// IsSwapMethod reports whether m is `Swap(i, j int)`.
func IsSwapMethod(m *node.Method) bool {
	return named(m, MethodSwap) &&
		SignatureMatches(m, []string{typeInt, typeInt}, nil)
}

// ImplementsSorter reports whether the method set declares all
// three of [sort.Interface].
//
// All three or none: a type carrying two of them satisfies nothing,
// and a generator emitting a sort against it produces a file that
// does not compile.
func ImplementsSorter(methods []*node.Method) bool {
	return anyMethod(methods, IsLenMethod) &&
		anyMethod(methods, IsLessMethod) &&
		anyMethod(methods, IsSwapMethod)
}

// IsEqualMethod reports whether m is `Equal(T) bool` for any single
// parameter.
//
// The parameter is unconstrained because the convention is
// self-typed — `func (t Time) Equal(u Time) bool` — and this
// package cannot compare a receiver it was not given. A caller that
// holds the owner checks the parameter against it.
func IsEqualMethod(m *node.Method) bool {
	return named(m, MethodEqual) && arity(m, 1, 1) && IsBool(returnType(m, 0))
}

// IsCompareMethod reports whether m is `Compare(T) int` — the
// ordering convention [cmp.Compare] and [slices.SortFunc] consume.
func IsCompareMethod(m *node.Method) bool {
	return named(m, MethodCompare) && arity(m, 1, 1) &&
		IsBuiltinNamed(returnType(m, 0), typeInt)
}

// IsCloneMethod reports whether m is `Clone() T` — the deep-copy
// convention, taking nothing and returning one value.
//
// Worth recognising because a generated independence check needs
// one: a builder that clones has to prove the copy shares no
// storage with its source, and a type that clones itself is the
// only one that can say how.
func IsCloneMethod(m *node.Method) bool {
	return named(m, MethodClone) && arity(m, 0, 1)
}

// IsResetMethod reports whether m is `Reset()` — the pool and
// buffer convention, taking nothing and returning nothing.
func IsResetMethod(m *node.Method) bool {
	return named(m, MethodReset) && arity(m, 0, 0)
}

// IsValidateMethod reports whether m is `Validate() error`.
//
// Not a standard-library interface but a near-universal
// convention, and the one a generated constructor calls before
// returning a value it built.
func IsValidateMethod(m *node.Method) bool {
	return named(m, MethodValidate) && ReturnsOnly(m, typeError)
}

// Codecs reports which encoding pairs a method set declares
// completely.
//
// Complete pairs only: a type declaring `MarshalJSON` without
// `UnmarshalJSON` does not round-trip, and a generated check
// asserting that it does fails against code that never claimed to.
// The result is ordered so generated output is byte-stable.
func Codecs(methods []*node.Method) []string {
	pairs := []struct {
		name             string
		marshal, unmarsh func(*node.Method) bool
	}{
		{"text", IsMarshalText, IsUnmarshalText},
		{"json", IsMarshalJSON, IsUnmarshalJSON},
		{"binary", IsMarshalBinary, IsUnmarshalBinary},
		{"gob", IsGobEncode, IsGobDecode},
		{"sql", IsValuerMethod, IsScanMethod},
	}
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		if anyMethod(methods, p.marshal) && anyMethod(methods, p.unmarsh) {
			out = append(out, p.name)
		}
	}
	return out
}

// isQualified reports whether t names the given package-qualified
// type.
func isQualified(t *node.TypeRef, pkg, name string) bool {
	return t != nil && t.Package == pkg && t.Name == name
}

// ImplementsError reports whether the method set declares
// `Error() string`.
//
// Takes the method list rather than the declaration holding it,
// because a struct, an interface and an alias all carry one and the
// question is the same for each — keying on the container is what
// leaves a generator with a private copy per kind.
//
// Pass a resolved set where the type embeds: a method promoted from
// an embedded type is part of the method set and absent from the
// declarations. See [node.MethodSet].
func ImplementsError(methods []*node.Method) bool {
	return anyMethod(methods, IsErrorMethod)
}

// ImplementsStringer reports whether the method set declares
// `String() string`.
func ImplementsStringer(methods []*node.Method) bool {
	return anyMethod(methods, IsStringMethod)
}

// ImplementsWriter reports whether the method set declares
// `Write([]byte) (int, error)`.
func ImplementsWriter(methods []*node.Method) bool {
	return anyMethod(methods, IsWriteMethod)
}

// ImplementsReader reports whether the method set declares
// `Read([]byte) (int, error)`.
func ImplementsReader(methods []*node.Method) bool {
	return anyMethod(methods, IsReadMethod)
}

// ReturnsOnly reports whether m returns exactly the named builtin
// and takes no parameters.
//
// The generalisation behind the single-return shapes above, for a
// convention this package does not know about — a project's own
// `Validate() error`, say.
func ReturnsOnly(m *node.Method, typeName string) bool {
	return arity(m, 0, 1) && IsBuiltinNamed(returnType(m, 0), typeName)
}

// SignatureMatches reports whether m's parameter and return types
// spell the given builtin names, in order.
//
// The escape hatch for a shape with no entry above. Only builtins
// are expressible, which is the limit that keeps it honest: a
// qualified type needs a package to match against, and a caller
// with one should compare the reference itself.
func SignatureMatches(m *node.Method, params, returns []string) bool {
	if m == nil || len(m.Params) != len(params) || len(m.Returns) != len(returns) {
		return false
	}
	for i, want := range params {
		if !IsBuiltinNamed(paramType(m, i), want) {
			return false
		}
	}
	for i, want := range returns {
		if !IsBuiltinNamed(returnType(m, i), want) {
			return false
		}
	}
	return true
}

// byteCountErr matches `([]byte) (int, error)` — the io read/write
// signature both directions share.
func byteCountErr(m *node.Method) bool {
	return arity(m, 1, 2) && IsByteSliceAny(paramType(m, 0)) &&
		IsBuiltinNamed(returnType(m, 0), typeInt) && IsError(returnType(m, 1))
}

// bytesErr matches `() ([]byte, error)` — the marshal direction.
func bytesErr(m *node.Method) bool {
	return arity(m, 0, 2) && IsByteSliceAny(returnType(m, 0)) && IsError(returnType(m, 1))
}

// bytesInErr matches `([]byte) error` — the unmarshal direction.
func bytesInErr(m *node.Method) bool {
	return arity(m, 1, 1) && IsByteSliceAny(paramType(m, 0)) && IsError(returnType(m, 0))
}

// IsByteSliceAny reports whether t is `[]byte` in either element
// spelling.
//
// Distinct from [IsByteSlice], which additionally requires the
// frontend's byte stamp for its template-branching role. A
// signature check wants the structural answer and both spellings:
// `[]uint8` is the same type an author may have written either way.
func IsByteSliceAny(t *node.TypeRef) bool {
	return t != nil && t.TypeKind == node.TypeRefSlice && IsByte(t.Elem)
}

// named reports whether m is non-nil and carries the given name.
func named(m *node.Method, name string) bool {
	return m != nil && m.Name == name
}

// arity reports whether m declares exactly the given counts.
func arity(m *node.Method, params, returns int) bool {
	return m != nil && len(m.Params) == params && len(m.Returns) == returns
}

// paramType returns the declared type of parameter i, or nil.
func paramType(m *node.Method, i int) *node.TypeRef {
	if m == nil || i >= len(m.Params) || m.Params[i] == nil {
		return nil
	}
	return m.Params[i].Type
}

// returnType returns the declared type of return slot i, or nil.
//
// The slot rather than its binding name: [node.Return] carries both
// and they spell the same field, which is how a classifier reading
// the wrong one compiles and matches nothing.
func returnType(m *node.Method, i int) *node.TypeRef {
	if m == nil || i >= len(m.Returns) || m.Returns[i] == nil {
		return nil
	}
	return m.Returns[i].Type
}

// anyMethod reports whether any method satisfies pred.
func anyMethod(methods []*node.Method, pred func(*node.Method) bool) bool {
	return slices.ContainsFunc(methods, pred)
}
