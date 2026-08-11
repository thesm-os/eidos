// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// method builds a method from its name, parameter types and return
// types, all anonymous.
//
// Anonymous deliberately: every `Error() string` in the wild is
// written that way, and a classifier that reads the binding name
// instead of the type matches none of them.
func method(name string, params, returns []*node.TypeRef) *node.Method {
	m := &node.Method{Name: name}
	for _, p := range params {
		m.Params = append(m.Params, &node.Param{Type: p})
	}
	for _, r := range returns {
		m.Returns = append(m.Returns, &node.Return{Type: r})
	}
	return m
}

// byteSlice returns the `[]byte` reference.
func byteSlice() *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: builtinRef("byte")}
}

func TestErrorShapes(t *testing.T) {
	t.Parallel()

	t.Run("an anonymous Error() string is recognised", func(t *testing.T) {
		t.Parallel()
		// The whole point: reading the return's binding name instead
		// of its type compiles and classifies nothing, because the
		// canonical spelling names no result.
		m := method("Error", nil, []*node.TypeRef{builtinRef("string")})
		if !golang.IsErrorMethod(m) {
			t.Fatalf("Error() string must be recognised")
		}
	})

	t.Run("Error returning the wrong type is rejected", func(t *testing.T) {
		t.Parallel()
		m := method("Error", nil, []*node.TypeRef{builtinRef("int")})
		if golang.IsErrorMethod(m) {
			t.Fatalf("Error() int must not be recognised")
		}
	})

	t.Run("Error taking arguments is a different method", func(t *testing.T) {
		t.Parallel()
		m := method("Error", []*node.TypeRef{builtinRef("int")}, []*node.TypeRef{builtinRef("string")})
		if golang.IsErrorMethod(m) {
			t.Fatalf("Error(int) string must not be recognised")
		}
	})

	t.Run("both Unwrap spellings are recognised", func(t *testing.T) {
		t.Parallel()
		// Go 1.20 added the multi-error form; recognising only the
		// single one classifies a joined error as unwrappable by
		// nothing.
		single := method("Unwrap", nil, []*node.TypeRef{errorRef()})
		multi := method("Unwrap", nil, []*node.TypeRef{
			{TypeKind: node.TypeRefSlice, Elem: errorRef()},
		})
		if !golang.IsUnwrapMethod(single) {
			t.Fatalf("Unwrap() error must be recognised")
		}
		if !golang.IsUnwrapMethod(multi) {
			t.Fatalf("Unwrap() []error must be recognised")
		}
	})

	t.Run("recognises the errors.Is hook", func(t *testing.T) {
		t.Parallel()
		m := method("Is", []*node.TypeRef{errorRef()}, []*node.TypeRef{builtinRef("bool")})
		if !golang.IsIsMethod(m) {
			t.Fatalf("Is(error) bool must be recognised")
		}
	})

	t.Run("recognises the errors.As hook in both any spellings", func(t *testing.T) {
		t.Parallel()
		bare := method("As", []*node.TypeRef{builtinRef("any")}, []*node.TypeRef{builtinRef("bool")})
		inline := method("As",
			[]*node.TypeRef{{TypeKind: node.TypeRefAnonInterface}},
			[]*node.TypeRef{builtinRef("bool")})
		if !golang.IsAsMethod(bare) || !golang.IsAsMethod(inline) {
			t.Fatalf("As(any) bool must be recognised in both spellings")
		}
	})
}

