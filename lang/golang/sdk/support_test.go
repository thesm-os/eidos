// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"maps"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// TestDialectNamesReexported pins the façade against the dialect it
// re-exports.
//
// The re-export exists so a plugin replacing the dialect names the
// entries through this package rather than reaching past it into the
// language package. An entry added there and not here sends the next
// author back to the import this package exists to remove — and they
// will not notice, because reaching past it compiles.
func TestDialectNamesReexported(t *testing.T) {
	t.Parallel()

	reexported := []string{
		sdkgo.FuncAssertEqual, sdkgo.FuncAssertDeepEqual,
		sdkgo.FuncAssertNotEqual, sdkgo.FuncAssertTrue,
		sdkgo.FuncAssertFalse, sdkgo.FuncAssertNil,
		sdkgo.FuncAssertNotNil, sdkgo.FuncAssertLen,
		sdkgo.FuncAssertNoError, sdkgo.FuncAssertError,
		sdkgo.FuncNeedsDiffHelper,
	}

	t.Run("every dialect entry is named here", func(t *testing.T) {
		t.Parallel()
		want := slices.Sorted(maps.Keys(golang.AssertFuncMap()))
		got := slices.Sorted(slices.Values(reexported))
		if slices.Equal(got, want) {
			return
		}
		t.Errorf("the re-export has drifted from the dialect;\n"+
			"  in the dialect but not re-exported: %v\n"+
			"  re-exported but not in the dialect: %v",
			missing(want, got), missing(got, want))
	})

	t.Run("each name matches the dialect's own", func(t *testing.T) {
		t.Parallel()
		// A re-export bound to the wrong constant registers a
		// replacement under a name nothing calls, and the plugin's
		// own templates go on rendering the default.
		if sdkgo.FuncAssertEqual != golang.FuncAssertEqual {
			t.Errorf("FuncAssertEqual = %q, want %q",
				sdkgo.FuncAssertEqual, golang.FuncAssertEqual)
		}
		if sdkgo.FuncNeedsDiffHelper != golang.FuncNeedsDiffHelper {
			t.Errorf("FuncNeedsDiffHelper = %q, want %q",
				sdkgo.FuncNeedsDiffHelper, golang.FuncNeedsDiffHelper)
		}
	})
}

// missing returns the elements of a that b does not contain.
func missing(a, b []string) []string {
	var out []string
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}

// resolverTable answers from a fixed table, standing in for the
// store-backed reader a plugin is handed.
type resolverTable map[string]sdk.Node

func (r resolverTable) Resolve(t *sdk.TypeRef) (sdk.Node, bool) {
	n, ok := r[golang.QName(t)]
	return n, ok
}

// TestMemberLookupForwards pins the façade against the language
// package for the two calls a generator aiming emitted code at a
// struct's members makes.
//
// A forwarder that answered from its own walk would drift from Go's
// promotion rules the moment the language package's changed, and the
// drift is invisible: both spellings compile and both return a type.
func TestMemberLookupForwards(t *testing.T) {
	t.Parallel()

	base := &sdk.Struct{
		Name: "Base", Package: "x",
		Fields: []*sdk.Field{{Name: "Version", Type: &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "int"}}},
	}
	user := &sdk.Struct{
		Name: "User", Package: "x",
		Embeds: []*sdk.Embed{{Type: &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Package: "x", Name: "Base"}}},
	}
	ref := &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Package: "x", Name: "Base"}
	r := resolverTable{"x.Base": base}

	t.Run("StructOf answers what the language package does", func(t *testing.T) {
		t.Parallel()
		got, ok := sdkgo.StructOf(ref, r)
		want, wantOK := golang.StructOf(ref, r)
		if got != want || ok != wantOK {
			t.Fatalf("StructOf = %v, %v; want %v, %v", got, ok, want, wantOK)
		}
		if !ok {
			t.Fatal("StructOf resolved nothing, so the comparison proved nothing")
		}
	})

	t.Run("MemberField reaches a promoted member", func(t *testing.T) {
		t.Parallel()
		// Through the embed, which is the half a forwarder reading
		// s.Fields would silently lose.
		got, ok := sdkgo.MemberField(user, "Version", r)
		if !ok || got.Name != "int" {
			t.Fatalf("MemberField = %v, %v; want int", got, ok)
		}
	})

	t.Run("MemberField answers what the language package does", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"Version", "Nonesuch"} {
			got, ok := sdkgo.MemberField(user, name, r)
			want, wantOK := golang.MemberField(user, name, r)
			if got != want || ok != wantOK {
				t.Errorf("MemberField(%q) = %v, %v; want %v, %v", name, got, ok, want, wantOK)
			}
		}
	})
}

// TestHandleLookupForwards pins the interface pair the way
// [TestMemberLookupForwards] pins the struct pair: forwarding, the
// embed walk, and agreement with the language package.
func TestHandleLookupForwards(t *testing.T) {
	t.Parallel()

	closer := &sdk.Interface{
		Name: "Closer", Package: "x",
		Methods: []*sdk.Method{{Name: "Close"}},
	}
	cursor := &sdk.Interface{
		Name: "Cursor", Package: "x",
		Methods: []*sdk.Method{{Name: "Next"}},
		Embeds:  []*sdk.Embed{{Type: &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Package: "x", Name: "Closer"}}},
	}
	ref := &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Package: "x", Name: "Cursor"}
	r := resolverTable{"x.Cursor": cursor, "x.Closer": closer}

	t.Run("InterfaceOf answers what the language package does", func(t *testing.T) {
		t.Parallel()
		got, ok := sdkgo.InterfaceOf(ref, r)
		want, wantOK := golang.InterfaceOf(ref, r)
		if got != want || ok != wantOK {
			t.Fatalf("InterfaceOf = %v, %v; want %v, %v", got, ok, want, wantOK)
		}
		if !ok {
			t.Fatal("InterfaceOf resolved nothing, so the comparison proved nothing")
		}
	})

	t.Run("MemberMethod reaches a method through an embed", func(t *testing.T) {
		t.Parallel()
		// The half a forwarder reading i.Methods would silently lose.
		got, ok := sdkgo.MemberMethod(cursor, "Close", r)
		if !ok || got.Name != "Close" {
			t.Fatalf("MemberMethod = %v, %v; want the embedded Close", got, ok)
		}
	})

	t.Run("MemberMethod answers what the language package does", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{"Next", "Close", "Nonesuch"} {
			got, ok := sdkgo.MemberMethod(cursor, name, r)
			want, wantOK := golang.MemberMethod(cursor, name, r)
			if got != want || ok != wantOK {
				t.Errorf("MemberMethod(%q) = %v, %v; want %v, %v", name, got, ok, want, wantOK)
			}
		}
	})
}
