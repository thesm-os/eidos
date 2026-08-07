// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// ctxRef returns a reference to context.Context as a Go frontend
// records it.
func ctxRef() *node.TypeRef { return namedTypeRef("context", "Context") }

// param builds a named parameter of the given type.
func param(name string, t *node.TypeRef) *node.Param {
	return &node.Param{Name: name, Type: t}
}

// ret builds an anonymous return slot of the given type.
func ret(t *node.TypeRef) *node.Return { return &node.Return{Type: t} }

func TestCallable(t *testing.T) {
	t.Parallel()

	t.Run("destructures a function", func(t *testing.T) {
		t.Parallel()
		fn := &node.Function{Params: []*node.Param{param("a", builtinRef("int"))}}
		p, r := golang.Callable(fn)
		if len(p) != 1 || len(r) != 0 {
			t.Fatalf("Callable = %d params, %d returns; want 1, 0", len(p), len(r))
		}
	})

	t.Run("destructures a method", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Returns: []*node.Return{ret(errorRef())}}
		p, r := golang.Callable(m)
		if len(p) != 0 || len(r) != 1 {
			t.Fatalf("Callable = %d params, %d returns; want 0, 1", len(p), len(r))
		}
	})

	t.Run("a non-callable yields nothing rather than panicking", func(t *testing.T) {
		t.Parallel()
		// Callers early-return on a length check rather than writing a
		// type-assertion ladder before every query.
		p, r := golang.Callable(&node.Struct{Name: "S"})
		if p != nil || r != nil {
			t.Fatalf("Callable(struct) = %v, %v; want nil, nil", p, r)
		}
	})
}

func TestContextQueries(t *testing.T) {
	t.Parallel()

	withCtx := []*node.Param{param("ctx", ctxRef()), param("id", builtinRef("string"))}

	t.Run("finds a leading context", func(t *testing.T) {
		t.Parallel()
		if !golang.HasContext(withCtx) {
			t.Fatalf("HasContext must find a leading context.Context")
		}
	})

	t.Run("a context elsewhere is not the leading one", func(t *testing.T) {
		t.Parallel()
		// Go's convention places it first; a context in another
		// position is not the cancellation scope a generator threads.
		rest := []*node.Param{param("id", builtinRef("string")), param("ctx", ctxRef())}
		if golang.HasContext(rest) {
			t.Fatalf("HasContext must only match the first parameter")
		}
	})

	t.Run("strips a leading context", func(t *testing.T) {
		t.Parallel()
		if got := golang.StripContext(withCtx); len(got) != 1 || got[0].Name != "id" {
			t.Fatalf("StripContext = %v, want just id", got)
		}
	})

	t.Run("strips nothing when there is no context", func(t *testing.T) {
		t.Parallel()
		// Applied unconditionally by a caller classifying on arity.
		bare := []*node.Param{param("id", builtinRef("string"))}
		if got := golang.StripContext(bare); len(got) != 1 {
			t.Fatalf("StripContext = %v, want the input unchanged", got)
		}
	})

	t.Run("an unstamped context is found by its spelling", func(t *testing.T) {
		t.Parallel()
		// The union: a fixture carries no stamp, and a stamp-only
		// answer would report that this signature threads none.
		if !golang.HasContext([]*node.Param{param("ctx", ctxRef())}) {
			t.Fatalf("an unstamped context.Context must be recognised")
		}
	})
}