func TestIOShapes(t *testing.T) {
	t.Parallel()

	countErr := []*node.TypeRef{builtinRef("int"), errorRef()}

	t.Run("recognises Write and Read", func(t *testing.T) {
		t.Parallel()
		if !golang.IsWriteMethod(method("Write", []*node.TypeRef{byteSlice()}, countErr)) {
			t.Fatalf("Write([]byte) (int, error) must be recognised")
		}
		if !golang.IsReadMethod(method("Read", []*node.TypeRef{byteSlice()}, countErr)) {
			t.Fatalf("Read([]byte) (int, error) must be recognised")
		}
	})

	t.Run("accepts the uint8 spelling of the byte slice", func(t *testing.T) {
		t.Parallel()
		// The frontend records whichever the author wrote, and a
		// check on one spelling misses the other.
		u8 := &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: builtinRef("uint8")}
		if !golang.IsWriteMethod(method("Write", []*node.TypeRef{u8}, countErr)) {
			t.Fatalf("Write([]uint8) must be recognised")
		}
	})

	t.Run("recognises Close", func(t *testing.T) {
		t.Parallel()
		if !golang.IsCloseMethod(method("Close", nil, []*node.TypeRef{errorRef()})) {
			t.Fatalf("Close() error must be recognised")
		}
	})

	t.Run("a same-named method of another shape is rejected", func(t *testing.T) {
		t.Parallel()
		// Reading the types is what keeps the check a check.
		if golang.IsWriteMethod(method("Write", []*node.TypeRef{builtinRef("string")}, countErr)) {
			t.Fatalf("Write(string) must not be recognised")
		}
	})
}

func TestMarshalShapes(t *testing.T) {
	t.Parallel()

	bytesErr := []*node.TypeRef{byteSlice(), errorRef()}

	t.Run("recognises the text codec pair", func(t *testing.T) {
		t.Parallel()
		if !golang.IsMarshalText(method("MarshalText", nil, bytesErr)) {
			t.Fatalf("MarshalText() ([]byte, error) must be recognised")
		}
		unmarshal := method("UnmarshalText", []*node.TypeRef{byteSlice()}, []*node.TypeRef{errorRef()})
		if !golang.IsUnmarshalText(unmarshal) {
			t.Fatalf("UnmarshalText([]byte) error must be recognised")
		}
	})

	t.Run("recognises the json codec pair", func(t *testing.T) {
		t.Parallel()
		if !golang.IsMarshalJSON(method("MarshalJSON", nil, bytesErr)) {
			t.Fatalf("MarshalJSON() ([]byte, error) must be recognised")
		}
		unmarshal := method("UnmarshalJSON", []*node.TypeRef{byteSlice()}, []*node.TypeRef{errorRef()})
		if !golang.IsUnmarshalJSON(unmarshal) {
			t.Fatalf("UnmarshalJSON([]byte) error must be recognised")
		}
	})
}

func TestImplements(t *testing.T) {
	t.Parallel()

	t.Run("finds the shape anywhere in the set", func(t *testing.T) {
		t.Parallel()
		// Takes the method list rather than the declaration, because
		// a struct, an interface and an alias all carry one.
		set := []*node.Method{
			method("Close", nil, []*node.TypeRef{errorRef()}),
			method("Error", nil, []*node.TypeRef{builtinRef("string")}),
		}
		if !golang.ImplementsError(set) {
			t.Fatalf("ImplementsError must find Error anywhere in the set")
		}
	})

	t.Run("an empty set implements nothing", func(t *testing.T) {
		t.Parallel()
		if golang.ImplementsError(nil) || golang.ImplementsStringer(nil) {
			t.Fatalf("an empty method set must implement nothing")
		}
	})

	t.Run("recognises Stringer, Writer and Reader", func(t *testing.T) {
		t.Parallel()
		countErr := []*node.TypeRef{builtinRef("int"), errorRef()}
		stringer := []*node.Method{method("String", nil, []*node.TypeRef{builtinRef("string")})}
		writer := []*node.Method{method("Write", []*node.TypeRef{byteSlice()}, countErr)}
		reader := []*node.Method{method("Read", []*node.TypeRef{byteSlice()}, countErr)}

		if !golang.ImplementsStringer(stringer) {
			t.Errorf("ImplementsStringer = false")
		}
		if !golang.ImplementsWriter(writer) {
			t.Errorf("ImplementsWriter = false")
		}
		if !golang.ImplementsReader(reader) {
			t.Errorf("ImplementsReader = false")
		}
	})
}

