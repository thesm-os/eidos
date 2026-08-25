# sample

Names the values a generated check writes for a type, where the ones a
language derives are wrong or impossible.

```go
//+gen:sample NewFakeNotifier alternate=NewOtherNotifier
type Notifier interface{ Notify() error }
```

## Why

A language works out what a value looks like from what the type is,
and for most types that is the whole answer: a `string` field gets
`"test-name"`, a struct gets a literal of its members. Two cases it
cannot serve.

**An interface has no literal form.** `SamplesOf` resolves the named
type, finds an interface, and refuses. So a member of one is refused a
value, and every check that needed one is *dropped* rather than
written against a guess. That is the correct behaviour and the
quietest way for coverage to go missing — the builder's setter check
for that member simply is not there, and nothing says so.

**A declaration's values may obey rules its structure does not
state.** `Account{ID, Email string}` admits a UUID and an address
holding an `@`; the derived pair is `test-id` and `test-email`. Nothing
in eidos validates what a check writes, so this costs nothing today —
but it costs everything the moment a generated value is fed to a
constructor that does.

## What it names

| | |
|---|---|
| positional | a function taking nothing and returning the type |
| `alternate=` | a second such function, returning a value that differs |

Either may be given alone; the other stays derived. Two values rather
than one because a check comparing against a single value passes
whenever the subject already held it — which is exactly what the
second exists to prevent.

Both accept a bare name, a qualified name, or a full import path, so a
function in a package imported only for this directive can be named
without writing an import that would not compile.

A directive naming neither is reported: it is a line the author did not
finish.

## How it is read

Nothing here is read by a generator. This plugin resolves the names and
stamps them; `SourceRules.SamplesOf` prefers a stamp over the value it
would derive, so **every consumer that asks a language for a value gets
the authored one without knowing this plugin exists** — builder's
setter checks, sentinel's message check, and whatever asks next.

That is the whole design decision. Each generator reading the stamp
itself would put one rule in three places and have two of them forget
it, and a check written against a derivation its author declared wrong
is worse than no check, because it passes.

```
+gen:sample  →  sample annotator  →  meta stamp
                                        ↓
builder ──┐                         SamplesOf
sentinel ─┼── ask the language ────────┘
future  ──┘
```

## Limits

- **The two values are not checked for distinctness.** Both are
  function calls, so nothing static can tell whether they return the
  same thing. Naming one function twice produces a check that passes
  vacuously.
- **A named function's signature is not verified.** The annotator
  resolves the name and stamps it; the failure for a function taking
  arguments, or returning the wrong type, lands in the consumer's
  build of the generated file.
- **Only the whole type can be named.** There is no way to say "this
  member's value in particular" — that is what
  [defaults](../defaults) does for a declared default, and the two
  compose: defaults seeds a constructor, this supplies what a check
  writes.
