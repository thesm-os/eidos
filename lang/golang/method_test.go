// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// getMethod is the canonical fixture: a named-return method with a
// context, a key and an error.
func getMethod() *node.Method {
	return &node.Method{
		Name: "Get",
		Params: []*node.Param{
			{Name: "ctx", Type: ctxRef()},
			{Name: "id", Type: builtinRef("string")},
		},
		Returns: []*node.Return{
			{Name: "item", Type: namedTypeRef("example.com/x", "User")},
			{Name: "err", Type: errorRef()},
		},
	}
}

// fieldNames projects a projection's return field names.
func fieldNames(s *golang.Sig) []string {
	out := make([]string, len(s.Returns))
	for i := range s.Returns {
		out[i] = s.Returns[i].Field
	}
	return out
}

func TestSigOf_Params(t *testing.T) {
	t.Parallel()

	t.Run("keeps declared names and derives fields", func(t *testing.T) {
		t.Parallel()
		s := golang.SigOf(getMethod())
		if !slices.Equal(s.Idents(), []string{"ctx", "id"}) {
			t.Fatalf("Idents = %v, want [ctx id]", s.Idents())
		}
		if s.Params[1].Field != "ID" {
			t.Fatalf("Field = %q, want ID (the initialism form)", s.Params[1].Field)
		}
	})

	t.Run("names an anonymous parameter positionally", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "Read", Params: []*node.Param{{Type: builtinRef("string")}}}
		s := golang.SigOf(m)
		if s.Params[0].Name != "arg0" {
			t.Fatalf("Name = %q, want arg0", s.Params[0].Name)
		}
		if s.Params[0].Declared != "" {
			t.Fatalf("Declared = %q, want empty for an anonymous parameter", s.Params[0].Declared)
		}
	})

	t.Run("a blank parameter is treated as anonymous", func(t *testing.T) {
		t.Parallel()
		// `_` cannot be referenced, so a body needs its own name.
		m := &node.Method{Name: "F", Params: []*node.Param{{Name: "_", Type: builtinRef("int")}}}
		if got := golang.SigOf(m).Params[0].Name; got != "arg0" {
			t.Fatalf("Name = %q, want arg0", got)
		}
	})

	t.Run("a keyword parameter is made safe", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "F", Params: []*node.Param{{Name: "type", Type: builtinRef("int")}}}
		if got := golang.SigOf(m).Params[0].Name; got == "type" {
			t.Fatalf("Name = %q, want a keyword-safe identifier", got)
		}
	})

	t.Run("a declared name cannot collide with a positional fallback", func(t *testing.T) {
		t.Parallel()
		// `Read(arg1 []byte, []byte)` names the first parameter
		// exactly what the second falls back to.
		m := &node.Method{Name: "Read", Params: []*node.Param{
			{Name: "arg1", Type: builtinRef("int")},
			{Type: builtinRef("int")},
		}}
		s := golang.SigOf(m)
		if s.Params[0].Name == s.Params[1].Name {
			t.Fatalf("Idents = %v, want distinct", s.Idents())
		}
	})

	t.Run("carries the variadic marker", func(t *testing.T) {
		t.Parallel()
		// The field stubgen and mockgen both drop, which is why their
		// doubles of a variadic method do not satisfy the interface.
		m := &node.Method{Name: "Print", Params: []*node.Param{
			{Name: "args", Type: builtinRef("string"), Variadic: true},
		}}
		s := golang.SigOf(m)
		if !s.Params[0].Variadic || !s.Variadic() {
			t.Fatalf("the variadic marker must survive projection")
		}
	})

	t.Run("retains the source type for structural queries", func(t *testing.T) {
		t.Parallel()
		// Every predicate in this package takes a node.TypeRef, so a
		// consumer classifying a parameter needs one.
		s := golang.SigOf(getMethod())
		if !golang.IsContext(s.Params[0].Source) {
			t.Fatalf("Source must carry the declared type")
		}
	})
}