func TestSignatureMatches(t *testing.T) {
	t.Parallel()

	t.Run("matches a builtin signature in order", func(t *testing.T) {
		t.Parallel()
		m := method("Resize", []*node.TypeRef{builtinRef("int"), builtinRef("int")},
			[]*node.TypeRef{builtinRef("bool")})
		if !golang.SignatureMatches(m, []string{"int", "int"}, []string{"bool"}) {
			t.Fatalf("SignatureMatches must accept the declared shape")
		}
	})

	t.Run("order matters", func(t *testing.T) {
		t.Parallel()
		m := method("F", []*node.TypeRef{builtinRef("int"), builtinRef("string")}, nil)
		if golang.SignatureMatches(m, []string{"string", "int"}, nil) {
			t.Fatalf("SignatureMatches must respect parameter order")
		}
	})

	t.Run("a nil method matches nothing", func(t *testing.T) {
		t.Parallel()
		if golang.SignatureMatches(nil, nil, nil) {
			t.Fatalf("SignatureMatches(nil) = true")
		}
	})

	t.Run("ReturnsOnly generalises the single-return shapes", func(t *testing.T) {
		t.Parallel()
		m := method("Validate", nil, []*node.TypeRef{errorRef()})
		if !golang.ReturnsOnly(m, "error") {
			t.Fatalf("ReturnsOnly(error) = false")
		}
		if golang.ReturnsOnly(m, "string") {
			t.Fatalf("ReturnsOnly(string) = true")
		}
	})
}

func TestNilMethodShapes(t *testing.T) {
	t.Parallel()

	t.Run("every shape rejects a nil method rather than panicking", func(t *testing.T) {
		t.Parallel()
		// These are called from per-method loops where a nil is a data
		// gap, not a programming error.
		for name, got := range map[string]bool{
			"Error":         golang.IsErrorMethod(nil),
			"Unwrap":        golang.IsUnwrapMethod(nil),
			"Is":            golang.IsIsMethod(nil),
			"As":            golang.IsAsMethod(nil),
			"String":        golang.IsStringMethod(nil),
			"Write":         golang.IsWriteMethod(nil),
			"Read":          golang.IsReadMethod(nil),
			"Close":         golang.IsCloseMethod(nil),
			"MarshalText":   golang.IsMarshalText(nil),
			"UnmarshalJSON": golang.IsUnmarshalJSON(nil),
		} {
			if got {
				t.Errorf("Is%sMethod(nil) = true", name)
			}
		}
	})
}

func TestSigShapeEdges(t *testing.T) {
	t.Parallel()

	t.Run("arity mismatch is rejected before types are read", func(t *testing.T) {
		t.Parallel()
		m := method("F", []*node.TypeRef{builtinRef("int")}, nil)
		if golang.SignatureMatches(m, nil, nil) {
			t.Fatalf("SignatureMatches must reject a parameter-count mismatch")
		}
		if golang.SignatureMatches(m, []string{"int"}, []string{"error"}) {
			t.Fatalf("SignatureMatches must reject a return-count mismatch")
		}
	})

	t.Run("a nil slot is not the type it was asked about", func(t *testing.T) {
		t.Parallel()
		// A bridge or a hand-built fixture can leave a gap in the list.
		m := &node.Method{Name: "F", Params: []*node.Param{nil}, Returns: []*node.Return{nil}}
		if golang.SignatureMatches(m, []string{"int"}, []string{"error"}) {
			t.Fatalf("a nil slot must not match a named builtin")
		}
		if golang.ReturnsOnly(m, "error") {
			t.Fatalf("ReturnsOnly must reject a nil slot")
		}
	})

	t.Run("a nil byte slice is not one", func(t *testing.T) {
		t.Parallel()
		if golang.IsByteSliceAny(nil) || golang.IsByteSliceAny(builtinRef("byte")) {
			t.Fatalf("IsByteSliceAny must require a slice")
		}
	})
}