func TestVariadicQueries(t *testing.T) {
	t.Parallel()

	variadic := []*node.Param{
		param("prefix", builtinRef("string")),
		{Name: "rest", Type: builtinRef("string"), Variadic: true},
	}

	t.Run("finds a trailing variadic", func(t *testing.T) {
		t.Parallel()
		if got := golang.TrailingVariadic(variadic); got == nil || got.Name != "rest" {
			t.Fatalf("TrailingVariadic = %v, want rest", got)
		}
	})

	t.Run("a fixed list has none", func(t *testing.T) {
		t.Parallel()
		fixed := []*node.Param{param("a", builtinRef("int"))}
		if got := golang.TrailingVariadic(fixed); got != nil {
			t.Fatalf("TrailingVariadic = %v, want nil", got)
		}
	})

	t.Run("strips the variadic tail", func(t *testing.T) {
		t.Parallel()
		if got := golang.StripVariadic(variadic); len(got) != 1 || got[0].Name != "prefix" {
			t.Fatalf("StripVariadic = %v, want just prefix", got)
		}
	})
}

func TestErrorQueries(t *testing.T) {
	t.Parallel()

	rets := []*node.Return{ret(builtinRef("string")), ret(errorRef())}

	t.Run("returns the error slot itself", func(t *testing.T) {
		t.Parallel()
		// The slot rather than the index, for a caller that wants the
		// declared name a generated body binds.
		if got := golang.ErrorReturn(rets); got == nil {
			t.Fatalf("ErrorReturn = nil, want the trailing slot")
		}
	})

	t.Run("strips the error and keeps the slots", func(t *testing.T) {
		t.Parallel()
		got := golang.StripError(rets)
		if len(got) != 1 || got[0].Type.Name != "string" {
			t.Fatalf("StripError = %v, want one string slot", got)
		}
	})

	t.Run("strips the error and projects to types", func(t *testing.T) {
		t.Parallel()
		got := golang.StripErrorTypes(rets)
		if len(got) != 1 || got[0].Name != "string" {
			t.Fatalf("StripErrorTypes = %v, want [string]", got)
		}
	})

	t.Run("a signature with no error is returned whole", func(t *testing.T) {
		t.Parallel()
		bare := []*node.Return{ret(builtinRef("string"))}
		if got := golang.StripError(bare); len(got) != 1 {
			t.Fatalf("StripError = %v, want the input unchanged", got)
		}
	})

	t.Run("strips only the first of several errors", func(t *testing.T) {
		t.Parallel()
		two := []*node.Return{ret(errorRef()), ret(errorRef())}
		if got := golang.StripError(two); len(got) != 1 {
			t.Fatalf("StripError = %v, want one slot left", got)
		}
	})
}

func TestTypeDeconstruction(t *testing.T) {
	t.Parallel()

	t.Run("reads a pointer element", func(t *testing.T) {
		t.Parallel()
		p := &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: builtinRef("int")}
		if got := golang.PointerElem(p); got == nil || got.Name != "int" {
			t.Fatalf("PointerElem = %v, want int", got)
		}
	})

	t.Run("reads a byte-slice element, unlike IsSlice", func(t *testing.T) {
		t.Parallel()
		// IsSlice routes []byte elsewhere so a template can branch; a
		// caller asking for the element wants it either way.
		b := &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: builtinRef("byte")}
		if got := golang.SliceElem(b); got == nil || got.Name != "byte" {
			t.Fatalf("SliceElem = %v, want byte", got)
		}
	})

	t.Run("an array yields element and length together", func(t *testing.T) {
		t.Parallel()
		// The zero of an array cannot be spelled without its length.
		a := &node.TypeRef{TypeKind: node.TypeRefArray, ArrayLen: 4, Elem: builtinRef("byte")}
		elem, n := golang.ArrayElem(a)
		if elem == nil || n != 4 {
			t.Fatalf("ArrayElem = %v, %d; want byte, 4", elem, n)
		}
	})

	t.Run("reads map key and value", func(t *testing.T) {
		t.Parallel()
		m := mapRef(builtinRef("string"), builtinRef("int"))
		if k := golang.MapKey(m); k == nil || k.Name != "string" {
			t.Fatalf("MapKey = %v, want string", k)
		}
		if v := golang.MapValue(m); v == nil || v.Name != "int" {
			t.Fatalf("MapValue = %v, want int", v)
		}
	})

	t.Run("deref strips every pointer layer", func(t *testing.T) {
		t.Parallel()
		// `**T` is legal Go, and a caller resolving the named type at
		// the bottom wants the bottom.
		inner := &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: namedTypeRef("x", "User")}
		outer := &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: inner}
		if got := golang.Deref(outer); got == nil || got.Name != "User" {
			t.Fatalf("Deref = %v, want User", got)
		}
	})

	t.Run("deref leaves a non-pointer alone", func(t *testing.T) {
		t.Parallel()
		u := namedTypeRef("x", "User")
		if got := golang.Deref(u); got != u {
			t.Fatalf("Deref changed a non-pointer")
		}
	})

	t.Run("a wrong-kind read yields nil rather than a wrong answer", func(t *testing.T) {
		t.Parallel()
		s := &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: builtinRef("int")}
		if got := golang.PointerElem(s); got != nil {
			t.Fatalf("PointerElem(slice) = %v, want nil", got)
		}
		if got := golang.MapKey(s); got != nil {
			t.Fatalf("MapKey(slice) = %v, want nil", got)
		}
	})
}