func TestSigOf_Returns(t *testing.T) {
	t.Parallel()

	t.Run("names fields from declared return names", func(t *testing.T) {
		t.Parallel()
		// A signature written `(item User, err error)` documents what
		// its returns mean, and a recorded call is the main consumer
		// of that documentation.
		if got := fieldNames(golang.SigOf(getMethod())); !slices.Equal(got, []string{"Item", "Err"}) {
			t.Fatalf("fields = %v, want [Item Err]", got)
		}
	})

	t.Run("the error slot falls back to Err", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "F", Returns: []*node.Return{{Type: errorRef()}}}
		if got := fieldNames(golang.SigOf(m)); !slices.Equal(got, []string{"Err"}) {
			t.Fatalf("fields = %v, want [Err]", got)
		}
	})

	t.Run("a lone value slot falls back to Result", func(t *testing.T) {
		t.Parallel()
		// An index distinguishes it from nothing.
		m := &node.Method{Name: "F", Returns: []*node.Return{
			{Type: builtinRef("string")}, {Type: errorRef()},
		}}
		if got := fieldNames(golang.SigOf(m)); !slices.Equal(got, []string{"Result", "Err"}) {
			t.Fatalf("fields = %v, want [Result Err]", got)
		}
	})

	t.Run("value slots number independently of the error", func(t *testing.T) {
		t.Parallel()
		// Adding an error return must not renumber the fields beside
		// it — the divergence between the two existing copies.
		m := &node.Method{Name: "F", Returns: []*node.Return{
			{Type: builtinRef("string")}, {Type: errorRef()}, {Type: builtinRef("int")},
		}}
		want := []string{"Result0", "Err", "Result1"}
		if got := fieldNames(golang.SigOf(m)); !slices.Equal(got, want) {
			t.Fatalf("fields = %v, want %v", got, want)
		}
	})

	t.Run("a second error slot is indexed rather than duplicated", func(t *testing.T) {
		t.Parallel()
		// Legal and vanishingly rare; a duplicate field name would not
		// compile.
		m := &node.Method{Name: "F", Returns: []*node.Return{
			{Type: errorRef()}, {Type: errorRef()},
		}}
		if got := fieldNames(golang.SigOf(m)); !slices.Equal(got, []string{"Err", "Err1"}) {
			t.Fatalf("fields = %v, want [Err Err1]", got)
		}
	})

	t.Run("finds the error slot by flag, not position", func(t *testing.T) {
		t.Parallel()
		// `(error, string)` is unusual but legal Go, and a positional
		// rule binds the wrong slot without failing to compile.
		m := &node.Method{Name: "F", Returns: []*node.Return{
			{Type: errorRef()}, {Type: builtinRef("string")},
		}}
		got := golang.SigOf(m).ErrReturn()
		if got == nil || got.Field != "Err" {
			t.Fatalf("ErrReturn = %+v, want the leading slot", got)
		}
	})
}

func TestSigOf_Locals(t *testing.T) {
	t.Parallel()

	t.Run("named results capture into their own names", func(t *testing.T) {
		t.Parallel()
		s := golang.SigOf(getMethod())
		if !slices.Equal(s.Locals(), []string{"item", "err"}) {
			t.Fatalf("Locals = %v, want [item err]", s.Locals())
		}
	})

	t.Run("anonymous results capture positionally", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "F", Returns: []*node.Return{
			{Type: builtinRef("string")}, {Type: errorRef()},
		}}
		if got := golang.SigOf(m).Locals(); !slices.Equal(got, []string{"r0", "r1"}) {
			t.Fatalf("Locals = %v, want [r0 r1]", got)
		}
	})

	t.Run("a local colliding with a parameter is prefixed", func(t *testing.T) {
		t.Parallel()
		// Shadowing a parameter would capture the wrong value.
		m := &node.Method{
			Name:    "F",
			Params:  []*node.Param{{Name: "r0", Type: builtinRef("int")}},
			Returns: []*node.Return{{Type: builtinRef("string")}},
		}
		if got := golang.SigOf(m).Locals(); !slices.Equal(got, []string{"_r0"}) {
			t.Fatalf("Locals = %v, want [_r0]", got)
		}
	})
}