func TestBinaryAndGobShapes(t *testing.T) {
	t.Parallel()

	bytesErr := []*node.TypeRef{byteSlice(), errorRef()}

	t.Run("recognises the binary codec pair", func(t *testing.T) {
		t.Parallel()
		// The family time.Time and net.IP implement; a table stopping
		// at JSON reports a binary-only type as encoding nothing.
		if !golang.IsMarshalBinary(method("MarshalBinary", nil, bytesErr)) {
			t.Fatalf("MarshalBinary() ([]byte, error) must be recognised")
		}
		un := method("UnmarshalBinary", []*node.TypeRef{byteSlice()}, []*node.TypeRef{errorRef()})
		if !golang.IsUnmarshalBinary(un) {
			t.Fatalf("UnmarshalBinary([]byte) error must be recognised")
		}
	})

	t.Run("recognises the gob pair separately", func(t *testing.T) {
		t.Parallel()
		// gob prefers its own pair and falls back to the binary one, so
		// a type declaring both encodes differently through each.
		if !golang.IsGobEncode(method("GobEncode", nil, bytesErr)) {
			t.Fatalf("GobEncode must be recognised")
		}
		dec := method("GobDecode", []*node.TypeRef{byteSlice()}, []*node.TypeRef{errorRef()})
		if !golang.IsGobDecode(dec) {
			t.Fatalf("GobDecode must be recognised")
		}
	})
}

func TestSQLShapes(t *testing.T) {
	t.Parallel()

	t.Run("recognises the scanner half on any", func(t *testing.T) {
		t.Parallel()
		m := method("Scan", []*node.TypeRef{builtinRef("any")}, []*node.TypeRef{errorRef()})
		if !golang.IsScanMethod(m) {
			t.Fatalf("Scan(any) error must be recognised")
		}
	})

	t.Run("recognises the valuer half on the qualified alias", func(t *testing.T) {
		t.Parallel()
		// A frontend records the written spelling, and a type declaring
		// `Value() (any, error)` is a different API that happens to be
		// assignable.
		driverValue := namedTypeRef("database/sql/driver", "Value")
		m := method("Value", nil, []*node.TypeRef{driverValue, errorRef()})
		if !golang.IsValuerMethod(m) {
			t.Fatalf("Value() (driver.Value, error) must be recognised")
		}
		bare := method("Value", nil, []*node.TypeRef{builtinRef("any"), errorRef()})
		if golang.IsValuerMethod(bare) {
			t.Fatalf("Value() (any, error) must not be taken for the driver interface")
		}
	})
}

func TestSorterShapes(t *testing.T) {
	t.Parallel()

	sorter := func() []*node.Method {
		return []*node.Method{
			method("Len", nil, []*node.TypeRef{builtinRef("int")}),
			method("Less", []*node.TypeRef{builtinRef("int"), builtinRef("int")},
				[]*node.TypeRef{builtinRef("bool")}),
			method("Swap", []*node.TypeRef{builtinRef("int"), builtinRef("int")}, nil),
		}
	}

	t.Run("recognises the full triple", func(t *testing.T) {
		t.Parallel()
		if !golang.ImplementsSorter(sorter()) {
			t.Fatalf("the sort.Interface triple must be recognised")
		}
	})

	t.Run("two of three satisfy nothing", func(t *testing.T) {
		t.Parallel()
		// A generator emitting a sort against a partial set produces a
		// file that does not compile.
		if golang.ImplementsSorter(sorter()[:2]) {
			t.Fatalf("a partial sort.Interface must not be recognised")
		}
	})
}

