# sentinel

Generates the checks that hold a package's error contract to what its
declarations promise. It writes no production code — the author
declares the errors, this plugin asserts their invariants.

```go
//+gen:sentinel
package auth

var (
    ErrNotFound = errors.New("auth: user not found")
    ErrConflict = errors.New("auth: user already exists")
)

type ValidationError struct {
    Field string
    Cause error
}
```

The directive is on the **package**, not on each declaration: what is
being asserted is mostly about how the errors relate to each other,
and no single declaration can opt into that.

## Why

Callers match on these values, read them in logs and branch on them,
so their messages and identities are as much an API as any exported
signature — and nothing in the compiler holds them to it. Two values
that match each other collapse a caller's branches. A wrapper that
drops its cause truncates every chain it takes part in. A message that
omits a field the type carries hides the one detail the struct exists
to record. Each is invisible from the declarations.

## What is found

**Declared error values** are found by the language's own naming
convention — `Err*` in Go — because a value's declared type says
nothing here: every one of them is the same interface, so the name is
what marks one as part of the contract. The generated file lists what
it covers, so an error named otherwise is visible by its absence.

**Error types** are matched on the protocol, not on a name. A type
declaring `Error()` with no result is not an error, and a check
calling it as one does not compile — which puts the failure in the
consuming repository rather than a diagnostic in this one.

The contract is read through whatever the declaration folds in. This
is the shape the old reading missed:

```go
type BaseError struct{ Op string; Cause error }
func (e *BaseError) Error() string { … }
func (e *BaseError) Unwrap() error { return e.Cause }

type NotFoundError struct{ BaseError }   // declares nothing, and is an error
```

Reading only what `NotFoundError` writes finds no message method at
all, so the package's directive says its errors are a contract and the
generated file covers half of them, silently.

The **member** set is *not* walked the same way, and the asymmetry is
deliberate: a promoted member is named by a selector, and a selector
is not a composite-literal key. The cause is the exception — the check
that needs it assigns rather than builds a literal, and promotion
makes that legal however deep the member sits. A cause behind a
*pointer* embed still refuses, because the zero of an embedded pointer
is nil and assigning through it panics on a type whose contract is
fine.

## What is asserted

Over the package's declared errors:

- **Each is assigned.** A var declared and never assigned is nil,
  compiles, and matches nothing — so every caller's branch on it is
  dead.
- **Each carries a message**, and **each carries the package prefix**.
  A message without one reads in a log exactly like one from any other
  package.
- **No two share a message**, and **no one's message begins with
  another's.** The second is a separate check: one message starting
  with another makes every search for the shorter match the longer.
- **No two match each other**, and **none matches one in a package
  named by `+gen:sentinel-no-overlap-with`.**

Over each error type:

- **Its zero value reports a message.** A message reading through a
  member the zero leaves unset panics — replacing the error a caller
  needed to report with a crash inside whatever was already going
  wrong.
- **It carries the package prefix**, for the same reason a declared
  value does.
- **It does not match the package's other error types.**
- **Where it compares itself and names no cause**, the standard
  comparison reaches that method. Both halves of that condition are
  load-bearing — see below.
- **Where it names a cause, it exposes it**, asserted by unwrapping
  rather than comparing.
- **Every member it carries reaches its message**, for the members
  whose value a message carries unchanged.

## What is deliberately not asserted

Three checks a reader might expect are absent, and each absence is
stated in the generated file rather than left as a gap:

- **That a value survives wrapping.** Identity is compared before
  anything a type declares is consulted, so every value passes —
  including one whose own comparison always refuses. The assertion
  would be about the standard library.
- **That a type can be recovered from a chain.** Recovery finds a
  value by assignability, so it succeeds for any type reachable
  through the chain and fails for none.
- **That the comparison agrees, on a type declaring a cause as well.**
  The standard comparison walks the chain, so a type exposing the same
  cause matches regardless of whether its own method was consulted —
  and the two sides then agree for every receiver form, including the
  one the check exists to catch. What the cause owes is checked
  instead.

A member whose value a message *formats* is written into the subject
but not asserted on. The width, base and precision a format applies
are not visible here, so asserting that `42` appears in the output
fails against a type reporting the same number perfectly well as
`042`.

## The assertion style

Single-subject assertions render through the backend's assertion
dialect, so the generated file depends on nothing but `testing`,
`errors`, `fmt` and `strings`. Swapping in a helper library is an
override of nine funcmap names rather than a fork of these templates —
see [the dialect](../../../lang/golang/assert.go).

Assertions inside a loop are written out instead. The dialect's
message is a literal, and a failure over a set has to name which
member of it broke — a report saying only "two sentinels match" sends
the reader to compare every pair by hand. The pairs are not unrolled
to get the literal back, because the count is quadratic in a number
this plugin does not bound.

## Composing with it

One slot: `checks`, on the check file, after the checks this plugin
derives. For an assertion it cannot see — an error that has to keep
matching one in a package it does not name, or a message whose wording
a wire format pins.

## Options and directives

| | | |
|---|---|---|
| `+gen:sentinel` | on the package | opts in |
| `+gen:sentinel prefix=<value>` | | overrides the expected message prefix |
| `+gen:sentinel prefix=off` | | suppresses the prefix check |
| `+gen:sentinel-no-overlap-with <path>` | on the package, repeats | names a neighbour this package's errors must stay distinct from |
| `prefix` (option) | | the repository's default prefix; a per-package directive still wins |

`prefix=off` withholds the check rather than comparing against the
empty string: every string begins with the empty string, so the check
would pass for any input and read as though the contract had been
examined.

## What is refused

- **A package with neither a declared error nor an error type.** The
  directive says its errors are a contract, and a file asserting
  nothing about an empty set reads as though they had been checked.
- **A no-overlap directive naming its own package.** Every value
  matches itself, so the check could only ever fail.

A no-overlap directive naming a package the run did not load is
*warned* rather than refused, and the empty check is emitted: a
directive pointing at a package with no errors should read as an empty
check rather than as one that was never generated.

## Limits

- **A member reached through an embed is not written into the
  message check.** Only the cause is, and only where the embed is not
  a pointer.
- **The message check tests containment, not formatting.** A message
  that names a member and mangles it passes.
- **Errors declared in a package the run did not load are invisible**
  to the cross-package check, which is why the run warns about one.