func TestSigOf_NamedReturns(t *testing.T) {
	t.Parallel()

	t.Run("a fully named signature keeps its names", func(t *testing.T) {
		t.Parallel()
		if !golang.SigOf(getMethod()).NamedReturns {
			t.Fatalf("a fully named signature must carry its names")
		}
	})

	t.Run("a partly named signature falls back", func(t *testing.T) {
		t.Parallel()
		// Go requires results to be all named or all anonymous, and
		// `(_ User, err error)` reaches the mixed state legitimately.
		m := &node.Method{Name: "F", Returns: []*node.Return{
			{Type: builtinRef("string")}, {Name: "err", Type: errorRef()},
		}}
		if golang.SigOf(m).NamedReturns {
			t.Fatalf("a mixed signature must fall back to anonymous")
		}
	})

	t.Run("a return colliding with the receiver falls back", func(t *testing.T) {
		t.Parallel()
		// `func (s *T) F() (s int)` does not compile.
		m := &node.Method{Name: "F", Returns: []*node.Return{{Name: "s", Type: builtinRef("int")}}}
		if golang.SigOf(m).NamedReturns {
			t.Fatalf("a return named for the receiver must fall back")
		}
	})

	t.Run("the reserved receiver is configurable", func(t *testing.T) {
		t.Parallel()
		// A template binding a different receiver would silently
		// invalidate the guard, so the guard takes the identifier.
		m := &node.Method{Name: "F", Returns: []*node.Return{{Name: "s", Type: builtinRef("int")}}}
		if !golang.SigOf(m, golang.WithReceiverIdent("m")).NamedReturns {
			t.Fatalf("with receiver m, a return named s is usable")
		}
	})

	t.Run("a return colliding with a parameter falls back", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{
			Name:    "F",
			Params:  []*node.Param{{Name: "item", Type: builtinRef("int")}},
			Returns: []*node.Return{{Name: "item", Type: builtinRef("int")}},
		}
		if golang.SigOf(m).NamedReturns {
			t.Fatalf("a return colliding with a parameter must fall back")
		}
	})
}

func TestSigOptions(t *testing.T) {
	t.Parallel()

	t.Run("the parameter prefix is configurable", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "F", Params: []*node.Param{{Type: builtinRef("int")}}}
		got := golang.SigOf(m, golang.WithParamPrefix("p")).Params[0].Name
		if got != "p0" {
			t.Fatalf("Name = %q, want p0", got)
		}
	})

	t.Run("the local prefix is configurable", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "F", Returns: []*node.Return{{Type: builtinRef("int")}}}
		got := golang.SigOf(m, golang.WithLocalPrefix("v")).Locals()
		if !slices.Equal(got, []string{"v0"}) {
			t.Fatalf("Locals = %v, want [v0]", got)
		}
	})
}

func TestSigOfFunc(t *testing.T) {
	t.Parallel()

	t.Run("a function reserves no receiver", func(t *testing.T) {
		t.Parallel()
		// Nothing is bound to `s`, so a return may legitimately take
		// the identifier a method's receiver would have claimed.
		f := &node.Function{Name: "F", Returns: []*node.Return{{Name: "s", Type: builtinRef("int")}}}
		s := golang.SigOfFunc(f)
		if !s.NamedReturns {
			t.Fatalf("a function's return named s must be usable")
		}
		if s.ReceiverIdent != "" {
			t.Fatalf("ReceiverIdent = %q, want empty for a function", s.ReceiverIdent)
		}
	})
}

func TestSigOfEmit(t *testing.T) {
	t.Parallel()

	emitMethod := func() *emit.Method {
		return &emit.Method{
			Name: "Get",
			Params: []*emit.Param{
				{Name: "ctx", Type: emit.External("context", "Context")},
				{Type: emit.Builtin("string")},
			},
			Returns: []*emit.Return{
				{Type: emit.Builtin("string")},
				{Type: emit.Builtin("error")},
			},
		}
	}

	t.Run("projects an emitted method the same way", func(t *testing.T) {
		t.Parallel()
		// A generator consuming upstream output needs the same
		// projection over a shape carrying no source node — which is
		// why the reference mock generator grew a private
		// intermediate to lower both onto.
		s := golang.SigOfEmit(emitMethod())
		if !slices.Equal(s.Idents(), []string{"ctx", "arg1"}) {
			t.Fatalf("Idents = %v, want [ctx arg1]", s.Idents())
		}
		if !slices.Equal(fieldNames(s), []string{"Result", "Err"}) {
			t.Fatalf("fields = %v, want [Result Err]", fieldNames(s))
		}
	})

	t.Run("finds the error slot on the emit side too", func(t *testing.T) {
		t.Parallel()
		// The emit layer carries no meta stamp, so the flag comes
		// from the reference's spelling alone.
		if !golang.SigOfEmit(emitMethod()).ReturnsError() {
			t.Fatalf("an emit-side error return must be recognised")
		}
	})

	t.Run("an emitted signature is always anonymous", func(t *testing.T) {
		t.Parallel()
		// A name on an emit.Return was chosen by the generator that
		// built it, not written by an author.
		if golang.SigOfEmit(emitMethod()).NamedReturns {
			t.Fatalf("an emit-side projection must not claim named returns")
		}
	})

	t.Run("carries the variadic marker", func(t *testing.T) {
		t.Parallel()
		m := &emit.Method{Name: "Print", Params: []*emit.Param{
			{Name: "args", Type: emit.Builtin("string"), Variadic: true},
		}}
		if !golang.SigOfEmit(m).Variadic() {
			t.Fatalf("an emit-side variadic must survive projection")
		}
	})
}