func TestConventionShapes(t *testing.T) {
	t.Parallel()

	t.Run("recognises the self-typed comparison conventions", func(t *testing.T) {
		t.Parallel()
		// The parameter is unconstrained because the convention is
		// self-typed and this package was not given the receiver.
		self := namedTypeRef("time", "Time")
		if !golang.IsEqualMethod(method("Equal", []*node.TypeRef{self}, []*node.TypeRef{builtinRef("bool")})) {
			t.Fatalf("Equal(T) bool must be recognised")
		}
		if !golang.IsCompareMethod(method("Compare", []*node.TypeRef{self}, []*node.TypeRef{builtinRef("int")})) {
			t.Fatalf("Compare(T) int must be recognised")
		}
	})

	t.Run("recognises clone, reset and validate", func(t *testing.T) {
		t.Parallel()
		self := namedTypeRef("x", "Config")
		if !golang.IsCloneMethod(method("Clone", nil, []*node.TypeRef{self})) {
			t.Fatalf("Clone() T must be recognised")
		}
		if !golang.IsResetMethod(method("Reset", nil, nil)) {
			t.Fatalf("Reset() must be recognised")
		}
		if !golang.IsValidateMethod(method("Validate", nil, []*node.TypeRef{errorRef()})) {
			t.Fatalf("Validate() error must be recognised")
		}
	})

	t.Run("a same-named method of another shape is rejected", func(t *testing.T) {
		t.Parallel()
		if golang.IsCloneMethod(method("Clone", []*node.TypeRef{builtinRef("int")}, nil)) {
			t.Fatalf("Clone(int) must not be recognised")
		}
		if golang.IsResetMethod(method("Reset", nil, []*node.TypeRef{errorRef()})) {
			t.Fatalf("Reset() error must not be recognised")
		}
	})
}

func TestCodecs(t *testing.T) {
	t.Parallel()

	bytesErr := []*node.TypeRef{byteSlice(), errorRef()}
	errOnly := []*node.TypeRef{errorRef()}

	t.Run("reports only complete pairs", func(t *testing.T) {
		t.Parallel()
		// A type declaring MarshalJSON without its partner does not
		// round-trip, and a check asserting that it does fails against
		// code that never claimed to.
		half := []*node.Method{method("MarshalJSON", nil, bytesErr)}
		if got := golang.Codecs(half); len(got) != 0 {
			t.Fatalf("Codecs = %v, want none for a half pair", got)
		}
	})

	t.Run("reports every complete pair in a stable order", func(t *testing.T) {
		t.Parallel()
		// Byte-stable output: a set-ordered result would reshuffle the
		// generated file between runs.
		set := []*node.Method{
			method("MarshalBinary", nil, bytesErr),
			method("UnmarshalBinary", []*node.TypeRef{byteSlice()}, errOnly),
			method("MarshalText", nil, bytesErr),
			method("UnmarshalText", []*node.TypeRef{byteSlice()}, errOnly),
		}
		want := []string{"text", "binary"}
		if got := golang.Codecs(set); !slices.Equal(got, want) {
			t.Fatalf("Codecs = %v, want %v", got, want)
		}
	})

	t.Run("a type declaring none reports none", func(t *testing.T) {
		t.Parallel()
		if got := golang.Codecs(nil); len(got) != 0 {
			t.Fatalf("Codecs(nil) = %v", got)
		}
	})
}

func TestSigShapeAccessorEdges(t *testing.T) {
	t.Parallel()

	t.Run("an out-of-range slot reads as nothing", func(t *testing.T) {
		t.Parallel()
		// SignatureMatches indexes both lists after checking arity, so
		// the guard is reached only by a direct caller.
		m := method("F", nil, nil)
		if golang.ReturnsOnly(m, "error") {
			t.Fatalf("ReturnsOnly matched an empty return list")
		}
		if golang.SignatureMatches(m, []string{"int"}, nil) {
			t.Fatalf("SignatureMatches matched a shorter parameter list")
		}
	})
}