func TestIteratorQueries(t *testing.T) {
	t.Parallel()

	seq := func(args ...*node.TypeRef) *node.TypeRef {
		name := "Seq"
		if len(args) == 2 {
			name = "Seq2"
		}
		r := namedTypeRef("iter", name)
		r.TypeArgs = args
		return r
	}

	t.Run("classifies a Seq", func(t *testing.T) {
		t.Parallel()
		if got := golang.IteratorOfType(seq(builtinRef("int"))); got != golang.SeqIterator {
			t.Fatalf("IteratorOfType = %q, want seq", got)
		}
	})

	t.Run("classifies a Seq2", func(t *testing.T) {
		t.Parallel()
		got := golang.IteratorOfType(seq(builtinRef("string"), builtinRef("int")))
		if got != golang.Seq2Iterator {
			t.Fatalf("IteratorOfType = %q, want seq2", got)
		}
	})

	t.Run("a consumer's own two-arg generic is not a sequence", func(t *testing.T) {
		t.Parallel()
		// Matched on the stdlib path: treating one as a sequence emits
		// helpers that do not compile.
		mine := namedTypeRef("example.com/x", "Seq2")
		mine.TypeArgs = []*node.TypeRef{builtinRef("string"), builtinRef("int")}
		if got := golang.IteratorOfType(mine); got != golang.NotIterator {
			t.Fatalf("IteratorOfType = %q, want none", got)
		}
	})

	t.Run("reads the element of both forms", func(t *testing.T) {
		t.Parallel()
		if got := golang.IteratorElem(seq(builtinRef("int"))); got == nil || got.Name != "int" {
			t.Fatalf("IteratorElem(Seq) = %v, want int", got)
		}
		two := seq(builtinRef("int"), errorRef())
		if got := golang.IteratorElem(two); got == nil || got.Name != "int" {
			t.Fatalf("IteratorElem(Seq2) = %v, want the first arg", got)
		}
	})

	t.Run("recognises the failable sequence", func(t *testing.T) {
		t.Parallel()
		// The one spelling where a helper can usefully append a
		// terminal failure.
		if !golang.IteratorYieldsError(seq(builtinRef("int"), errorRef())) {
			t.Fatalf("iter.Seq2[V, error] must read as failable")
		}
		if golang.IteratorYieldsError(seq(builtinRef("string"), builtinRef("int"))) {
			t.Fatalf("iter.Seq2[K, V] must not read as failable")
		}
	})
}