func TestSigAccessors(t *testing.T) {
	t.Parallel()

	t.Run("Taken reserves the receiver and every parameter", func(t *testing.T) {
		t.Parallel()
		// What a caller passes to UniqueIdent when introducing a name
		// of its own, so a helper cannot shadow the signature.
		s := golang.SigOf(getMethod())
		if !slices.Equal(s.Taken(), []string{"s", "ctx", "id"}) {
			t.Fatalf("Taken = %v, want [s ctx id]", s.Taken())
		}
	})

	t.Run("HasResults distinguishes a void method", func(t *testing.T) {
		t.Parallel()
		if golang.SigOf(&node.Method{Name: "Close"}).HasResults() {
			t.Fatalf("a method returning nothing must report no results")
		}
		if !golang.SigOf(getMethod()).HasResults() {
			t.Fatalf("a method returning values must report results")
		}
	})

	t.Run("IsGeneric reads the type-parameter list", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "F", TypeParams: params("T")}
		if !golang.SigOf(m).IsGeneric() {
			t.Fatalf("a parameterised method must read as generic")
		}
	})

	t.Run("a nil method projects to nil rather than panicking", func(t *testing.T) {
		t.Parallel()
		// A caller iterating a resolved method set that contains one
		// skips rather than crashes.
		if golang.SigOf(nil) != nil || golang.SigOfEmit(nil) != nil || golang.SigOfFunc(nil) != nil {
			t.Fatalf("a nil declaration must project to nil")
		}
	})

	t.Run("every accessor tolerates a nil projection", func(t *testing.T) {
		t.Parallel()
		var s *golang.Sig
		if s.HasResults() || s.ReturnsError() || s.IsGeneric() || s.Variadic() {
			t.Fatalf("a nil projection must answer false throughout")
		}
		if s.Idents() != nil || s.Locals() != nil || s.Taken() != nil || s.ErrReturn() != nil {
			t.Fatalf("a nil projection must yield nothing")
		}
	})
}

func TestSigEdges(t *testing.T) {
	t.Parallel()

	t.Run("a signature with no error has no error slot", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "F", Returns: []*node.Return{{Type: builtinRef("string")}}}
		if got := golang.SigOf(m).ErrReturn(); got != nil {
			t.Fatalf("ErrReturn = %+v, want nil", got)
		}
	})

	t.Run("a method reserves the default receiver", func(t *testing.T) {
		t.Parallel()
		if got := golang.SigOf(getMethod()).ReceiverIdent; got != golang.DefaultReceiverIdent {
			t.Fatalf("ReceiverIdent = %q, want %q", got, golang.DefaultReceiverIdent)
		}
	})

	t.Run("a nil projection reserves nothing", func(t *testing.T) {
		t.Parallel()
		// Taken is the nil-safe reader of the receiver identifier;
		// the field beside it is unguarded, as Name, Params and
		// Returns have always been.
		var s *golang.Sig
		if got := s.Taken(); len(got) != 0 {
			t.Fatalf("Taken on nil = %v, want nothing reserved", got)
		}
	})

	t.Run("a function projects its type parameters", func(t *testing.T) {
		t.Parallel()
		f := &node.Function{Name: "Map", TypeParams: params("T")}
		if !golang.SigOfFunc(f).IsGeneric() {
			t.Fatalf("a parameterised function must read as generic")
		}
	})

	t.Run("a nil parameter entry falls back positionally", func(t *testing.T) {
		t.Parallel()
		// A bridge or a hand-built fixture can leave a gap in the list.
		s := golang.SigOf(&node.Method{Name: "F", Params: []*node.Param{nil}})
		if s.Params[0].Name != "arg0" {
			t.Fatalf("Name = %q, want arg0", s.Params[0].Name)
		}
	})

	t.Run("a nil emit parameter falls back too", func(t *testing.T) {
		t.Parallel()
		s := golang.SigOfEmit(&emit.Method{Name: "F", Params: []*emit.Param{nil}, Returns: []*emit.Return{nil}})
		if s.Params[0].Name != "arg0" || len(s.Returns) != 1 {
			t.Fatalf("SigOfEmit = %+v", s)
		}
	})

	t.Run("an emit signature with several values numbers them", func(t *testing.T) {
		t.Parallel()
		m := &emit.Method{Name: "F", Returns: []*emit.Return{
			{Type: emit.Builtin("string")},
			{Type: emit.Builtin("int")},
			{Type: emit.Builtin("error")},
		}}
		if got := fieldNames(golang.SigOfEmit(m)); !slices.Equal(got, []string{"Result0", "Result1", "Err"}) {
			t.Fatalf("fields = %v", got)
		}
	})
}