// variadicMethod builds a method whose final parameter is variadic,
// recorded the way a frontend records one: the ELEMENT type, with the
// flag set beside it.
func variadicMethod(name string, params, returns []*node.TypeRef) *node.Method {
	m := method(name, params, returns)
	if n := len(m.Params); n > 0 {
		m.Params[n-1].Variadic = true
	}
	return m
}

func TestShapesRejectAVariadicParameter(t *testing.T) {
	t.Parallel()

	// A frontend records `...T` as T with Variadic set, so every
	// predicate here reads exactly the type the fixed-arity contract
	// wants. Go does not agree: `Write(p ...[]byte)` has a different
	// method set from `Write(p []byte)` and satisfies io.Writer not at
	// all. Each case below returned true before the guard landed.
	countErr := []*node.TypeRef{builtinRef("int"), errorRef()}

	t.Run("a variadic byte slice is not io.Writer's Write", func(t *testing.T) {
		t.Parallel()
		m := variadicMethod("Write", []*node.TypeRef{byteSlice()}, countErr)
		if golang.IsWriteMethod(m) {
			t.Fatal("Write(p ...[]byte) reported as io.Writer's Write")
		}
	})

	t.Run("a variadic byte slice is not io.Reader's Read", func(t *testing.T) {
		t.Parallel()
		m := variadicMethod("Read", []*node.TypeRef{byteSlice()}, countErr)
		if golang.IsReadMethod(m) {
			t.Fatal("Read(p ...[]byte) reported as io.Reader's Read")
		}
	})

	t.Run("a variadic int is not sort.Interface's Less", func(t *testing.T) {
		t.Parallel()
		// The general comparator, SignatureMatches, rather than the
		// byte-slice family — the two reached the same wrong answer by
		// different routes.
		ints := []*node.TypeRef{builtinRef("int"), builtinRef("int")}
		m := variadicMethod("Less", ints, []*node.TypeRef{builtinRef("bool")})
		if golang.IsLessMethod(m) {
			t.Fatal("Less(i int, j ...int) reported as sort.Interface's Less")
		}
	})

	t.Run("a variadic argument is not sql.Scanner's Scan", func(t *testing.T) {
		t.Parallel()
		m := variadicMethod("Scan", []*node.TypeRef{builtinRef("any")}, []*node.TypeRef{errorRef()})
		if golang.IsScanMethod(m) {
			t.Fatal("Scan(v ...any) reported as sql.Scanner's Scan")
		}
	})

	t.Run("the non-variadic form still matches", func(t *testing.T) {
		t.Parallel()
		// The guard is a narrowing; it must remove false positives and
		// nothing else.
		if !golang.IsWriteMethod(method("Write", []*node.TypeRef{byteSlice()}, countErr)) {
			t.Fatal("the canonical Write stopped matching")
		}
	})
}

// TestSignatureMatches_ReturnMismatch pins the return half of the
// comparison, including the slot that does not exist.
//
// A classifier asking for more returns than a method declares must
// read the absence as a mismatch. Reading it as a match would accept
// every nullary method as the shape it is looking for.
func TestSignatureMatches_ReturnMismatch(t *testing.T) {
	t.Parallel()

	t.Run("a method returning nothing does not match one return", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "Close"}
		if golang.SignatureMatches(m, nil, []string{"error"}) {
			t.Error("a method with no returns matched a one-return shape")
		}
	})

	t.Run("a method returning the wrong type does not match", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "Len", Returns: []*node.Return{ret(builtinRef("int"))}}
		if golang.SignatureMatches(m, nil, []string{"error"}) {
			t.Error("an int return matched an error shape")
		}
	})

	t.Run("a method returning the declared type matches", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "Len", Returns: []*node.Return{ret(builtinRef("int"))}}
		if !golang.SignatureMatches(m, nil, []string{"int"}) {
			t.Error("an int return did not match an int shape")
		}
	})
}
