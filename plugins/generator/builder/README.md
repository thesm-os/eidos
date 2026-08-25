# builder

Generates a fluent builder for an annotated type, plus a companion
file of checks over it.

```go
//+gen:builder
type Order struct {
    ID      string
    Lines   []string
    Totals  map[string]int
    Note    *string
}
```

```go
o := NewOrder().
    WithID("ord-1").
    AppendLines("widget").
    WithTotalsEntry("widget", 500).
    WithNote("gift wrap").
    Build()
```

## Why

A composite literal restates every member at every call site. Add a
member to `Order` and every literal breaks at once; read one and you
cannot tell which members that call actually cares about — they are
all present, so none of them stands out.

A builder inverts that. The constructor supplies the rest and each
call states only what it varies, so a test reads as the one fact it is
about and a new member changes nothing that did not mention it.

## What a member owes

The setters a member gets follow the *shape* of its type, not its
name. Shapes are the language-neutral vocabulary `sdk.TypeShape`
defines; which of a language's spellings maps to which shape is that
language's business, answered through `sdk.SourceRules`.

| Shape | Go spelling | Setters |
|---|---|---|
| scalar | anything else | `WithX` |
| sequence | `[]T` | `WithX(...T)`, `AppendX(...T)` |
| bytes | `[]byte`, `[]uint8` | `WithX([]byte)`, `WithXString(string)` |
| mapping | `map[K]V` | `WithX`, `WithXEntry(k, v)`, `WithXEntries(map)` |
| set | `map[K]struct{}` | `WithX`, `WithXEntry(k)`, `WithXEntries(...k)` |
| optional | `*T` | `WithX(T)` — takes the value and addresses it |

Three of those exist because the obvious single setter is wrong:

- **A set is a map**, so it is classified first. Read as a mapping, its
  entry setter would ask the caller for a `struct{}` — the one thing
  they cannot vary.
- **A byte sequence can also be spelled as text.** That conversion is
  available for this element type and no other, and a caller holding a
  string should not write it at every call site.
- **An optional distinguishes unset from zero.** A setter demanding an
  address would make every caller allocate before they can say "set".
  Clearing it goes through `Mutate`.

`With`, `Append`, `Entry` and the PascalCase joining them are Go's
conventions, declared as words in the plugin's Go binding. The core
composes each identifier from them without naming a language, which is
what lets it notice two members reaching one setter before the file is
written.

## Seeding

Three sources, in order of specificity:

1. **A companion** — `OrderDefaults() Order` beside the type, found by
   convention. Configure the word with `companion-word`.
2. **A declared default** per member — `//+gen:default 8080` or
   `` `default:"8080"` `` — owned by the
   [defaults](../../annotator/defaults) annotator, so a fixture or
   validation generator reads the same stamp.
3. **Nothing**, which gives the zero value.

A companion and declared defaults compose: the companion runs, then
each declared default is applied over it.

`+gen:builder defaults=example.com/seed.OrderDefaults` names a
companion explicitly, including one in a package imported only for the
directive — an import written for a directive alone does not compile,
which is why the full-path notation exists.

Exclude a member with `` `builder:"-"` ``. Any other value is
reported rather than obeyed: a typo that silently dropped the setter
would leave an author with a builder that cannot set the member and
nothing saying why.

## The checks

The second output drives the builder the way a consumer does — the
`_test.go` ending puts it in the external test package — and asserts:

- **`From` round-trips**, member by member. Whole-value comparison is
  not used: a struct holding a slice is not comparable in Go, and
  where it is, the failure names the type rather than the member.
- **Each setter writes the member it names.**
- **Each scalar setter replaces what was there**, written with two
  distinct values. The check above cannot tell a working setter from
  one that does nothing — if the value was already present, asserting
  it is present afterwards passes either way.
- **Declared defaults reach the constructor**, except where the
  declared value *is* the type's zero. `+gen:default 0` on an int is a
  legitimate declaration, and a check for it asserts `0 == 0`, which
  passes against a constructor that ignored the declaration entirely.
- **A clone is independent**, for the members that own storage.

Assertions render through the backend's assertion dialect, so the
generated file depends on nothing but `testing`. Swapping in a helper
library is an override of nine funcmap names rather than a fork of
these templates — see [the dialect](../../../lang/golang/assert.go).

Turn the file off with `no-tests`.

## Composing with it

Three slots, and a stamp.

| Slot | On | For |
|---|---|---|
| `setters` | the builder | a setter the shapes do not model — one taking a domain type and filling several members |
| `build` | inside `Build` | making the value *correct*: a normalisation, or a validation generator's check |
| `checks` | the check file | an assertion this plugin cannot derive |

`builder.type` is stamped on the annotated declaration, carrying the
generated builder's name. A second generator reads it rather than
re-deriving the convention — which would break the moment a run
configures a different `suffix`.

## What is refused

- **A declaration with no member a builder can set.** Emitting the
  shell would hide a type that cannot do what the directive says.
- **Two members reaching one setter identifier.** `Data []byte` owes
  a text-accepting setter beside its plain one, so it reaches
  `WithDataString` — and so does a plain `DataString`. A duplicate
  method does not compile, so the builder is broken either way; the
  difference is whether the failure names the two members or lands in
  the consumer's build.

## Options

| Option | Default | |
|---|---|---|
| `suffix` | `Builder` | the word the builder's identifier carries |
| `companion-word` | `Defaults` | the word a seed function's identifier carries |
| `no-tests` | `false` | suppress the companion check file |

Each is a *word*, not a spelling: the language joins it to the type
name in its own convention.

## Limits

- **Generic declarations** get the structural checks, instantiated at
  derived witnesses, but no per-member setter checks — a test function
  cannot take type parameters, so a check naming one in a member
  position would not compile. A declaration whose constraints admit no
  witness gets a note in place of its checks.
- **Entry setters are not exercised** by the generated checks. A
  sample of a keyed type is the whole collection, and the projection
  carries no sample of its key.
- **A collision with `Build`, `Clone` or `Mutate` is not detected.**
  Those three are spelled in the template rather than derived, so the
  core cannot see them — and no member reaches them anyway while the
  setter word is non-empty.