func TestWithReceiverFromType(t *testing.T) {
	t.Parallel()

	t.Run("derives the receiver from the emitted type", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "Get"}
		got := golang.SigOf(m, golang.WithReceiverFromType("StoreStub")).ReceiverIdent
		if got != "s" {
			t.Fatalf("ReceiverIdent = %q, want s", got)
		}
	})

	t.Run("dodges a parameter that already binds the initial", func(t *testing.T) {
		t.Parallel()
		// The ordering a caller cannot resolve alone: the identifier is
		// the type's initial made unique against the parameters, and the
		// parameters are what the projection is producing. Without this
		// the emitted method reads `func (s *StoreStub) Do(s string)`,
		// where the parameter shadows the receiver.
		strRef := &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"}
		m := &node.Method{Name: "Do", Params: []*node.Param{{Name: "s", Type: strRef}}}
		got := golang.SigOf(m, golang.WithReceiverFromType("StoreStub")).ReceiverIdent
		if got == "s" {
			t.Fatal("receiver collides with the parameter it shares scope with")
		}
	})

	t.Run("takes precedence over a literal receiver", func(t *testing.T) {
		t.Parallel()
		// Honouring the literal would reinstate exactly the shadowing
		// the derived form exists to prevent.
		strRef := &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"}
		m := &node.Method{Name: "Do", Params: []*node.Param{{Name: "s", Type: strRef}}}
		got := golang.SigOf(m,
			golang.WithReceiverIdent("s"),
			golang.WithReceiverFromType("StoreStub"),
		).ReceiverIdent
		if got == "s" {
			t.Fatalf("ReceiverIdent = %q; the literal overrode the derived form", got)
		}
	})
}

// TestSig_ParamByField pins the reverse lookup from a recorded-call
// field to the parameter it came from.
//
// [golang.Param.Field] is this package's own projection, so a consumer
// holding a field name has no way back without repeating the
// derivation and trusting the two to agree.
func TestSig_ParamByField(t *testing.T) {
	t.Parallel()

	sig := golang.SigOf(&node.Method{
		Name: "Put",
		Params: []*node.Param{
			{Name: "key", Type: builtinRef("string")},
			{Name: "limit", Type: builtinRef("int")},
		},
	})

	t.Run("finds the parameter behind a field name", func(t *testing.T) {
		t.Parallel()
		got, ok := sig.ParamByField("Limit")
		if !ok {
			t.Fatalf("ParamByField(Limit) found nothing; fields are %v", fieldsOf(sig))
		}
		if got.Name != "limit" {
			t.Errorf("Name = %q, want the declared parameter identifier", got.Name)
		}
	})

	t.Run("an unknown field reports not found", func(t *testing.T) {
		t.Parallel()
		if _, ok := sig.ParamByField("Absent"); ok {
			t.Errorf("ParamByField(Absent) = true, want false")
		}
	})

	t.Run("a nil signature reports not found", func(t *testing.T) {
		t.Parallel()
		var nilSig *golang.Sig
		if _, ok := nilSig.ParamByField("Limit"); ok {
			t.Errorf("a nil Sig must find nothing")
		}
	})
}

// fieldsOf lists a signature's recorded-call field names, so a failure
// says what was available rather than only what was missing.
func fieldsOf(s *golang.Sig) []string {
	out := make([]string, 0, len(s.Params))
	for _, p := range s.Params {
		out = append(out, p.Field)
	}
	return out
}
