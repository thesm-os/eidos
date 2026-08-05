// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package stubgen generates a recording test double for every
// interface annotated `+gen:stub`, plus a companion test file that
// proves the double satisfies the interface it stands in for.
//
// It is the reference tier's worked example of multi-output
// generation. Every other plugin under `reference/` emits a single
// file; the two-output combination — a tagged companion, a
// plugin-defined emit kind per output, a template per kind, and
// routing that can redirect either file independently — is
// otherwise demonstrated only in `plugins/generator`, where it sits
// behind a language-adapter split and an options surface that a
// reader copying the pattern does not need.
//
// # Relationship to mockgen
//
// The two overlap and the split is deliberate. [mockgen] emits the
// minimal func-valued double: one `<Method>Func` field per method
// and a body that delegates to it, nothing more. stubgen records —
// each call appends a struct carrying the arguments and the
// returned values, so a test can assert on what happened rather
// than only on what was returned.
//
// Reach for mockgen when the test controls behaviour and asserts on
// results. Reach for stubgen when the test asserts on the
// interaction: that a call happened, how many times, with what.
//
// # Directive
//
// A source interface opts in with `+gen:stub`. The directive takes
// no positional argument and denies the negated form — a stub
// exists exactly where one is declared, so removing the line is the
// suppression.
//
//	//+gen:stub
//	type Store interface { ... }
//
// # Output set
//
// Two outputs flow from one annotated interface:
//
//   - Primary, untagged, suffix `_stub.go`. Hosts the [KindStub]
//     emit value, rendered by `stub.impl.tmpl`. Declares the
//     source package, so the double is importable by other
//     packages' tests rather than trapped in this one.
//   - Tagged `test`, suffix `_stub_test.go`. Hosts the
//     [KindStubTests] value, rendered by `stub.test.tmpl`. The
//     `_test.go` ending triggers the framework's automatic
//     `<pkg>_test` package shift, so the generated test cannot
//     read package-private state.
//
// The tag is what makes the two independently routable. A source
// author redirects one without the other through
// `+gen:out tag=test <path>`, project config under the plugin's
// `tags:` block, or the CLI `-o stubgen:test=<path>`.
//
// Both emit values append into the file's `top` slot — the region
// between the package clause and the first core decl, which is the
// natural placement for a block of whole declarations rendered by
// one template.
//
// # Worked example
//
// Given `users/store.go`:
//
//	package users
//
//	//+gen:stub
//	type Store interface {
//	    Get(ctx context.Context, id string) (item User, err error)
//	    Put(ctx context.Context, u User) error
//	}
//
// The primary output lands at `users/store_stub.go`:
//
//	type StoreGetCall struct {
//	    Ctx  context.Context
//	    ID   string
//	    Item User
//	    Err  error
//	}
//
//	type StoreStub struct {
//	    GetFunc func(ctx context.Context, id string) (User, error)
//	    PutFunc func(ctx context.Context, u User) error
//
//	    GetCalls []StoreGetCall
//	    PutCalls []StorePutCall
//	}
//
//	func (s *StoreStub) Get(ctx context.Context, id string) (item User, err error) {
//	    item, err = s.GetFunc(ctx, id)
//	    s.GetCalls = append(s.GetCalls, StoreGetCall{Ctx: ctx, ID: id, Item: item, Err: err})
//	    return item, err
//	}
//
// and the companion at `users/store_stub_test.go`, in `users_test`:
//
//	var _ users.Store = (*users.StoreStub)(nil)
//
//	func TestStoreStubRecordsGet(t *testing.T) { ... }
//
// # Recorded-call field names
//
// `Item` and `Err` above are not positional placeholders — they are
// the source's declared return names, read from
// [node.Return.Name]. A signature written `(item User, err error)`
// documents what its returns mean, and a recorded-call struct is
// the main consumer of that documentation.
//
// Returns without declared names fall back to `Result0`, `Result1`,
// … positionally. The blank identifier counts as unnamed, since `_`
// cannot be used as a field name.
//
// # Named returns on the generated methods
//
// The generated method carries the source's return names on its own
// signature — `(item User, err error)`, not `(User, error)` — so the
// documentation the interface author wrote survives into the double.
// The body assigns with `=` rather than `:=`, since named results
// are already declared, and returns explicitly rather than bare: a
// naked return in generated code reads as an omission.
//
// Propagation is all-or-nothing, and falls back to unnamed returns
// when either condition fails:
//
//   - Every return must carry a name. Go's grammar requires a
//     signature's results to be all named or all anonymous, and the
//     emit layer enforces it — a mixed slice fails the render with
//     [emit.ErrMixedNamedReturns]. A source signature can produce a
//     mixed slice legitimately: `(_ User, err error)` is valid Go,
//     and the blank normalises to unnamed, so the model holds one
//     named and one unnamed slot.
//   - No return name may collide with the receiver identifier or
//     with any parameter name. `func (s *T) F(item int) (item int)`
//     does not compile, and the stub cannot rename around it
//     without breaking the correspondence the names exist to carry.
//
// Falling back costs documentation on the generated signature and
// nothing else — the recorded-call struct keeps its derived field
// names either way, because those are per-return and have no
// all-or-nothing constraint.
//
// # Options
//
// `Suffix` overrides the stub type's name suffix (default `Stub`,
// producing `StoreStub`).
//
// Recording is not optional. A stub that records nothing is
// [mockgen], and offering the toggle would leave two reference
// plugins whose difference is a config value rather than a purpose.
//
// # A nil func field panics
//
// The generated method calls its `<Method>Func` field
// unconditionally, so a stub used without assigning the field for a
// method under test panics with a nil dereference rather than
// returning zero values. This is the same semantic mockgen has.
//
// It follows from recording: the method must invoke the func,
// capture what came back, append the record, and only then return.
// A stub that tolerated a nil field would have to invent return
// values, and inventing them is what makes a double lie about the
// system under test. The panic names the unassigned field, which is
// the fastest available diagnosis.
//
// # Imports
//
// Every cross-package and stdlib reference is expressed as an
// [sdk.NewExternal] expression rather than a hard-coded import in
// the template. The Go backend's `renderExpr` funcmap registers the
// referenced package on the rendered file's import set, so the two
// templates carry no import blocks and the same source renders
// correctly whether the double lands beside its source or in a
// centralised directory.
//
// The companion test is in an external test package, so identifiers
// from the source package qualify through the same mechanism.
package stubgen
