// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/golangtest"
)

func TestTypes(t *testing.T) {
	t.Parallel()

	t.Run("finds a declared type", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertType(t, "StoreStub")
	})

	t.Run("reports an absent one against what is there", func(t *testing.T) {
		t.Parallel()
		// The point of going structural: a substring failure says a
		// substring is missing, and this says what the file does declare.
		s := probe(t)
		parse(t).AssertType(s, "Absent")
		if !s.failed || !strings.Contains(s.msg, "StoreGetStub") {
			t.Fatalf("message %q does not list the declared types", s.msg)
		}
	})

	t.Run("rejects a type that must not be there", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertNoType(t, "Absent")
		s := probe(t)
		parse(t).AssertNoType(s, "StoreStub")
		if !s.failed {
			t.Fatal("AssertNoType accepted a declared type")
		}
	})
}

func TestFuncsAndMethods(t *testing.T) {
	t.Parallel()

	t.Run("finds a plain function", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertFunc(t, "NewStoreStub")
	})

	t.Run("says when the caller asked for a method as a function", func(t *testing.T) {
		t.Parallel()
		// The commonest way to hold this wrong, and the message that
		// turns it into a one-line fix.
		s := probe(t)
		parse(t).AssertFunc(s, "Get")
		if !s.failed || !strings.Contains(s.msg, "does declare the method StoreStub.Get") {
			t.Fatalf("message %q does not point at the method", s.msg)
		}
	})

	t.Run("finds a method whatever its receiver form", func(t *testing.T) {
		t.Parallel()
		// A caller asking whether the setter exists does not mean to pin
		// the receiver form; AssertPointerReceiver is for when they do.
		parse(t).AssertMethod(t, "StoreStub", "Get")
		parse(t).AssertMethod(t, "StoreStub", "Name")
	})

	t.Run("lists the receiver's methods when one is absent", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertMethod(s, "StoreStub", "Absent")
		if !s.failed || !strings.Contains(s.msg, "[Close Get Name]") {
			t.Fatalf("message %q does not list the receiver's methods", s.msg)
		}
	})

	t.Run("rejects a method that must not be there", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertNoMethod(t, "StoreStub", "String")
		s := probe(t)
		parse(t).AssertNoMethod(s, "StoreStub", "Get")
		if !s.failed {
			t.Fatal("AssertNoMethod accepted a declared method")
		}
	})
}

func TestSignature(t *testing.T) {
	t.Parallel()

	t.Run("compares free of formatting", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertMethod(t, "StoreStub", "Get").
			Signature(t, "(ctx context.Context, id string) (string, error)")
	})

	t.Run("reports the signature it found", func(t *testing.T) {
		t.Parallel()
		// What is actually true nine times in ten is that the declaration
		// exists with a different signature, and only saying so turns the
		// failure into a fix rather than a diff-read.
		s := probe(t)
		parse(t).AssertMethod(t, "StoreStub", "Get").Signature(s, "(id string) error")
		const declared = "(ctx context.Context, id string) (string, error)"
		if !s.failed || !strings.Contains(s.msg, declared) {
			t.Fatalf("message %q does not carry the real signature", s.msg)
		}
	})

	t.Run("chains off an absent declaration without panicking", func(t *testing.T) {
		t.Parallel()
		// The lookup already reported; returning nil would turn one
		// failure into a panic in the next chained call.
		s := probe(t)
		parse(t).AssertMethod(s, "StoreStub", "Absent").Signature(s, "()")
	})

	t.Run("declines a type, which has no signature", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertType(t, "StoreStub").Signature(s, "()")
		if !s.failed || !strings.Contains(s.msg, "no signature") {
			t.Fatalf("message %q", s.msg)
		}
	})
}

func TestReceiverForm(t *testing.T) {
	t.Parallel()

	t.Run("pins a pointer receiver", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertMethod(t, "StoreStub", "Get").AssertPointerReceiver(t, true)
	})

	t.Run("pins a value receiver", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertMethod(t, "StoreStub", "Name").AssertPointerReceiver(t, false)
	})

	t.Run("reports the form it found", func(t *testing.T) {
		t.Parallel()
		// An Error or an Is on the wrong form is never consulted, and the
		// type silently behaves as though it declared nothing.
		s := probe(t)
		parse(t).AssertMethod(t, "StoreStub", "Name").AssertPointerReceiver(s, true)
		if !s.failed || !strings.Contains(s.msg, "declared on a value receiver") {
			t.Fatalf("message %q", s.msg)
		}
	})

	t.Run("declines a declaration with no receiver", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertFunc(t, "NewStoreStub").AssertPointerReceiver(s, true)
		if !s.failed || !strings.Contains(s.msg, "no receiver") {
			t.Fatalf("message %q", s.msg)
		}
	})
}