func TestStructuralPredicates(t *testing.T) {
	t.Parallel()

	t.Run("classifies the numeric families", func(t *testing.T) {
		t.Parallel()
		for name, want := range map[string]struct{ integer, float, cplx bool }{
			"int8":       {true, false, false},
			"byte":       {true, false, false},
			"rune":       {true, false, false},
			"float64":    {false, true, false},
			"complex128": {false, false, true},
			"string":     {false, false, false},
		} {
			r := builtinRef(name)
			if golang.IsInteger(r) != want.integer {
				t.Errorf("IsInteger(%s) = %v", name, !want.integer)
			}
			if golang.IsFloat(r) != want.float {
				t.Errorf("IsFloat(%s) = %v", name, !want.float)
			}
			if golang.IsComplex(r) != want.cplx {
				t.Errorf("IsComplex(%s) = %v", name, !want.cplx)
			}
		}
	})

	t.Run("IsNumeric unions the three", func(t *testing.T) {
		t.Parallel()
		// The set whose zero value is the literal 0 — the question a
		// generator writing a composite literal actually asks.
		for _, name := range []string{"int8", "float32", "complex64", "uintptr"} {
			if !golang.IsNumeric(builtinRef(name)) {
				t.Errorf("IsNumeric(%s) = false", name)
			}
		}
		if golang.IsNumeric(builtinRef("bool")) {
			t.Fatalf("IsNumeric(bool) = true")
		}
	})

	t.Run("IsAny accepts both spellings", func(t *testing.T) {
		t.Parallel()
		// `any` arrives as a builtin and `interface{}` as an anonymous
		// interface; a check on one misses code written the other way.
		if !golang.IsAny(builtinRef("any")) {
			t.Fatalf("IsAny(any) = false")
		}
		if !golang.IsAny(&node.TypeRef{TypeKind: node.TypeRefAnonInterface}) {
			t.Fatalf("IsAny(interface{}) = false")
		}
	})

	t.Run("a non-empty inline interface is not any", func(t *testing.T) {
		t.Parallel()
		iface := &node.TypeRef{
			TypeKind: node.TypeRefAnonInterface,
			Methods:  []*node.Method{{Name: "Read"}},
		}
		if golang.IsAny(iface) {
			t.Fatalf("an interface declaring a method must not read as any")
		}
	})

	t.Run("a qualified type is not a builtin of that name", func(t *testing.T) {
		t.Parallel()
		if golang.IsBuiltinNamed(namedTypeRef("example.com/x", "int"), "int") {
			t.Fatalf("x.int must not match the builtin int")
		}
	})

	t.Run("Nilable covers every shape whose zero is nil", func(t *testing.T) {
		t.Parallel()
		for name, r := range map[string]*node.TypeRef{
			"pointer":   {TypeKind: node.TypeRefPointer, Elem: builtinRef("int")},
			"slice":     {TypeKind: node.TypeRefSlice, Elem: builtinRef("int")},
			"map":       mapRef(builtinRef("string"), builtinRef("int")),
			"func":      {TypeKind: node.TypeRefFunc},
			"interface": {TypeKind: node.TypeRefAnonInterface},
			"error":     builtinRef("error"),
			"any":       builtinRef("any"),
		} {
			if !golang.Nilable(r) {
				t.Errorf("Nilable(%s) = false", name)
			}
		}
		if golang.Nilable(builtinRef("int")) {
			t.Fatalf("Nilable(int) = true")
		}
	})

	t.Run("Keyable refuses what Go has no equality for", func(t *testing.T) {
		t.Parallel()
		// Slices, maps and functions have no equality at all, so "the
		// same key" is not expressible for them.
		for name, r := range map[string]*node.TypeRef{
			"slice":            {TypeKind: node.TypeRefSlice, Elem: builtinRef("int")},
			"map":              mapRef(builtinRef("string"), builtinRef("int")),
			"func":             {TypeKind: node.TypeRefFunc},
			"inline interface": {TypeKind: node.TypeRefAnonInterface},
		} {
			if golang.Keyable(r) {
				t.Errorf("Keyable(%s) = true", name)
			}
		}
		if !golang.Keyable(builtinRef("string")) {
			t.Fatalf("Keyable(string) = false")
		}
	})

	t.Run("IsBlank recognises the one unnameable identifier", func(t *testing.T) {
		t.Parallel()
		if !golang.IsBlank("_") || golang.IsBlank("x") {
			t.Fatalf("IsBlank must match exactly the blank identifier")
		}
	})
}

