# enum

Generates an enumerated type's textual surface, plus a companion file
of checks over it.

```go
//+gen:enum
type Status int

const (
    StatusDraft Status = iota
    StatusActive
    StatusArchived
)
```

```go
s, err := ParseStatus("Active")   // errors.Is(err, ErrUnknownStatus) on anything else
s.String()                        // "Active"
s.IsValid()                       // true
StatusValues()                    // every declared variant, in order
```

## Why

A Go enum is a convention, not a language feature: a defined type and
a block of typed constants. Nothing stops a conversion admitting a
value outside the set, nothing notices when a variant is added without
the handling it needs, and nothing relates the type's textual form to
the values it was declared with. Each is a one-line mistake that
compiles.

## What is generated

Six members, each withheld where the type already declares it.

| Surface | Go spelling | |
|---|---|---|
| render | `String() string` | the textual form; an undeclared value renders as itself |
| parse | `ParseStatus(string)` | the inverse, plus `ErrUnknownStatus` |
| values | `StatusValues() []Status` | the declared set, a fresh slice per call |
| valid | `IsValid() bool` | the only thing between a conversion and the rest of the program |
| encode | `MarshalText()` | |
| decode | `UnmarshalText()` | |

Text rather than JSON. `encoding/json` reaches for `TextMarshaler` on
its own and so does every YAML library, and it is `MarshalText` that
makes the type legal as a map key — which a JSON marshaller alone does
not.

Each Go spelling comes from a word the Go binding declares, joined to
the type name by the language. The core composes no identifier and the
templates spell only the signatures around them.

### What rides with what

An author who wrote their own `String` meant to keep it, so a member
that exists is skipped silently. Two of the six are not members at
all, which changes how they are decided:

- **The parser and the set accessor sit beside the type**, so a
  hand-written one is invisible to the declaration. They ride with the
  renderer: a type keeping its own renderer almost always keeps its
  own parser, and generating one that shadows theirs is the worse
  guess.
- **The decoder is written in terms of the parser**, so it needs one
  to exist — the generated one, or the author's under the same derived
  name. With neither, the encoder goes too rather than shipping half a
  pair: a type that encodes as text and decodes from a number is what
  no author asks for.

`//+gen:enum methods=off` suppresses all six and leaves the checks,
for a surface already written by hand that only wants pinning.

## The textual form

For a numeric enum the identifier is the only textual form the
declaration carries, so `StatusActive` on `type Status int` renders as
`Active` — the type name is context wherever the value appears, and
repeating it is noise in every log line.

For a string enum the value *is* the textual form, and it is already
written down. `US Region = "us-east"` renders as `us-east` and parses
from it. Deriving `US` instead discards the only thing the declaration
said and breaks every value arriving from JSON, a database column or a
query parameter — while still round-tripping against itself, so a
check testing only the generated pair passes.

A `+gen:value <text>` directive on a variant overrides both, for the
case where the derived spelling clashes with a protocol's and the
derivation cannot be taught about it.

## What is refused

Three declarations produce no output at all, because generating
against them would be confidently false rather than merely incomplete:

- **An annotated enum with no variants.** There is nothing to generate
  against.
- **Two variants rendering alike.** The parser maps text to exactly
  one variant, so a collision makes one unreachable through it — and
  the generated round trip then fails with no indication of the cause.
- **Variants declared outside the type's own package.** Legal Go, and
  invisible to the projection: the validity test would reject a value
  someone declared, and the arity check would pin a count that is not
  the truth. Reaching across the boundary instead would make the
  generated set depend on which packages a run happened to include.

## The checks

The second output drives the surface the way a consumer does — the
`_test.go` ending puts it in the external test package — and asserts
only what the declaration earns:

- **The arity**, so a variant added without the handling it needs is
  something a reviewer is made to notice.
- **Each variant holds a distinct value**, and — where a renderer
  exists — **renders distinctly**. Two names for one value make one
  branch unreachable and the compiler has nothing to say about it.
- **A string enum renders the value it declares.** The one assertion
  that tells a correct derivation from one taking the identifier:
  every other check here passes for both.
- **The round trip through text**, and the same through the encoding
  pair.
- **Text naming no variant is refused**, with the declared refusal.
- **A value outside the set is invalid and renders distinctly**,
  probed with a value the projection derived and checked for collision
  against the declared set.
- **The zero is what it claims** — a declared variant, or none. The
  two cases read as opposite assertions, and which one an enumeration
  earns is what the check exists to tell apart.

A check whose probe could not be derived is dropped rather than
written against a guess. A float-valued set is the case: the values
are read as integers and the bounds of no float type are known, so no
successor can be named, and the two checks needing one go. Everything
else a float set earns is unaffected.

Assertions render through the backend's assertion dialect, so the
generated file depends on nothing but `testing`. Swapping in a helper
library is an override of nine funcmap names rather than a fork of
these templates — see [the dialect](../../../lang/golang/assert.go).

Turn the file off with `no-tests`.

## Composing with it

Two slots, and two stamps.

| Slot | On | For |
|---|---|---|
| `surface` | the API file | a member this plugin does not model — a database codec pair, a flag binding, a schema description |
| `checks` | the check file | an assertion this plugin cannot see — a wire format the set has to stay compatible with |

`enum.parse` and `enum.sentinel` are stamped on the annotated
declaration, carrying the identifiers this run generated. A generator
constructing a value from text reads them rather than re-deriving the
convention, which would break the moment a run configures a different
word.

## Options

| Option | Default | |
|---|---|---|
| `parse-word` | `Parse` | the word the parse function's identifier carries |
| `sentinel-word` | `Unknown` | the subject the refusal is named for |
| `no-tests` | `false` | suppress the companion check file |

Each is a *word*, not a spelling: the language joins it to the type
name in its own convention, and spells the refusal through its own
error convention on top.

## Where the output lands

Beside its source, and it cannot be routed elsewhere: the surface
declares methods on the enumeration's type, which Go permits only in
that type's own package. An `out` directive sending it away produces a
file naming an undefined type. The checks travel with it and take the
external test package the `_test.go` ending gives them.

## Limits

- **A float-valued set gets no boundary checks**, for the reason
  above.
- **The `value` override is not checked against the protocol it
  exists for.** A directive pinning text that collides with another
  variant's is caught; one pinning text that no consumer sends is not
  something this plugin can see.
- **A second language needs its own words.** The core names none, but
  a language that declares no `Words` entry for a surface member
  composes its identifier from the type name alone — which is legal
  and almost certainly not what was meant.