func TestFieldsAndEmbeds(t *testing.T) {
	t.Parallel()

	t.Run("compares a field free of column padding", func(t *testing.T) {
		t.Parallel()
		// The fixture pads `OnGet` to align with a longer neighbour. A
		// substring assertion carries that padding and breaks when an
		// unrelated field changes the alignment.
		parse(t).AssertField(t, "StoreGetStub", "OnGet", "*StoreGetStub")
	})

	t.Run("reports the type it found", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertField(s, "StoreGetStub", "OnGet", "*WrongType")
		if !s.failed || !strings.Contains(s.msg, `is "*StoreGetStub"`) {
			t.Fatalf("message %q does not carry the real type", s.msg)
		}
	})

	t.Run("lists the fields when one is absent", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertField(s, "StoreGetStub", "Absent", "string")
		if !s.failed || !strings.Contains(s.msg, "hidden string") {
			t.Fatalf("message %q does not list the fields", s.msg)
		}
	})

	t.Run("declines a type that is not a struct", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertField(s, "Absent", "X", "string")
		if !s.failed {
			t.Fatal("AssertField accepted an absent type")
		}
	})

	t.Run("finds an embed", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertEmbeds(t, "StoreGetStub", "io.Closer")
	})

	t.Run("lists the embeds when one is absent", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertEmbeds(s, "StoreGetStub", "io.Reader")
		if !s.failed || !strings.Contains(s.msg, "io.Closer") {
			t.Fatalf("message %q does not list the embeds", s.msg)
		}
	})

	t.Run("declines a type that can embed nothing", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertEmbeds(s, "Absent", "io.Closer")
		if !s.failed {
			t.Fatal("AssertEmbeds accepted an absent type")
		}
	})
}

func TestDoc(t *testing.T) {
	t.Parallel()

	t.Run("reads a type's doc comment", func(t *testing.T) {
		t.Parallel()
		// Generated documentation is output too: a plugin saying why a
		// check is absent is answering a reader who came looking for it.
		parse(t).AssertType(t, "StoreStub").AssertDoc(t, "stands in for Store")
	})

	t.Run("reports the doc it found", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		parse(t).AssertType(t, "StoreStub").AssertDoc(s, "absent sentence")
		if !s.failed || !strings.Contains(s.msg, "stands in for Store") {
			t.Fatalf("message %q does not carry the doc", s.msg)
		}
	})
}

func TestNoFunc(t *testing.T) {
	t.Parallel()

	t.Run("rejects a function that must not be there", func(t *testing.T) {
		t.Parallel()
		parse(t).AssertNoFunc(t, "Absent")
		s := probe(t)
		parse(t).AssertNoFunc(s, "NewStoreStub")
		if !s.failed {
			t.Fatal("AssertNoFunc accepted a declared function")
		}
	})
}

func TestDeclEdges(t *testing.T) {
	t.Parallel()

	// shapes carries the declaration kinds the lookups have arms for
	// but the primary fixture does not hold.
	shapes := func(t *testing.T) *golangtest.Source {
		t.Helper()
		return golangtest.Parse(t, golangtest.File{
			Path: "shapes.go", Src: []byte(
				"package x\n\n" +
					"// Reader is documented on its own spec.\n" +
					"type (\n\t// Reader reads.\n\tReader interface{ io.Closer }\n)\n\n" +
					"type Alias = string\n\n" +
					"// Do is documented on the func.\nfunc Do() {}\n\n" +
					"func External()\n"),
		})
	}

	t.Run("finds an interface's embed", func(t *testing.T) {
		t.Parallel()
		shapes(t).AssertEmbeds(t, "Reader", "io.Closer")
	})

	t.Run("declines a type that can embed nothing", func(t *testing.T) {
		t.Parallel()
		// An alias has no member list, so the question has no answer
		// rather than a negative one.
		s := probe(t)
		shapes(t).AssertEmbeds(s, "Alias", "io.Closer")
		if !s.failed || !strings.Contains(s.msg, "not a struct or interface") {
			t.Fatalf("message %q", s.msg)
		}
	})

	t.Run("declines a field lookup on a type that is not a struct", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		shapes(t).AssertField(s, "Alias", "X", "string")
		if !s.failed || !strings.Contains(s.msg, "not a struct") {
			t.Fatalf("message %q", s.msg)
		}
	})

	t.Run("reads a doc comment from the spec rather than the group", func(t *testing.T) {
		t.Parallel()
		shapes(t).AssertType(t, "Reader").AssertDoc(t, "Reader reads")
	})

	t.Run("reads a function's own doc comment", func(t *testing.T) {
		t.Parallel()
		shapes(t).AssertFunc(t, "Do").AssertDoc(t, "documented on the func")
	})

	t.Run("reports a declaration carrying no doc at all", func(t *testing.T) {
		t.Parallel()
		s := probe(t)
		shapes(t).AssertType(t, "Alias").AssertDoc(s, "anything")
		if !s.failed {
			t.Fatal("AssertDoc accepted an undocumented declaration")
		}
	})

	t.Run("narrows to a declaration with no body", func(t *testing.T) {
		t.Parallel()
		// An assembly or linkname declaration is legal Go and has no
		// body to search.
		if got := shapes(t).InFunc(t, "External").Body(); got != "" {
			t.Fatalf("Body = %q, want empty", got)
		}
	})
}