func TestReceiverQueries(t *testing.T) {
	t.Parallel()

	t.Run("reads a method receiver", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Receiver: &node.TypeRef{
			TypeKind: node.TypeRefPointer, Elem: namedTypeRef("x", "Repo"),
		}}
		if got := golang.ReceiverOf(m); got == nil || !got.IsPointer() {
			t.Fatalf("ReceiverOf = %v, want a pointer receiver", got)
		}
	})

	t.Run("a function has no receiver", func(t *testing.T) {
		t.Parallel()
		if got := golang.ReceiverOf(&node.Function{Name: "F"}); got != nil {
			t.Fatalf("ReceiverOf(func) = %v, want nil", got)
		}
	})

	t.Run("an interface method is told apart by its absent receiver", func(t *testing.T) {
		t.Parallel()
		// The distinction matters wherever a generator emits a body:
		// an interface method has none to emit.
		if !golang.IsInterfaceMethod(&node.Method{Name: "Get"}) {
			t.Fatalf("a receiverless method must read as an interface method")
		}
		concrete := &node.Method{Name: "Get", Receiver: namedTypeRef("x", "Repo")}
		if golang.IsInterfaceMethod(concrete) {
			t.Fatalf("a method with a receiver must not read as an interface method")
		}
	})
}

func TestQueryNilAndEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("every deconstructor tolerates nil", func(t *testing.T) {
		t.Parallel()
		// Called from per-type loops where a nil is a data gap, not a
		// programming error.
		nilOut := golang.PointerElem(nil) == nil && golang.SliceElem(nil) == nil &&
			golang.MapKey(nil) == nil && golang.MapValue(nil) == nil &&
			golang.Deref(nil) == nil && golang.IteratorElem(nil) == nil &&
			golang.IteratorSecond(nil) == nil
		if !nilOut {
			t.Fatalf("a nil reference must deconstruct to nil")
		}
		if elem, n := golang.ArrayElem(nil); elem != nil || n != 0 {
			t.Fatalf("ArrayElem(nil) = %v, %d", elem, n)
		}
		if p, r := golang.FuncSignature(nil); p != nil || r != nil {
			t.Fatalf("FuncSignature(nil) = %v, %v", p, r)
		}
	})

	t.Run("reads a function type's signature", func(t *testing.T) {
		t.Parallel()
		// A function type carries no parameter names — the model
		// records only types — so a field of this type binds nothing.
		fn := &node.TypeRef{
			TypeKind:    node.TypeRefFunc,
			FuncParams:  []*node.TypeRef{builtinRef("int")},
			FuncReturns: []*node.TypeRef{errorRef()},
		}
		p, r := golang.FuncSignature(fn)
		if len(p) != 1 || len(r) != 1 {
			t.Fatalf("FuncSignature = %d, %d; want 1, 1", len(p), len(r))
		}
	})

	t.Run("a non-func yields no signature", func(t *testing.T) {
		t.Parallel()
		if p, r := golang.FuncSignature(builtinRef("int")); p != nil || r != nil {
			t.Fatalf("FuncSignature(int) = %v, %v", p, r)
		}
	})

	t.Run("returns the leading context parameter itself", func(t *testing.T) {
		t.Parallel()
		params := []*node.Param{param("ctx", ctxRef()), param("id", builtinRef("string"))}
		if got := golang.ContextParam(params); got == nil || got.Name != "ctx" {
			t.Fatalf("ContextParam = %v, want ctx", got)
		}
		if got := golang.ContextParam(params[1:]); got != nil {
			t.Fatalf("ContextParam = %v, want nil", got)
		}
	})

	t.Run("reads a Seq2's second argument", func(t *testing.T) {
		t.Parallel()
		// The error slot in the failable spelling; a Seq has none.
		two := namedTypeRef("iter", "Seq2")
		two.TypeArgs = []*node.TypeRef{builtinRef("int"), errorRef()}
		if got := golang.IteratorSecond(two); got == nil || got.Name != "error" {
			t.Fatalf("IteratorSecond = %v, want error", got)
		}
		one := namedTypeRef("iter", "Seq")
		one.TypeArgs = []*node.TypeRef{builtinRef("int")}
		if got := golang.IteratorSecond(one); got != nil {
			t.Fatalf("IteratorSecond(Seq) = %v, want nil", got)
		}
	})

	t.Run("an iter type of the wrong arity is not a sequence", func(t *testing.T) {
		t.Parallel()
		// Matched on arity as well as name: `iter.Seq` with no
		// argument names no element a helper could yield.
		bare := namedTypeRef("iter", "Seq")
		if got := golang.IteratorOfType(bare); got != golang.NotIterator {
			t.Fatalf("IteratorOfType = %q, want none", got)
		}
		other := namedTypeRef("iter", "Pull")
		if got := golang.IteratorOfType(other); got != golang.NotIterator {
			t.Fatalf("IteratorOfType(iter.Pull) = %q, want none", got)
		}
	})

	t.Run("every predicate rejects nil", func(t *testing.T) {
		t.Parallel()
		anyTrue := golang.IsAny(nil) || golang.Nilable(nil) || golang.Keyable(nil) ||
			golang.IsInteger(nil) || golang.IsBuiltinNamed(nil, "int")
		if anyTrue {
			t.Fatalf("a nil reference must answer false throughout")
		}
	})

	t.Run("a named non-nilable type is not nilable", func(t *testing.T) {
		t.Parallel()
		// A struct is comparable and not nilable; a slice is the
		// reverse. The two questions are independent.
		if golang.Nilable(namedTypeRef("time", "Duration")) {
			t.Fatalf("a defined numeric type must not read as nilable")
		}
	})

	t.Run("an array of anything is keyable", func(t *testing.T) {
		t.Parallel()
		// Deliberately broader than Go's rule, which also rejects an
		// array of an uncomparable element — resolving that needs the
		// declaration a caller holding only a reference lacks.
		arr := &node.TypeRef{TypeKind: node.TypeRefArray, ArrayLen: 2, Elem: builtinRef("int")}
		if !golang.Keyable(arr) {
			t.Fatalf("Keyable(array) = false")
		}
	})

	t.Run("a strip on an empty list yields the list", func(t *testing.T) {
		t.Parallel()
		if got := golang.StripVariadic(nil); got != nil {
			t.Fatalf("StripVariadic(nil) = %v", got)
		}
		if got := golang.ErrorReturn(nil); got != nil {
			t.Fatalf("ErrorReturn(nil) = %v", got)
		}
	})

	t.Run("a non-array yields no element", func(t *testing.T) {
		t.Parallel()
		if elem, n := golang.ArrayElem(builtinRef("int")); elem != nil || n != 0 {
			t.Fatalf("ArrayElem(int) = %v, %d", elem, n)
		}
		if got := golang.MapValue(builtinRef("int")); got != nil {
			t.Fatalf("MapValue(int) = %v", got)
		}
		if got := golang.SliceElem(builtinRef("int")); got != nil {
			t.Fatalf("SliceElem(int) = %v", got)
		}
	})
}

func TestNilableNamedInterface(t *testing.T) {
	t.Parallel()

	t.Run("a stamped named interface is nilable", func(t *testing.T) {
		t.Parallel()
		// The one shape the structural half cannot see: a named ref
		// carries a package and an identifier and nothing about what
		// they resolve to.
		r := namedTypeRef("io", "Reader")
		golang.MetaIsInterface.Set(r.EnsureMeta(), true, "test")
		if !golang.Nilable(r) {
			t.Fatalf("a stamped interface must read as nilable")
		}
	})
}
