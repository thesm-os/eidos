# witness

Names the concrete type a generated entry point instantiates each of a
declaration's type parameters at, where a language cannot derive one.

```go
//+gen:witness T=int
//+gen:builder
type Sorted[T constraints.Ordered] struct{ Items []T }
```

## Why

A generated check is an ordinary function, and an ordinary function
cannot take type parameters. So anything exercising a generic
declaration has to name concrete types somewhere, and a language
derives them only where the constraint's type set is knowable without
loading the package that declares it.

In Go that is `any` and `comparable`. A parameter bounded by
`constraints.Ordered`, or by a project's own interface, is a reference
into a package the generator never read: nothing can be derived, and
the whole declaration gets no checks at all. The rendered file carries
a note saying so, which is the honest outcome — but it is a note, not
a test.

The directive is what turns it back into one. Only an author knows
which admissible type is representative.

## What it writes

One pair of meta keys per parameter, on the parameter itself:

- `witness.type` — the type's identifier.
- `witness.type.package` — the package it resolves in, absent for a
  type needing no import.

The package is absent for a bare name, which is the difference from
`+gen:sample`. A sample names a function, and a function always lives
somewhere; a witness is commonly a builtin whose spelling needs no
import, and qualifying `int` would render an import nothing resolves.

A qualified name or a full import path reaches a type elsewhere, and
resolves against the imports of the file that declared the parameter.

## What reads it

Nothing here is read by a generator. `SourceRules.Witnesses` prefers an
authored witness over one it would derive, so every consumer that asks
a language which types to instantiate at gets the authored answer
without knowing this plugin exists.

That is the same arrangement `+gen:sample` uses, for the same reason: a
stamp each generator has to remember to prefer is a stamp two of them
will forget.

## All or nothing

An entry point instantiates a declaration's whole parameter list at
once, so a witness for one parameter is worth nothing without one for
the rest. Naming some and leaving others to a constraint that admits no
derivation leaves the declaration with no witnesses at all — and its
checks still withheld.

Naming one parameter and leaving another whose constraint *is*
derivable is fine: the derived answer stands for that one.
